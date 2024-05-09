package raft

import (
	"context"
	"fmt"
	proto "github.com/lquyet/raft/pb"
	"strconv"
	"sync"
	"testing"
	"time"
)

type RaftCluster struct {
	Cluster      []*Server
	NumOfServers int
	Connected    []bool

	mu          *sync.Mutex
	commits     [][]CommitEntry
	commitChans []chan CommitEntry

	t *testing.T
}

func NewRaftCluster(n int, t *testing.T) *RaftCluster {
	ns := make([]*Server, n)
	ready := make(chan interface{})
	connected := make([]bool, n)

	for i := 0; i < n; i++ {
		peerIds := make([]int32, 0)
		peerAddrs := make(map[int32]string)
		for p := 0; p < n; p++ {
			if p != i {
				peerIds = append(peerIds, int32(p))
				peerAddrs[int32(p)] = fmt.Sprintf("localhost:%d", 8080+p)
			}
		}
		fmt.Println(peerIds, peerAddrs)
		ns[i] = NewServer(int32(i), peerIds, peerAddrs, ready, "localhost:"+fmt.Sprintf("%d", 8080+i))
		go func(i int) {
			ns[i].Serve()
		}(i)
	}

	// Connect all peers to each other.
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i != j {
				ns[i].ConnectToPeer(int32(j))
			}
		}
		connected[i] = true
	}

	// Getting all commit channels
	commitChans := make([]chan CommitEntry, n)
	for i := 0; i < n; i++ {
		commitChans[i] = *ns[i].GetRaftModule().GetCommitChannel()
	}

	// We need a place to save the commits
	// Basically we just look for all commits in commitChan and push them here
	commits := make([][]CommitEntry, n)

	close(ready)

	raftCluster := RaftCluster{
		Cluster:      ns,
		NumOfServers: n,
		Connected:    connected,

		mu:          &sync.Mutex{},
		commits:     commits,
		commitChans: commitChans,

		t: t,
	}

	for i := 0; i < n; i++ {
		go raftCluster.collectCommits(i)
	}

	return &raftCluster
}

func (rc *RaftCluster) Shutdown() {
	// Disconnect all pairs of servers
	for i := 0; i < rc.NumOfServers; i++ {
		rc.Cluster[i].DisconnectAll()
		rc.Connected[i] = false
	}

	// Shutdown all servers
	for i := 0; i < rc.NumOfServers; i++ {
		rc.Cluster[i].Shutdown()
	}
}

// Submit submits a command to a server in the cluster.
func (rc *RaftCluster) Submit(serverId int32, cmd int) bool {
	res, err := rc.Cluster[serverId].GetRaftModule().Submit(context.Background(), &proto.SubmitRequest{
		Command: strconv.Itoa(cmd),
	})
	if err != nil {
		rc.t.Error(err)
	}

	return res.Success
}

// DisconnectPeers disconnects a server from all other servers in the cluster.
func (rc *RaftCluster) DisconnectPeers(id int32) {
	rc.Cluster[id].DisconnectAll()
	for i := 0; i < rc.NumOfServers; i++ {
		if int32(i) != id {
			rc.Cluster[i].DisconnectPeer(id)
		}
	}

	rc.Connected[id] = false
}

func (rc *RaftCluster) ReconnectPeers(id int32) {
	for j := 0; j < rc.NumOfServers; j++ {
		if int32(j) != id {
			if err := rc.Cluster[id].ConnectToPeer(int32(j)); err != nil {
				rc.t.Fatal(err)
			}
			if err := rc.Cluster[j].ConnectToPeer(id); err != nil {
				rc.t.Fatal(err)
			}
		}
	}
	rc.Connected[id] = true
}

func (rc *RaftCluster) CheckSingleLeader() (int32, int32) {
	for r := 0; r < 8; r++ {
		var leaderId int32 = -1
		var leaderTerm int32 = -1
		for i := 0; i < rc.NumOfServers; i++ {
			if rc.Connected[i] {
				_, term, isLeader := rc.Cluster[i].GetRaftModule().GetInfo()
				if isLeader {
					if leaderId < 0 {
						leaderId = int32(i)
						leaderTerm = term
					} else {
						rc.t.Fatalf("both %d and %d think they're leaders", leaderId, i)
					}
				}
			}
		}
		if leaderId >= 0 {
			return leaderId, leaderTerm
		}
		time.Sleep(350 * time.Millisecond)
	}

	rc.t.Fatalf("leader not found")
	return -1, -1
}

