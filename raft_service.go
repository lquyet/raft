package distributed_lock

import (
	"context"
	"log"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	raft "github.com/lquyet/distributed-lock/pb"
	"google.golang.org/grpc"
)

var (
	heartbeat       uint32
	electionTimeout uint32
	tick            uint32 // for election timer
)

// NodeState of a Node - Follower, Candidate, Leader, Unknown
type NodeState uint8

const (
	// Follower is the initial state
	Follower NodeState = iota

	// Candidate is state where a Node could become a Leader
	Candidate

	// Leader is state where a Node is leader
	Leader
)

func (s NodeState) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return "Unknown"
	}
}

// LogEntry is the valuable data
type LogEntry struct {
	cmd  string // for state machine
	term uint32 // beginning 1
}

// RaftServiceClient stores the client connection and the address to which it has connected
type RaftServiceClient struct {
	Client raft.RaftServiceClient
	Addr   string
}

// RaftService is the sum total state stored on each Node
type RaftService struct {
	// Persistent
	CurrentTerm uint32 // 0 on boot and monotonically increasing
	VotedFor    uint32 // 0 means nil
	Logs        []LogEntry

	// Volatile
	CommitIndex uint32 // index of highest known log entry to be committed. 0 on boot and monotonically increasing
	LastApplied uint32 // index of highest applied log entry to state machine. 0 on boot and monotonically increasing

	// Volatile - Leader
	NextIndex  map[uint32]uint32 // index here is the next log entry to send to that node. (leader's last log index)+1 on boot and monotonically increasing
	MatchIndex map[uint32]uint32 // index here is the highest log entry known to be replicated on that node. 0 on boot and monotonically increasing

	// Generic
	// NodeID is the id of this Node
	NodeID uint32
	// ElectionTime is the last time election was done for this Node
	LastElectionTime time.Time
	// Current State
	State NodeState
	// mx protects concurrent access to state
	mx sync.Mutex
	// Peers' client to make calls to
	Peers map[uint32]RaftServiceClient
	// Client to make calls from
	client raft.RaftServiceClient

	raft.UnimplementedRaftServiceServer
}

// NewRaftService initialises a new gRPC RaftService
func NewRaftService(id uint32, client *raft.RaftServiceClient, et uint32, hb uint32, t uint32, ready <-chan bool) *RaftService {
	rs := new(RaftService)
	rs.NodeID = id
	rs.Peers = make(map[uint32]RaftServiceClient)
	rs.client = *client
	rs.State = Follower
	rs.Logs = append(rs.Logs, LogEntry{cmd: "", term: rs.CurrentTerm})

	heartbeat = hb
	electionTimeout = et
	tick = t

	go func() {
		<-ready
		rs.mx.Lock()
		rs.LastElectionTime = time.Now()
		rs.mx.Unlock()
		rs.runElectionTimer()
	}()

	return rs
}

// Become is used to change to a particular state for a particular term
func (rs *RaftService) become(state NodeState, term uint32) {
	log.Printf("Node-%v becomes %v in Term %v", rs.NodeID, state, term)

	rs.State = state
	rs.CurrentTerm = term

	if state == Follower {
		rs.LastElectionTime = time.Now()
		go rs.runElectionTimer()
	}
}

