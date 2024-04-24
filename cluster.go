package distributed_lock

type RaftCluster struct {
	Cluster      []*Server
	NumOfServers int
}

func NewRaftCluster(n int) {
	ns := make([]*Server, n)
	ready := make(chan interface{})

	for i := 0; i < n; i++ {
		peerIds := make([]int, 0)
		for p := 0; p < n; p++ {
			if p != i {
				peerIds = append(peerIds, p)
			}
		}
	}
}
