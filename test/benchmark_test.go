package test

import (
	"fmt"
	"github.com/lquyet/raft"
	"strconv"
	"testing"
	"time"
)

func TestBenchmarkClusterOf3Submit(t *testing.T) {
	rc := raft.NewRaftCluster(3, nil, t)
	//defer h.Shutdown()
	raft.SleepMs(500)
	rc.CheckSingleLeader()

	origLeaderId, _ := rc.CheckSingleLeader()

	rc.Start = time.Now()
	for i := 0; i < 100000; i++ {
		rc.Submit(origLeaderId, strconv.Itoa(i))
	}

	//for i := 0; i < 100000; i++ {
	//	h.CheckCommittedN(i, 3)
	//	time.Sleep(100 * time.Millisecond)
	//}
	time.Sleep(1 * time.Second)
}

func TestBenchmarkClusterOf5Submit(t *testing.T) {
	rc := raft.NewRaftCluster(5, nil, t)
	//defer h.Shutdown()
	raft.SleepMs(500)
	rc.CheckSingleLeader()

	origLeaderId, _ := rc.CheckSingleLeader()

	rc.Start = time.Now()
	for i := 0; i < 100000; i++ {
		rc.Submit(origLeaderId, strconv.Itoa(i))
	}

	//for i := 0; i < 100000; i++ {
	//	h.CheckCommittedN(i, 3)
	//	time.Sleep(100 * time.Millisecond)
	//}
	time.Sleep(1 * time.Second)
}

func TestBenchmarkElection(t *testing.T) {
	rc := raft.NewRaftCluster(101, nil, t)
	raft.SleepMs(500)
	_, term := rc.CheckSingleLeader()
	fmt.Println("term: ", term)
	rc.Shutdown()
	time.Sleep(1 * time.Second)
}
