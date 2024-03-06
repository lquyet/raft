package distributed_lock

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLock(t *testing.T) {
	result, err := Lock("test")
	assert.Equal(t, true, result)
	assert.Nil(t, err)
}