// AppendEntries is invoked by Leader to replicate log entries and also used as heartbeat
func (rs *RaftService) AppendEntries(ctx context.Context, in *raft.AppendEntriesRequest) (resp *raft.AppendEntriesResponse, err error) {
	resp = new(raft.AppendEntriesResponse)

	rs.mx.Lock()
	defer rs.mx.Unlock()

	log.Printf("Node-%v received AppendEntriesRequest: {term: %v, leaderId: %v, entries: %+v, leaderCommit: %v, prevLogIndex: %v, prevLogTerm: %v}", rs.NodeID, in.GetTerm(), in.GetLeaderId(), in.GetEntries(), in.GetLeaderCommit(), in.GetPrevLogIndex(), in.GetPrevLogTerm())

	resp.Term = rs.CurrentTerm
	resp.Success = false

	// 1.
	if in.Term < rs.CurrentTerm {
		log.Printf("Node-%v's AppendEntriesResponse: {term: %v, success: %v}", rs.NodeID, resp.Term, resp.Success)
		return
	}

	if in.Term > rs.CurrentTerm {
		log.Printf("Node-%v's term out of date in AppendEntries", rs.NodeID)
		rs.become(Follower, in.Term)
	}

	// 2. Reply false if log doesn’t contain an entry at prevLogIndex whose term matches prevLogTerm
	if int(in.PrevLogIndex) >= len(rs.Logs) {
		resp.Term = rs.CurrentTerm
		log.Printf("Node-%v's AppendEntriesResponse: {term: %v, success: %v}", rs.NodeID, resp.Term, resp.Success)
		return
	}

	// 3. If an existing entry conflicts with a new one (same index but different terms), delete the existing entry and all that follow it
	if rs.Logs[in.PrevLogIndex].term != in.PrevLogTerm {
		rs.Logs = rs.Logs[:in.PrevLogIndex]
		rs.LastApplied = uint32(len(rs.Logs) - 1)
	}

	// 4. Append any new entries not already in the log
	for _, entry := range in.Entries {
		rs.Logs = append(rs.Logs, LogEntry{cmd: entry, term: in.Term})
		rs.LastApplied++
	}

	// 5. If leaderCommit > commitIndex, set commitIndex =min(leaderCommit, index of last new entry)
	if in.LeaderCommit > rs.CommitIndex {
		if in.LeaderCommit <= rs.LastApplied {
			rs.CommitIndex = in.LeaderCommit
		} else {
			rs.CommitIndex = rs.LastApplied
		}
	}

	if rs.State == Candidate {
		rs.become(Follower, in.Term)
	}

	if rs.CommitIndex > rs.LastApplied {
		rs.LastApplied++
	}

	resp.Success = true
	rs.LastElectionTime = time.Now()

	log.Printf("Node-%v's AppendEntriesResponse: {term: %v, success: %v}", rs.NodeID, resp.Term, resp.Success)
	return
}

// RequestVote is invoked by Candidate(s) to gather votes
func (rs *RaftService) RequestVote(ctx context.Context, in *raft.RequestVoteRequest) (resp *raft.RequestVoteResponse, err error) {
	resp = new(raft.RequestVoteResponse)

	rs.mx.Lock()
	defer rs.mx.Unlock()

	log.Printf("Node-%v received RequestVoteRequest: {term: %v, candidateId: %v, lastLogIndex: %v, lastLogTerm: %v}", rs.NodeID, in.GetTerm(), in.GetCandidateId(), in.GetLastLogIndex(), in.GetLastLogTerm())

	resp.Term = rs.CurrentTerm
	resp.VoteGranted = false

	// 1. Reply false if term < currentTerm
	if in.Term < rs.CurrentTerm {
		log.Printf("Node-%v's RequestVoteResponse: {term: %+v, votegranted: %v}", rs.NodeID, resp.Term, resp.VoteGranted)
		return
	}

	if in.Term > rs.CurrentTerm {
		log.Printf("Node-%v's term out of date in RequestVote", rs.NodeID)
		rs.become(Follower, in.Term)
	}

	// 2.  If votedFor is null or candidateId, and candidate’s log is at least as up-to-date as receiver’s log, grant vote
	if (rs.VotedFor == 0 || rs.VotedFor == in.GetCandidateId()) && in.LastLogIndex >= uint32(len(rs.Logs)-1) && rs.Logs[in.LastLogIndex].term >= (in.LastLogTerm) {
		resp.VoteGranted = true
		rs.VotedFor = in.GetCandidateId()
		rs.LastElectionTime = time.Now()
	}

	if rs.CommitIndex > rs.LastApplied {
		rs.LastApplied++
	}

	log.Printf("Node-%v's RequestVoteResponse: {term: %+v, votegranted: %v}", rs.NodeID, resp.Term, resp.VoteGranted)
	return
}

