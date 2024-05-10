package test

import (
	"fmt"
	"testing"
	"time"
)

func TestBenchmarkSubmit(t *testing.T) {
	rc := NewRaftCluster(3, t)
	//defer h.Shutdown()
	SleepMs(500)
	rc.CheckSingleLeader()

	origLeaderId, _ := rc.CheckSingleLeader()

	startTime := time.Now()
	for i := 0; i < 100000; i++ {
		rc.Submit(origLeaderId, i)
	}
	elapsed := time.Since(startTime)
	t.Logf("100000 commands committed successfully took %s", elapsed)

	SleepMs(150)
	//for i := 0; i < 100000; i++ {
	//	h.CheckCommittedN(i, 3)
	//	time.Sleep(100 * time.Millisecond)
	//}

	for _, s := range rc.Cluster {
		fmt.Println(s.GetRaftModule().CommitIndex)
	}
}

func TestBenchmarkElection(t *testing.T) {
	var sum int32 = 0
	for i := 0; i < 20; i++ {
		rc := NewRaftCluster(11, t)
		SleepMs(500)
		_, term := rc.CheckSingleLeader()
		sum += term
		fmt.Println("Iteration ", i+1, " term: ", term)
		rc.Shutdown()
		time.Sleep(3 * time.Second)
	}

	fmt.Println("Required total term to elect leader in 20 iterations: ", sum)
}
