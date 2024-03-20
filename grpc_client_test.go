package distributed_lock

import (
	proto "github.com/lquyet/distributed-lock/pb"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestClient(t *testing.T) {
	client := NewGrpcLockClient("localhost:8000", 1000000)
	_, err := client.SendAcquireLockRequest(&proto.AcquireLockRequest{LockId: int32(1)})
	_, err = client.SendReleaseLockRequest(&proto.ReleaseLockRequest{LockId: int32(1)})
	assert.Nil(t, err)
}
