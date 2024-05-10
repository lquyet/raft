package raft

import (
	"context"
	"errors"
	"fmt"
	proto "github.com/lquyet/raft/pb"
	"log"
	"math/rand"
	"sync"
	"time"
)

const DebugMode = 0

type RaftState int

const (
	Follower RaftState = iota
	Candidate
	Leader
	Dead
)

type CommitEntry struct {
	Command interface{}
	Index   int32
	Term    int32
}

type RaftModule struct {
	// mutex to protect shared resources in concurrent environment
	mu sync.Mutex

	// node identifier
	id int32

	// list of peers in cluster
	peerIds []int32

	// server containing this Raft Module, to issue RPC calls
	server *Server

	// commitChan channel to report committed entries to the server
	// To be clear, the state machine subscribes to this channel and executes commands from entries in order.
	//commitChan chan<- CommitEntry
	commitChan chan CommitEntry

	// newCommitReady channel to report new committed entries to the server
	newCommitReadyChan chan struct{}

	// Persistent raft state on all servers
	currentTerm int32            // the latest term server has seen (init to 0 on first boot, increases monotonically)
	votedFor    int32            // candidateId that received vote in current term (or null if none)
	log         []proto.LogEntry // log entries; each entry contains command for state machine, and term when entry was received by leader

	// Volatile state on all servers
	CommitIndex        int32     // index of highest log entry known to be committed (init to 0, increases monotonically)
	lastApplied        int32     // index of highest log entry applied to state machine (init to 0, increases monotonically)
	state              RaftState // whether the server is a follower, candidate, or leader
	electionResetEvent time.Time // time of the last election reset

	// Volatile state on leaders (reinitialized after election)
	nextIndex  map[int32]int32 // for each server, index of the next log entry to send to that server (initialized to leader last log index + 1)
	matchIndex map[int32]int32 // for each server, index of highest log entry known to be replicated on server
}

func (rm *RaftModule) dlog(format string, args ...interface{}) {
	if DebugMode > 0 {
		format = fmt.Sprintf("[%d] ", rm.id) + format
		log.Printf(format, args...)
	}
}

func (rm *RaftModule) becomeFollower(term int32) {
	rm.dlog("becomes Follower with term=%d; log=%v", term, rm.log)
	rm.state = Follower
	rm.currentTerm = term
	rm.votedFor = -1
	rm.electionResetEvent = time.Now()
	rm.dlog("becoming a FOLLOWER")

	go rm.runElectionTimer()
}

func (rm *RaftModule) electionTimeout() time.Duration {
	// If RAFT_FORCE_MORE_REELECTION is set, stress-test by deliberately
	// generating a hard-coded number very often. This will create collisions
	// between different servers and force more re-elections.
	//if len(os.Getenv("RAFT_FORCE_MORE_REELECTION")) > 0 && rand.Intn(3) == 0 {
	//	return time.Duration(150) * time.Millisecond
	//} else {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
	//}
}

func (rm *RaftModule) lastLogIndexAndTerm() (int32, int32) {
	if len(rm.log) > 0 {
		lastIndex := int32(len(rm.log)) - 1
		return lastIndex, rm.log[lastIndex].Term
	} else {
		return -1, -1
	}
}

