package distributed_lock

import (
	"context"
	"time"
)

type ILockClientAPI interface {
	AcquireLock(ctx context.Context, resource string, ttl time.Duration) error
	ReleaseLock(ctx context.Context, resource string, ttl time.Duration) error
	RefreshLock(ctx context.Context, resource string, ttl time.Duration) error
}

type lockClient struct {
}

func (l lockClient) AcquireLock(ctx context.Context, resource string, ttl time.Duration) error {
	//TODO implement me
	panic("implement me")
}

func (l lockClient) ReleaseLock(ctx context.Context, resource string, ttl time.Duration) error {
	//TODO implement me
	panic("implement me")
}

func (l lockClient) RefreshLock(ctx context.Context, resource string, ttl time.Duration) error {
	//TODO implement me
	panic("implement me")
}

var _ ILockClientAPI = &lockClient{}

func NewLockClient(target string) ILockClientAPI {
	return &lockClient{}
}
