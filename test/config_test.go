package test

import (
	"fmt"
	"github.com/lquyet/distributed-lock"
	"testing"
)

func TestConfig(t *testing.T) {
	cfg := distributed_lock.Load()
	fmt.Print(cfg)
}