func (rm *RaftModule) leaderSendHeartbeats() {
	rm.mu.Lock()
	if rm.state != Leader {
		rm.mu.Unlock()
		return
	}

	savedCurrentTerm := rm.currentTerm
	rm.mu.Unlock()

	for _, peerId := range rm.peerIds {
		go func(peerId int32) {
			rm.mu.Lock()
			ni := rm.nextIndex[peerId]
			prevLogIndex := ni - 1
			prevLogTerm := ToInt32(-1)
			if prevLogIndex >= 0 {
				prevLogTerm = rm.log[prevLogIndex].Term
			}
			entries := rm.log[ni:]

			request := proto.AppendEntriesRequest{
				Term:         savedCurrentTerm,
				LeaderId:     rm.id,
				PrevLogIndex: prevLogIndex,
				PrevLogTerm:  prevLogTerm,
				Entries:      EntriesValueToPointer(entries),
				LeaderCommit: rm.CommitIndex,
			}
			rm.mu.Unlock()

			rm.dlog("sending AppendEntries to %d: ni=%d, entries=%v", peerId, ni, entries)
			response, err := rm.server.peerClients[peerId].AppendEntries(context.Background(), &request)
			if err != nil {
				return
			}

			rm.mu.Lock()
			defer rm.mu.Unlock()
			if response.Term > rm.currentTerm {
				rm.dlog("term out of date in heartbeat response")
				rm.becomeFollower(response.Term)
				return
			}

			if rm.state == Leader && savedCurrentTerm == response.Term {
				if response.Success {
					rm.nextIndex[peerId] = ni + ToInt32(len(entries))
					rm.matchIndex[peerId] = rm.nextIndex[peerId] - 1
					rm.dlog("AppendEntries successful: peer=%d, nextIndex=%d, matchIndex=%d", peerId, rm.nextIndex[peerId], rm.matchIndex[peerId])

					savedCommitIndex := rm.CommitIndex
					for i := rm.CommitIndex + 1; i < int32(len(rm.log)); i++ {
						if rm.log[i].Term == rm.currentTerm {
							count := 1
							for _, pid := range rm.peerIds {
								if rm.matchIndex[pid] >= i {
									count++
								}
							}
							if count*2 > len(rm.peerIds)+1 {
								rm.CommitIndex = i
							}
						}
					}

					if savedCommitIndex != rm.CommitIndex {
						rm.dlog("new commit detected: ", rm.CommitIndex)
						rm.newCommitReadyChan <- struct{}{}
					}
				} else {
					rm.nextIndex[peerId] = ni - 1
					rm.dlog("AppendEntries not successful: peer=%d, nextIndex=%d", peerId, ni-1)
				}
			}
		}(peerId)
	}
}

func (rm *RaftModule) startLeader() {
	rm.state = Leader

	for _, peerId := range rm.peerIds {
		rm.nextIndex[peerId] = ToInt32(len(rm.log))
		rm.matchIndex[peerId] = -1
	}
	rm.dlog("becomes Leader; term=%d, nextIndex=%v, matchIndex=%v", rm.currentTerm, rm.nextIndex, rm.matchIndex)

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		// Send periodic heartbeats, as long as still leader.
		for {
			rm.leaderSendHeartbeats()
			<-ticker.C

			rm.mu.Lock()
			if rm.state != Leader {
				rm.mu.Unlock()
				return
			}
			rm.mu.Unlock()
		}
	}()

}

func (rm *RaftModule) startElection() {
	rm.state = Candidate
	rm.currentTerm += 1
	savedCurrentTerm := rm.currentTerm
	rm.electionResetEvent = time.Now()
	rm.votedFor = rm.id
	rm.dlog("becomes Candidate (currentTerm=%d); log=%v", savedCurrentTerm, rm.log)

	voteReceived := 1

	for _, peerId := range rm.peerIds {
		go func(peerId int32) {
			rm.mu.Lock()
			savedLastLogIndex, savedLastLogTerm := rm.lastLogIndexAndTerm()
			rm.mu.Unlock()

			request := proto.RequestVoteRequest{
				Term:         savedCurrentTerm,
				CandidateId:  rm.id,
				LastLogIndex: savedLastLogIndex,
				LastLogTerm:  savedLastLogTerm,
			}

			rm.dlog("sending RequestVote to %d: %+v", peerId, request)
			response, err := rm.server.peerClients[peerId].RequestVote(context.Background(), &request)
			if err != nil {
				return
			}

			rm.mu.Lock()
			defer rm.mu.Unlock()
			rm.dlog("received RequestVoteResponse: %+v", response)

			if rm.state != Candidate {
				rm.dlog("while waiting for reply, state=%s", rm.state)
				return
			}

			if response.Term > savedCurrentTerm {
				rm.dlog("term out of date in RequestVoteResponse")
				rm.becomeFollower(response.Term)
				return
			} else if response.Term == savedCurrentTerm {
				if response.VoteGranted {
					voteReceived++
					rm.dlog("votes: %d", voteReceived)
					if voteReceived*2 > len(rm.peerIds)+1 {
						rm.dlog("wins election with %d votes", voteReceived)
						rm.startLeader()
						return
					}
				}
			}

		}(peerId)
	}

	//// Handle a special case when there is only one server in the cluster
	//if len(rm.peerIds) == 0 && rm.state == Candidate {
	//	rm.startLeader()
	//}

	go rm.runElectionTimer()
}

