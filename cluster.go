package distributed_lock

import "fmt"

type RaftCluster struct {
	Cluster      []*Server
	NumOfServers int
	Connected    []bool
}

func NewRaftCluster(n int) *RaftCluster {
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
		ns[i] = NewServer(int32(i), peerIds, peerAddrs, ready, "localhost:808"+fmt.Sprintf("%d", i))
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

	close(ready)

	raftCluster := RaftCluster{
		Cluster:      ns,
		NumOfServers: n,
		Connected:    connected,
	}

	return &raftCluster
}
