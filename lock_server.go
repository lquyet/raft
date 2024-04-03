package distributed_lock

import (
	"context"
	"log"
	"net"

	raft "github.com/lquyet/distributed-lock/pb"
	"google.golang.org/grpc"
)

// Server holds references to set up a Raft Node
type Server struct {
	id    uint32
	s     *grpc.Server
	lis   net.Listener // underlying connection to gRPC Server
	c     raft.RaftServiceClient
	r     *RaftService
	rc    *ClientCommService
	ready chan bool // only required to hold off starting server until Serve is called

}

// New is used to initialise a new server
func New(addr string, join string, id uint32, et uint32, hb uint32, t uint32) *Server {
	// For node-node communication
	s := new(Server)
	s.ready = make(chan bool)
	s.id = id
	s.s = s.newServer(addr)
	if len(join) > 0 {
		s.c = s.newClient(join)
	}
	s.r = NewRaftService(id, &s.c, et, hb, t, s.ready)
	raft.RegisterRaftServiceServer(s.s, s.r)

	// For client-node communication
	rc := NewClientCommService()
	raft.RegisterLockServiceServer(s.s, rc)

	return s
}

func (s *Server) newServer(addr string) *grpc.Server {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Panicf("server failed to listen: %v", err)
	}

	s.lis = lis

	return grpc.NewServer()
}

// Serve is used to start serving requests and join (if address is provided) the cluster afterwards
func (s *Server) Serve(join string) {
	log.Printf("Node-%v starting on %v", s.id, s.lis.Addr())
	s.ready <- true

	if join != "" {
		go s.join(join)
	}

	if err := s.s.Serve(s.lis); err != nil {
		log.Fatalf("server failed to server: %v", err)
	}
}

func (s *Server) newClient(join string) raft.RaftServiceClient {
	conn, err := grpc.Dial(join, grpc.WithInsecure())
	if err != nil {
		log.Panicf("client failed to listen: %v", err)
	}

	client := raft.NewRaftServiceClient(conn)
	return client
}

func (s *Server) join(join string) {
	if resp, err := s.c.JoinCluster(context.Background(), &raft.JoinClusterRequest{
		Id:   s.id,
		Addr: s.lis.Addr().String(),
	}); err != nil {
		log.Printf("Node-%v error joining address %v: %v", s.id, join, err)
	} else {
		s.r.AddPeer(resp.GetId(), join)
		s.joinHelper(resp.Ids, resp.Addrs)
	}
}

func (s *Server) joinHelper(ids []uint32, addrs []string) {
	for i := 0; i < len(ids); i++ {
		// If the ids is not the Node's own, handshake with peer
		if s.id != ids[i] {
			s.r.AddPeer(ids[i], addrs[i])
			if peer, prs := s.r.Peers[ids[i]]; prs {
				peer.Client.JoinCluster(context.Background(), &raft.JoinClusterRequest{
					Id:   s.id,
					Addr: s.lis.Addr().String(),
				})
			}
		}
	}
}