// JoinCluster is a helper gRPC used to connect a new Node to existing Cluster
func (rs *RaftService) JoinCluster(ctx context.Context, in *raft.JoinClusterRequest) (resp *raft.JoinClusterResponse, err error) {
	log.Printf("Node-%v received JoinClusterRequest: {id: %v, addr: %v}", rs.NodeID, in.GetId(), in.GetAddr())

	err = rs.AddPeer(in.GetId(), in.GetAddr())
	if err != nil {
		log.Printf("Node-%v failed to add peer Node-%v: %v", rs.NodeID, in.GetId(), err)
		return
	}

	addrs := make([]string, 0)
	ids := make([]uint32, 0)

	rs.mx.Lock()
	peers := rs.Peers
	rs.mx.Unlock()

	for id, client := range peers {
		if client.Addr != "" {
			ids = append(ids, id)
			addrs = append(addrs, client.Addr)
		}
	}

	// log.Printf("Node-%v's peers are: %+v", rs.NodeID, rs.Peers)

	resp = &raft.JoinClusterResponse{
		Id:    rs.NodeID,
		Addrs: addrs,
		Ids:   ids,
	}

	log.Printf("Node-%v's JoinClusterResponse: %+v", rs.NodeID, resp)

	return
}

// AddPeer adds a peer to the Node's state
func (rs *RaftService) AddPeer(id uint32, addr string) error {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return err
	}

	log.Printf("Node-%v joining Node-%v at address %v", rs.NodeID, id, addr)
	client := raft.NewRaftServiceClient(conn)

	rs.mx.Lock()
	defer rs.mx.Unlock()

	rs.Peers[id] = RaftServiceClient{Client: client, Addr: addr}
	return nil
}

func (rs *RaftService) deletePeer(nodeID uint32) {
	rs.mx.Lock()
	defer rs.mx.Unlock()

	log.Printf("Node-%v deleting Node-%v from list of peers", rs.NodeID, nodeID)
	delete(rs.Peers, nodeID)
}

func (rs *RaftService) startElectionRoutine(nodeID uint32, client raft.RaftServiceClient, req *raft.RequestVoteRequest, votes *uint32, wg *sync.WaitGroup) {

	log.Printf("Node-%v is sending RequestVote to Node-%v for term %v", rs.NodeID, nodeID, req.Term)

	defer wg.Done()

	resp, err := client.RequestVote(context.Background(), req)
	if err != nil {
		log.Printf("Node-%v error sending requestVote to Node-%v: %v", rs.NodeID, nodeID, err)
		if strings.Contains(err.Error(), "Unavailable") {
			rs.deletePeer(nodeID)
			return
		}
	}

	rs.mx.Lock()
	defer rs.mx.Unlock()

	if rs.State != Candidate {
		log.Printf("Node-%v is waiting for RequestVoteReply in state %v", rs.NodeID, rs.State)
	}

	if resp.Term > req.Term { // !=
		log.Printf("Node-%v's RequestVote term different than current(%v) from Node-%v: %v", rs.NodeID, req.Term, nodeID, resp.Term)
		rs.become(Follower, resp.Term)
		return
	}

	if resp.VoteGranted {
		atomic.AddUint32(votes, 1)
	}
}