func (rm *RaftModule) runElectionTimer() {
	timeoutDuration := rm.electionTimeout()
	rm.mu.Lock()
	termStarted := rm.currentTerm
	rm.mu.Unlock()
	rm.dlog("election timer started (%v), term=%d", timeoutDuration, termStarted)

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		<-ticker.C

		rm.mu.Lock()
		if rm.state != Candidate && rm.state != Follower {
			rm.dlog("in election timer state=%s, bailing out", rm.state)
			rm.mu.Unlock()
			return
		}

		if termStarted != rm.currentTerm {
			rm.dlog("in election timer term changed from %d to %d, bailing out", termStarted, rm.currentTerm)
			rm.mu.Unlock()
			return
		}

		// Start an election if we haven't heard from a leader or haven't voted for
		// someone for the duration of the timeout.
		if elapsed := time.Since(rm.electionResetEvent); elapsed >= timeoutDuration {
			rm.startElection()
			rm.mu.Unlock()
			return
		}
		rm.mu.Unlock()
	}
}

var NodeDead = errors.New("Node is DEAD")

func (rm *RaftModule) AppendEntries(ctx context.Context, request *proto.AppendEntriesRequest) (*proto.AppendEntriesResponse, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.state == Dead {
		return nil, NodeDead
	}
	rm.dlog("AppendEntries incoming request: %+v", request)

	if request.Term > rm.currentTerm {
		rm.dlog("AppendEntries request receive has higher term then rm term")
		rm.dlog("... become follower")
		rm.becomeFollower(request.Term)
	}

	reply := proto.AppendEntriesResponse{}

	reply.Success = false

	if request.Term == rm.currentTerm {
		if rm.state != Follower {
			rm.becomeFollower(request.Term)
		}
		rm.electionResetEvent = time.Now()

		if request.PrevLogTerm == -1 ||
			(request.PrevLogIndex < int32(len(rm.log)) && request.PrevLogTerm == rm.log[request.PrevLogIndex].Term) {
			reply.Success = true

			logInsertIndex := request.PrevLogIndex + 1
			newEntriesIndex := 0

			for {
				if logInsertIndex >= int32(len(rm.log)) || newEntriesIndex >= len(request.Entries) {
					break
				}

				if rm.log[logInsertIndex].Term != request.Entries[newEntriesIndex].Term {
					break
				}

				logInsertIndex++
				newEntriesIndex++
			}

			if newEntriesIndex < len(request.Entries) {
				rm.dlog("... inserting entries %v from index %d", request.Entries[newEntriesIndex:], logInsertIndex)
				rm.log = append(rm.log[:logInsertIndex], EntriesPointerToValue(request.Entries[newEntriesIndex:])...)
				rm.dlog("... log is now: %v", rm.log)
			}

			if request.LeaderCommit > rm.CommitIndex {
				rm.CommitIndex = min(request.LeaderCommit, int32(len(rm.log))-1)
				rm.dlog("... setting commitIndex=%d", rm.CommitIndex)
				rm.newCommitReadyChan <- struct{}{}
			}
		}
	}

	reply.Term = rm.currentTerm
	rm.dlog("AppendEntries response: %+v", reply)
	return &reply, nil
}