func (rc *RaftCluster) CheckNoLeader() {
	for i := 0; i < rc.NumOfServers; i++ {
		if rc.Connected[i] {
			_, _, isLeader := rc.Cluster[i].GetRaftModule().GetInfo()
			if isLeader {
				rc.t.Fatalf("server %d leader; want none", i)
			}
		}
	}
}

// CheckCommitted verifies that all connected servers have cmd committed with
// // the same index. It also verifies that all commands *before* cmd in
// // the commit sequence match. For this to work properly, all commands submitted
// // to Raft should be unique positive integers.
// Parameters:
// cmd (int): The command to check if it has been committed.
//
// Returns:
// nc (int32): The number of connected servers that have the command committed.
// index (int32): The index at which the command is committed. If the command is not found, it returns -1.
func (rc *RaftCluster) CheckCommitted(cmd int) (nc int32, index int32) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Find the length of the commits slice for connected servers. All commits length must be the same
	commitsLen := -1
	for i := 0; i < rc.NumOfServers; i++ {
		if rc.Connected[i] {
			if commitsLen >= 0 {
				// If this was set already, expect the new length to be the same.
				if len(rc.commits[i]) != commitsLen {
					rc.t.Fatalf("commits[%d] = %d, commitsLen = %d", i, rc.commits[i], commitsLen)
				}
			} else {
				commitsLen = len(rc.commits[i])
			}
		}
	}

	// Check consistency of commits from the start and to the command we're asked
	// about. This loop will return once a command=cmd is found.
	for c := 0; c < commitsLen; c++ {
		cmdAtC := ""
		for i := 0; i < rc.NumOfServers; i++ {
			if rc.Connected[i] {
				cmdOfN := rc.commits[i][c].Command.(string)
				if len(cmdAtC) > 0 {
					if cmdOfN != cmdAtC {
						rc.t.Errorf("got %s, want %s at h.commits[%d][%d]", cmdOfN, cmdAtC, i, c)
					}
				} else {
					cmdAtC = cmdOfN
				}
			}
		}
		if cmdAtC == strconv.Itoa(cmd) {
			// Check consistency of Index.
			var index int32 = -1
			var nc int32 = 0
			for i := 0; i < rc.NumOfServers; i++ {
				if rc.Connected[i] {
					if index >= 0 && rc.commits[i][c].Index != int32(index) {
						rc.t.Errorf("got Index=%d, want %d at h.commits[%d][%d]", rc.commits[i][c].Index, index, i, c)
					} else {
						index = rc.commits[i][c].Index
					}
					nc++
				}
			}
			return nc, index
		}
	}

	// If there's no early return, we haven't found the command we were looking
	// for.
	rc.t.Errorf("cmd=%d not found in commits", cmd)
	return -1, -1
}

// CheckCommittedN verifies that cmd was committed by exactly n connected
// servers.
func (rc *RaftCluster) CheckCommittedN(cmd int, n int) {
	nc, _ := rc.CheckCommitted(cmd)
	if nc != int32(n) {
		rc.t.Errorf("CheckCommittedN got nc=%d, want %d", nc, n)
	}
}

// CheckNotCommitted verifies that no command equal to cmd has been committed
// by any of the active servers yet.
func (rc *RaftCluster) CheckNotCommitted(cmd int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	for i := 0; i < rc.NumOfServers; i++ {
		if rc.Connected[i] {
			for c := 0; c < len(rc.commits[i]); c++ {
				gotCmd := rc.commits[i][c].Command.(string)
				if gotCmd == strconv.Itoa(cmd) {
					rc.t.Errorf("found %d at commits[%d][%d], expected none", cmd, i, c)
				}
			}
		}
	}
}

func sleepMs(n int) {
	time.Sleep(time.Duration(n) * time.Millisecond)
}

// collectCommits reads channel commitChans[i] and adds all received entries
// to the corresponding commits[i]. It's blocking and should be run in a
// separate goroutine. It returns when commitChans[i] is closed.
func (rc *RaftCluster) collectCommits(i int) {
	for c := range rc.commitChans[i] {
		rc.mu.Lock()
		//tlog("collectCommits(%d) got %+v", i, c)
		rc.commits[i] = append(rc.commits[i], c)
		rc.mu.Unlock()
	}
}
