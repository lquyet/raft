package distributed_lock

import (
	proto "github.com/lquyet/distributed-lock/pb"
	"google.golang.org/grpc"
	"net"
	"sync"
)

// Server holds references to set up a Raft Node
type Server struct {
	mu sync.Mutex

	id          int32                             // Node identifier
	peerIds     []int32                           // List of peers in cluster
	peerClients map[int32]proto.RaftServiceClient // RPC clients to peers

	raftModule *RaftModule // The consensus module

	listener net.Listener

	grpcServer *grpc.Server

	proto.UnimplementedRaftServiceServer
}

func NewServer() *Server {
	return &Server{}
}
