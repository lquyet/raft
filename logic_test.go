package distributed_lock

import (
	"github.com/fortytw2/leaktest"
	"testing"
	"time"
)

func TestElectionBasic(t *testing.T) {
	rc := NewRaftCluster(3, t)
	defer rc.Shutdown()

	rc.CheckSingleLeader()
}

func TestElectionLeaderDisconnect(t *testing.T) {
	rc := NewRaftCluster(3, t)
	defer rc.Shutdown()

	origLeaderId, origTerm := rc.CheckSingleLeader()

	rc.DisconnectPeers(origLeaderId)
	sleepMs(350)

	newLeaderId, newTerm := rc.CheckSingleLeader()
	if newLeaderId == origLeaderId {
		t.Errorf("want new leader to be different from orig leader")
	}
	if newTerm <= origTerm {
		t.Errorf("want newTerm <= origTerm, got %d and %d", newTerm, origTerm)
	}
}

func TestElectionLeaderAndAnotherDisconnect(t *testing.T) {
	rc := NewRaftCluster(3, t)
	defer rc.Shutdown()

	origLeaderId, _ := rc.CheckSingleLeader()

	rc.DisconnectPeers(origLeaderId)
	otherId := (origLeaderId + 1) % 3
	rc.DisconnectPeers(otherId)

	// No quorum.
	sleepMs(450)
	rc.CheckNoLeader()

	// Reconnect one other server; now we'll have quorum.
	rc.ReconnectPeers(otherId)
	rc.CheckSingleLeader()
}

func TestDisconnectAllThenRestore(t *testing.T) {
	rc := NewRaftCluster(3, t)
	defer rc.Shutdown()

	sleepMs(100)
	//	Disconnect all servers from the start. There will be no leader.
	for i := 0; i < 3; i++ {
		rc.DisconnectPeers(int32(i))
	}
	sleepMs(450)
	rc.CheckNoLeader()

	// Reconnect all servers. A leader will be found.
	for i := 0; i < 3; i++ {
		rc.ReconnectPeers(int32(i))
	}
	rc.CheckSingleLeader()
}

func TestElectionLeaderDisconnectThenReconnect(t *testing.T) {
	rc := NewRaftCluster(3, t)
	defer rc.Shutdown()
	origLeaderId, _ := rc.CheckSingleLeader()

	rc.DisconnectPeers(origLeaderId)

	sleepMs(350)
	newLeaderId, newTerm := rc.CheckSingleLeader()

	rc.ReconnectPeers(origLeaderId)
	sleepMs(150)

	againLeaderId, againTerm := rc.CheckSingleLeader()

	if newLeaderId != againLeaderId {
		t.Errorf("again leader id got %d; want %d", againLeaderId, newLeaderId)
	}
	if againTerm != newTerm {
		t.Errorf("again term got %d; want %d", againTerm, newTerm)
	}
}

func TestElectionLeaderDisconnectThenReconnect5(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	rc := NewRaftCluster(5, t)
	defer rc.Shutdown()

	origLeaderId, _ := rc.CheckSingleLeader()

	rc.DisconnectPeers(origLeaderId)
	sleepMs(150)
	newLeaderId, newTerm := rc.CheckSingleLeader()

	rc.ReconnectPeers(origLeaderId)
	sleepMs(150)

	againLeaderId, againTerm := rc.CheckSingleLeader()

	if newLeaderId != againLeaderId {
		t.Errorf("again leader id got %d; want %d", againLeaderId, newLeaderId)
	}
	if againTerm != newTerm {
		t.Errorf("again term got %d; want %d", againTerm, newTerm)
	}
}

func TestElectionFollowerComesBack(t *testing.T) {
	//defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	rc := NewRaftCluster(3, t)
	defer rc.Shutdown()

	origLeaderId, origTerm := rc.CheckSingleLeader()

	otherId := (origLeaderId + 1) % 3
	rc.DisconnectPeers(otherId)
	time.Sleep(650 * time.Millisecond)
	rc.ReconnectPeers(otherId)
	sleepMs(150)

	// We can't have an assertion on the new leader id here because it depends
	// on the relative election timeouts. We can assert that the term changed,
	// however, which implies that re-election has occurred.
	_, newTerm := rc.CheckSingleLeader()
	if newTerm <= origTerm {
		t.Errorf("newTerm=%d, origTerm=%d", newTerm, origTerm)
	}
}

func TestCommitOneCommand(t *testing.T) {
	//defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	rc := NewRaftCluster(3, t)
	//defer rc.Shutdown()

	origLeaderId, _ := rc.CheckSingleLeader()

	//tlog("submitting 42 to %d", origLeaderId)
	isLeader := rc.Submit(origLeaderId, 42)
	if !isLeader {
		t.Errorf("want id=%d leader, but it's not", origLeaderId)
	}

	sleepMs(500)
	rc.CheckCommittedN(42, 3)
}

func TestSubmitNonLeaderFails(t *testing.T) {
	rc := NewRaftCluster(3, t)
	defer rc.Shutdown()

	origLeaderId, _ := rc.CheckSingleLeader()
	sid := (origLeaderId + 1) % 3
	//tlog("submitting 42 to %d", sid)
	isLeader := rc.Submit(sid, 42)
	if isLeader {
		t.Errorf("want id=%d !leader, but it is", sid)
	}
	sleepMs(10)
}

