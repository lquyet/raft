package distributed_lock

import (
	"context"
	"fmt"
	proto "github.com/lquyet/distributed-lock/pb"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Server holds references to set up a Raft Node
type Server struct {
	mu sync.Mutex

	id          int32                             // Node identifier
	addr        string                            // Address of this server
	peerIds     []int32                           // List of peers in cluster
	peerAddrs   map[int32]string                  // Map of peer IDs to network addresses
	peerClients map[int32]proto.RaftServiceClient // RPC clients to peers

	ready <-chan interface{} // Channel to signal when the server is ready to start

	raftModule *RaftModule // The consensus module

	listener net.Listener

	grpcServer *grpc.Server

	proto.UnimplementedRaftServiceServer
}

func NewServer(serverId int32, peerIds []int32, peerAddrs map[int32]string, ready <-chan interface{}, addr string) *Server {
	s := Server{}
	s.id = serverId
	s.peerIds = peerIds
	s.peerAddrs = peerAddrs
	s.ready = ready
	s.addr = addr
	return &s
}

func (s *Server) Serve() {
	s.mu.Lock()
	s.raftModule = NewRaftModule(s.id, s.peerIds, s, s.ready)
	s.mu.Unlock()

	s.grpcServer = grpc.NewServer()
	proto.RegisterRaftServiceServer(s.grpcServer, s)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		listener, err := net.Listen("tcp", s.addr)
		if err != nil {
			panic(err)
		}

		fmt.Println("Server started at", s.addr)
		err = s.grpcServer.Serve(listener)
		if err != nil {
			panic(err)
		}
		fmt.Println("Shutdown gRPC server successfully")
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	s.grpcServer.GracefulStop()

	wg.Wait()
}
