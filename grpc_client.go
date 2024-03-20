package distributed_lock

import (
	"context"
	proto "github.com/lquyet/distributed-lock/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"time"
)

type GrpcLockClient struct {
	// nodeAddress is the address of a particular node that this client keeps the connection to
	nodeAddress string
	conn        *grpc.ClientConn
	timeout     uint32
	// Should add logger later below
}

func NewGrpcLockClient(address string, timeout uint32) *GrpcLockClient {
	return &GrpcLockClient{
		nodeAddress: address,
		timeout:     timeout,
	}
}

func (l *GrpcLockClient) IsReady() bool {
	if l.conn != nil {
		return l.conn.GetState() == connectivity.Ready
	}
	return false
}

func (l *GrpcLockClient) Connect() error {
	conn, err := grpc.Dial(l.nodeAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	l.conn = conn
	return nil
}

func (l *GrpcLockClient) SendAcquireLockRequest(request *proto.AcquireLockRequest) (*proto.AcquireLockResponse, error) {
	if !l.IsReady() {
		err := l.Connect()
		if err != nil {
			// Should do some retry logic here
			return nil, err
		}
	}

	grpcExternalClient := proto.NewLockServiceClient(l.conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(l.timeout)*time.Millisecond)
	defer cancel()

	res, err := grpcExternalClient.AcquireLock(ctx, request)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (l *GrpcLockClient) SendReleaseLockRequest(request *proto.ReleaseLockRequest) (*proto.ReleaseLockResponse, error) {
	if !l.IsReady() {
		err := l.Connect()
		if err != nil {
			// Should do some retry logic here
			return nil, err
		}
	}

	grpcExternalClient := proto.NewLockServiceClient(l.conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(l.timeout)*time.Millisecond)
	defer cancel()

	res, err := grpcExternalClient.ReleaseLock(ctx, request)
	if err != nil {
		return nil, err
	}

	return res, nil
}
