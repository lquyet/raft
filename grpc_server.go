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

type GrpcLockServer struct {
	proto.UnimplementedLockServiceServer
	// node the reference to its own raft node to do consensus
	raftNode *RaftNode
}

func (g *GrpcLockServer) AcquireLock(ctx context.Context, request *proto.AcquireLockRequest) (*proto.AcquireLockResponse, error) {
	fmt.Println("Got AcquireLock request")
	return &proto.AcquireLockResponse{LockId: 1}, nil
}

func (g *GrpcLockServer) ReleaseLock(ctx context.Context, request *proto.ReleaseLockRequest) (*proto.ReleaseLockResponse, error) {
	fmt.Println("Got ReleaseLock request")
	return &proto.ReleaseLockResponse{LockId: 1}, nil
}

func NewGrpcLockServer(raftNode *RaftNode) (*GrpcLockServer, *grpc.Server) {
	baseServer := grpc.NewServer()
	gLockServer := &GrpcLockServer{raftNode: raftNode}
	proto.RegisterLockServiceServer(baseServer, gLockServer)
	return gLockServer, baseServer
}

func StartServer(baseServer *grpc.Server) {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Should use config variable for listen string
		listener, err := net.Listen("tcp", "localhost:8000")
		if err != nil {
			panic(err)
		}
		fmt.Println("gRPC Server listens at", listener.Addr().String())
		err = baseServer.Serve(listener)
		if err != nil {
			panic(err)
		}
		fmt.Println("Shutdown gRPC server successfully")
	}()

	//--------------------------------
	// Graceful Shutdown
	//--------------------------------
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	baseServer.GracefulStop()
	wg.Wait()
}