func (rs *RaftService) startElectionNow() {
	rs.mx.Lock()
	rs.become(Candidate, rs.CurrentTerm+1)
	log.Printf("Node-%v is starting election for term %v", rs.NodeID, rs.CurrentTerm)
	rs.VotedFor = rs.NodeID
	term := rs.CurrentTerm
	rs.LastElectionTime = time.Now()
	rs.mx.Unlock()

	var votes uint32 = 1

	var wg sync.WaitGroup

	// For each peer, send RequestVote RPC
	for nodeID, peer := range rs.Peers {
		wg.Add(1)
		go rs.startElectionRoutine(nodeID, peer.Client, &raft.RequestVoteRequest{
			Term:         term,
			CandidateId:  rs.NodeID,
			LastLogIndex: rs.LastApplied,
			LastLogTerm:  rs.Logs[rs.LastApplied].term,
		}, &votes, &wg)
	}

	wg.Wait()

	if (votes * 2) > uint32(len(rs.Peers)+1) {
		// Got quorum
		log.Printf("Node-%v wins election with %d votes", rs.NodeID, votes)
		rs.lead()
		return
	}

	go rs.runElectionTimer()

}

func (rs *RaftService) runElectionTimer() {
	timeout := time.Duration(electionTimeout+uint32(rand.Intn(int(electionTimeout)))) * time.Millisecond
	rs.mx.Lock()
	term := rs.CurrentTerm
	rs.mx.Unlock()

	log.Printf("Node-%v's election timer started for Term %v", rs.NodeID, term)

	ticker := time.NewTicker(time.Duration(tick) * time.Millisecond)
	defer ticker.Stop()
	for {
		<-ticker.C

		rs.mx.Lock()

		if term != rs.CurrentTerm {
			log.Printf("Node-%v's term changed from %v to %v", rs.NodeID, term, rs.CurrentTerm)
			rs.mx.Unlock()
			return
		}

		if rs.State != Follower && rs.State != Candidate {
			log.Printf("Node-%v is neither Follower nor Candidate in election. State is %v", rs.NodeID, rs.State.String())
			rs.mx.Unlock()
			return
		}

		if elapsed := time.Since(rs.LastElectionTime); elapsed >= timeout {
			rs.mx.Unlock()
			rs.startElectionNow()
			return
		}

		rs.mx.Unlock()
	}
}

func (rs *RaftService) sendHeartbeatRoutine(nodeID uint32, client raft.RaftServiceClient, req *raft.AppendEntriesRequest) {
	log.Printf("Node-%v is sending heartbeat to Node-%v", rs.NodeID, nodeID)

	resp, err := client.AppendEntries(context.Background(), req)
	if err != nil {
		log.Printf("error sending heartbeat to Node-%v: %v", nodeID, err)
		if strings.Contains(err.Error(), "Unavailable") {
			rs.deletePeer(nodeID)
			if rs.State != Leader {
				rs.startElectionNow()
			}
			return
		}
	}

	rs.mx.Lock()
	defer rs.mx.Unlock()
	if resp.Term > req.Term {
		log.Printf("Node-%v's heartbeat term different than current(%v) from Node-%v: %v", rs.NodeID, req.Term, nodeID, resp.Term)
		rs.become(Follower, resp.Term)
	}
}

// SendHeartbeat send a heartbeat to all peers and then collects their replies to adjust raft state
func (rs *RaftService) sendHeartbeat() {
	rs.mx.Lock()
	term := rs.CurrentTerm
	rs.mx.Unlock()

	// For each peer, send AppendEntriesRequest with empty "Entries"
	for nodeID, peer := range rs.Peers {
		go rs.sendHeartbeatRoutine(nodeID, peer.Client, &raft.AppendEntriesRequest{
			Term:         term,
			LeaderId:     rs.NodeID,
			LeaderCommit: rs.CommitIndex,
		})
	}
}

func (rs *RaftService) heartbeatWrapper() {
	ticker := time.NewTicker(time.Duration(heartbeat) * time.Millisecond)
	defer ticker.Stop()

	for {
		rs.sendHeartbeat()
		<-ticker.C

		rs.mx.Lock()
		if rs.State != Leader {
			rs.mx.Unlock()
			break
		}
		rs.mx.Unlock()
	}
}

// Lead is invoked by Candidate soon after it wins the election
func (rs *RaftService) lead() {
	rs.mx.Lock()
	rs.become(Leader, rs.CurrentTerm)
	rs.mx.Unlock()

	go rs.heartbeatWrapper()
}
