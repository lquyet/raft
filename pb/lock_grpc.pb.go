// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v3.11.4
// source: lock.proto

package proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

const (
	LockService_AcquireLock_FullMethodName = "/proto.LockService/AcquireLock"
	LockService_ReleaseLock_FullMethodName = "/proto.LockService/ReleaseLock"
)

// LockServiceClient is the client API for LockService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type LockServiceClient interface {
	AcquireLock(ctx context.Context, in *AcquireLockRequest, opts ...grpc.CallOption) (*AcquireLockResponse, error)
	ReleaseLock(ctx context.Context, in *ReleaseLockRequest, opts ...grpc.CallOption) (*ReleaseLockResponse, error)
}

type lockServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewLockServiceClient(cc grpc.ClientConnInterface) LockServiceClient {
	return &lockServiceClient{cc}
}

func (c *lockServiceClient) AcquireLock(ctx context.Context, in *AcquireLockRequest, opts ...grpc.CallOption) (*AcquireLockResponse, error) {
	out := new(AcquireLockResponse)
	err := c.cc.Invoke(ctx, LockService_AcquireLock_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *lockServiceClient) ReleaseLock(ctx context.Context, in *ReleaseLockRequest, opts ...grpc.CallOption) (*ReleaseLockResponse, error) {
	out := new(ReleaseLockResponse)
	err := c.cc.Invoke(ctx, LockService_ReleaseLock_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LockServiceServer is the server API for LockService service.
// All implementations must embed UnimplementedLockServiceServer
// for forward compatibility
type LockServiceServer interface {
	AcquireLock(context.Context, *AcquireLockRequest) (*AcquireLockResponse, error)
	ReleaseLock(context.Context, *ReleaseLockRequest) (*ReleaseLockResponse, error)
	mustEmbedUnimplementedLockServiceServer()
}

// UnimplementedLockServiceServer must be embedded to have forward compatible implementations.
type UnimplementedLockServiceServer struct {
}

func (UnimplementedLockServiceServer) AcquireLock(context.Context, *AcquireLockRequest) (*AcquireLockResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AcquireLock not implemented")
}
func (UnimplementedLockServiceServer) ReleaseLock(context.Context, *ReleaseLockRequest) (*ReleaseLockResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReleaseLock not implemented")
}
func (UnimplementedLockServiceServer) mustEmbedUnimplementedLockServiceServer() {}

// UnsafeLockServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to LockServiceServer will
// result in compilation errors.
type UnsafeLockServiceServer interface {
	mustEmbedUnimplementedLockServiceServer()
}

func RegisterLockServiceServer(s grpc.ServiceRegistrar, srv LockServiceServer) {
	s.RegisterService(&LockService_ServiceDesc, srv)
}

func _LockService_AcquireLock_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AcquireLockRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LockServiceServer).AcquireLock(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: LockService_AcquireLock_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LockServiceServer).AcquireLock(ctx, req.(*AcquireLockRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _LockService_ReleaseLock_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReleaseLockRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LockServiceServer).ReleaseLock(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: LockService_ReleaseLock_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LockServiceServer).ReleaseLock(ctx, req.(*ReleaseLockRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// LockService_ServiceDesc is the grpc.ServiceDesc for LockService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var LockService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.LockService",
	HandlerType: (*LockServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "AcquireLock",
			Handler:    _LockService_AcquireLock_Handler,
		},
		{
			MethodName: "ReleaseLock",
			Handler:    _LockService_ReleaseLock_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "lock.proto",
}
