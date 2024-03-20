package distributed_lock

import (
	"fmt"
	"testing"
)

func TestConfig(t *testing.T) {
	cfg := Load()
	fmt.Print(cfg)
}