func TestCommitMultipleCommands(t *testing.T) {
	rc := NewRaftCluster(3, t)
	defer rc.Shutdown()

	origLeaderId, _ := rc.CheckSingleLeader()

	values := []int{42, 55, 81}
	for _, v := range values {
		//tlog("submitting %d to %d", v, origLeaderId)
		isLeader := rc.Submit(origLeaderId, v)
		if !isLeader {
			t.Errorf("want id=%d leader, but it's not", origLeaderId)
		}
		sleepMs(300)
	}

	sleepMs(500)
	nc, i1 := rc.CheckCommitted(42)
	_, i2 := rc.CheckCommitted(55)
	if nc != 3 {
		t.Errorf("want nc=3, got %d", nc)
	}
	if i1 >= i2 {
		t.Errorf("want i1<i2, got i1=%d i2=%d", i1, i2)
	}

	_, i3 := rc.CheckCommitted(81)
	if i2 >= i3 {
		t.Errorf("want i2<i3, got i2=%d i3=%d", i2, i3)
	}
}

func TestCommitWithDisconnectionAndRecover(t *testing.T) {
	rc := NewRaftCluster(3, t)
	//defer rc.Shutdown()

	// Submit a couple of values to a fully connected cluster.
	origLeaderId, _ := rc.CheckSingleLeader()
	rc.Submit(origLeaderId, 5)
	sleepMs(50)
	rc.Submit(origLeaderId, 6)

	sleepMs(300)
	rc.CheckCommittedN(6, 3)

	dPeerId := (origLeaderId + 1) % 3
	rc.DisconnectPeers(dPeerId)
	sleepMs(300)

	// Submit a new command; it will be committed but only to two servers.
	rc.Submit(origLeaderId, 7)
	sleepMs(300)
	rc.CheckCommittedN(7, 2)

	// Now reconnect dPeerId and wait a bit; it should find the new command too.
	rc.ReconnectPeers(dPeerId)
	sleepMs(300)
	rc.CheckSingleLeader()

	sleepMs(300)
	rc.CheckCommittedN(7, 3)
}

func TestNoCommitWithNoQuorum(t *testing.T) {
	//defer leaktest.CheckTimeout(t, 100*time.Millisecond)()
	rc := NewRaftCluster(3, t)
	//defer rc.Shutdown()

	// Submit a couple of values to a fully connected cluster.
	origLeaderId, origTerm := rc.CheckSingleLeader()
	rc.Submit(origLeaderId, 5)
	sleepMs(50)
	rc.Submit(origLeaderId, 6)

	sleepMs(300)
	rc.CheckCommittedN(6, 3)

	// Disconnect both followers.
	dPeer1 := (origLeaderId + 1) % 3
	dPeer2 := (origLeaderId + 2) % 3
	rc.DisconnectPeers(dPeer1)
	rc.DisconnectPeers(dPeer2)
	sleepMs(300)

	rc.Submit(origLeaderId, 8)
	sleepMs(300)
	rc.CheckNotCommitted(8)

	// Reconnect both other servers, we'll have quorum now.
	rc.ReconnectPeers(dPeer1)
	rc.ReconnectPeers(dPeer2)
	sleepMs(600)

	// 8 is still not committed because the term has changed.
	rc.CheckNotCommitted(8)

	// A new leader will be elected. It could be a different leader, even though
	// the original's log is longer, because the two reconnected peers can elect
	// each other.
	newLeaderId, againTerm := rc.CheckSingleLeader()
	if origTerm == againTerm {
		t.Errorf("got origTerm==againTerm==%d; want them different", origTerm)
	}

	// But new values will be committed for sure...
	rc.Submit(newLeaderId, 9)
	sleepMs(50)
	rc.Submit(newLeaderId, 10)
	sleepMs(50)
	rc.Submit(newLeaderId, 11)
	sleepMs(350)

	for _, v := range []int{9, 10, 11} {
		rc.CheckCommittedN(v, 3)
	}
}

func TestCommitsWithLeaderDisconnects(t *testing.T) {
	//defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	rc := NewRaftCluster(5, t)
	//defer rc.Shutdown()

	// Submit a couple of values to a fully connected cluster.
	origLeaderId, _ := rc.CheckSingleLeader()
	rc.Submit(origLeaderId, 5)
	sleepMs(50)
	rc.Submit(origLeaderId, 6)

	sleepMs(300)
	rc.CheckCommittedN(6, 5)

	// Leader disconnected...
	rc.DisconnectPeers(origLeaderId)
	sleepMs(10)

	// Submit 7 to original leader, even though it's disconnected.
	rc.Submit(origLeaderId, 7)

	sleepMs(300)
	rc.CheckNotCommitted(7)

	newLeaderId, _ := rc.CheckSingleLeader()

	// Submit 8 to new leader.
	rc.Submit(newLeaderId, 8)
	sleepMs(300)
	rc.CheckCommittedN(8, 4)

	// Reconnect old leader and let it settle. The old leader shouldn't be the one
	// winning.
	rc.ReconnectPeers(origLeaderId)
	sleepMs(600)

	finalLeaderId, _ := rc.CheckSingleLeader()
	if finalLeaderId == origLeaderId {
		t.Errorf("got finalLeaderId==origLeaderId==%d, want them different", finalLeaderId)
	}

	// Submit 9 and check it's fully committed.
	rc.Submit(newLeaderId, 9)
	sleepMs(150)
	rc.CheckCommittedN(9, 5)
	sleepMs(150)
	rc.CheckCommittedN(8, 5)
	sleepMs(150)

	// But 7 is not committed...
	rc.CheckNotCommitted(7)
}
