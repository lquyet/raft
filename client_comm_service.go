package distributed_lock

import (
	"context"
	"fmt"
	proto "github.com/lquyet/distributed-lock/pb"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type ClientCommService struct {
	ch chan string
	proto.UnimplementedClientServiceServer
	proto.UnimplementedRequestServiceServer
}

func (g *ClientCommService) SendRequest(ctx context.Context, request *proto.Req) (*proto.Resp, error) {
	fmt.Println("----- Got SendRequest request -----")
	g.ch <- "SendRequest: " + request.GetKey() + " | " + request.GetValue()
	return &proto.Resp{Status: "done"}, nil
}

func (g *ClientCommService) AcquireLock(ctx context.Context, request *proto.AcquireLockRequest) (*proto.AcquireLockResponse, error) {
	fmt.Println("----- Got AcquireLock request -----")
	fmt.Println("LockId: ", request.LockId)

	g.ch <- "Acquire: " + strconv.Itoa(int(request.GetLockId()))
	return &proto.AcquireLockResponse{LockId: 1}, nil
}

func (g *ClientCommService) ReleaseLock(ctx context.Context, request *proto.ReleaseLockRequest) (*proto.ReleaseLockResponse, error) {
	fmt.Println("----- Got ReleaseLock request -----")
	fmt.Println("LockId: ", request.LockId)

	g.ch <- "Release: " + strconv.Itoa(int(request.GetLockId()))
	return &proto.ReleaseLockResponse{LockId: 1}, nil
}

func NewClientCommService(queue chan string) *ClientCommService {
	return &ClientCommService{
		ch: queue,
	}
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
