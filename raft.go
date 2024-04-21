package distributed_lock

import (
	"context"
	"fmt"
	proto "github.com/lquyet/distributed-lock/pb"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"
)

const DebugMode = 1

type RaftState int

const (
	Follower RaftState = iota
	Candidate
	Leader
	Dead
)

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
	// TODO: implement commit entry struct and add to channel. NOT USED YET
	commitChan chan<- any

	// newCommitReady channel to report new committed entries to the server
	// TODO: same as above
	newCommitReadyChan chan interface{}

	// Persistent raft state on all servers
	currentTerm int32
	votedFor    int32
	log         []proto.LogEntry

	// Volatile state on all servers
	commitIndex        int32
	lastApplied        int32
	state              RaftState
	electionResetEvent time.Time

	// Volatile state on leaders
	nextIndex  map[int32]int32
	matchIndex map[int32]int32
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
	if len(os.Getenv("RAFT_FORCE_MORE_REELECTION")) > 0 && rand.Intn(3) == 0 {
		return time.Duration(150) * time.Millisecond
	} else {
		return time.Duration(150+rand.Intn(150)) * time.Millisecond
	}
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
				LeaderCommit: rm.commitIndex,
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

					// TODO: commit logic
				} else {
					rm.nextIndex[peerId] = ni - 1
					rm.dlog("AppendEntries not successful: peer=%d, nextIndex=%d", peerId, rm.nextIndex[peerId])
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

func (rm *RaftModule) AppendEntries(ctx context.Context, request *proto.AppendEntriesRequest) (*proto.AppendEntriesResponse, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.state == Dead {
		return nil, nil
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

			if request.LeaderCommit > rm.commitIndex {
				rm.commitIndex = min(request.LeaderCommit, int32(len(rm.log))-1)
				rm.dlog("... setting commitIndex=%d", rm.commitIndex)
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