func (rm *RaftModule) RequestVote(ctx context.Context, request *proto.RequestVoteRequest) (*proto.RequestVoteResponse, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.state == Dead {
		return nil, nil
	}

	lastLogIndex, lastLogTerm := rm.lastLogIndexAndTerm()
	rm.dlog("RequestVote incoming request: %+v, lastLogIndex=%d, lastLogTerm=%d", request, lastLogIndex, lastLogTerm)

	response := proto.RequestVoteResponse{}

	if request.Term > rm.currentTerm {
		rm.dlog("RequestVote request receive has higher term then rm term")
		rm.dlog("... become follower")
		rm.becomeFollower(request.Term)
	}

	if rm.currentTerm == request.Term &&
		(rm.votedFor == -1 || rm.votedFor == request.CandidateId) &&
		(request.LastLogTerm > lastLogTerm || (request.LastLogTerm == lastLogTerm && request.LastLogIndex >= lastLogIndex)) {
		rm.dlog("... granting vote")
		response.VoteGranted = true
		rm.votedFor = request.CandidateId
		rm.electionResetEvent = time.Now()
	} else {
		rm.dlog("... rejecting vote")
		response.VoteGranted = false
	}
	response.Term = rm.currentTerm
	rm.dlog("RequestVote response: %+v", response)
	return &response, nil
}

func (rm *RaftModule) Submit(ctx context.Context, request *proto.SubmitRequest) (*proto.SubmitResponse, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.dlog("Submit incoming request: %+v", request)
	if rm.state == Leader {
		rm.log = append(rm.log, proto.LogEntry{Term: rm.currentTerm, Command: request.Command})
		rm.dlog("... log is now: %v", rm.log)
		return &proto.SubmitResponse{Success: true}, nil
	}

	return &proto.SubmitResponse{Success: false}, nil
}

func (rm *RaftModule) commitChanHandler() {
	for range rm.newCommitReadyChan {
		rm.mu.Lock()
		savedTerm := rm.currentTerm
		savedLastApplied := rm.lastApplied

		var entries []proto.LogEntry
		if rm.CommitIndex > rm.lastApplied {
			entries = rm.log[rm.lastApplied+1 : rm.CommitIndex+1]
			rm.lastApplied = rm.CommitIndex
		}
		rm.mu.Unlock()
		rm.dlog("commitChanHandler: entries=%v", entries)
		rm.dlog("savedTerm=%d, savedLastApplied=%d", savedTerm, savedLastApplied)

		for i := range entries {
			rm.commitChan <- CommitEntry{
				Command: entries[i].Command,
				Index:   savedLastApplied + int32(i) + 1,
				Term:    savedTerm,
			}
		}
	}
}

func NewRaftModule(id int32, peerIds []int32, server *Server, ready <-chan interface{}) *RaftModule {
	rm := RaftModule{}
	rm.id = id
	rm.peerIds = peerIds
	rm.server = server
	rm.state = Follower
	rm.CommitIndex = -1
	rm.lastApplied = -1
	rm.currentTerm = 0
	rm.votedFor = -1
	rm.log = make([]proto.LogEntry, 0, 10)
	rm.nextIndex = make(map[int32]int32)
	rm.matchIndex = make(map[int32]int32)

	rm.commitChan = make(chan CommitEntry)
	rm.newCommitReadyChan = make(chan struct{}, 16)

	go func() {
		<-ready
		rm.mu.Lock()
		rm.electionResetEvent = time.Now()
		rm.mu.Unlock()
		rm.runElectionTimer()
		rm.dlog("ran election timer")
	}()

	go rm.commitChanHandler()
	return &rm
}

func (rm *RaftModule) GetCommitChannel() *chan CommitEntry {
	return &rm.commitChan
}

func (rm *RaftModule) Stop() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.state = Dead
	rm.dlog("Node shutting down")
}

func (rm *RaftModule) GetInfo() (id int32, term int32, isLeader bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.id, rm.currentTerm, rm.state == Leader
}
