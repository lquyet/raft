package main

import (
	"fmt"
	"github.com/lquyet/raft"
	"time"
)

// KVStorageEngine represents a simple state machine that store committed entries with key-value format
type KVStorageEngine struct {
	database map[string]string
}

// Interface assertion to make sure that KVStorageEngine implement StateMachine interface
var _ KVStorageEngine = KVStorageEngine{}

func (e *KVStorageEngine) Apply(entry raft.CommitEntry) error {
	fmt.Println("Apply to state machine")
	return nil
}

func (e *KVStorageEngine) Set(key string, value string) error {
	e.database[key] = value
	return nil
}

func (e *KVStorageEngine) Get(key string) (string, error) {
	if val, ok := e.database[key]; ok {
		return val, nil
	}

	return "", fmt.Errorf("key not found in database: %s", key)
}

func NewKVStorageEngine() raft.StateMachine {
	return &KVStorageEngine{}
}

func main() {
	// Create a state machine
	stateMachine := NewKVStorageEngine()

	cluster := raft.NewRaftCluster(3, stateMachine, nil)

	origLeaderId, _ := cluster.CheckSingleLeader()

	_ = cluster.Submit(origLeaderId, "42")

	time.Sleep(5 * time.Second)

	t := make(chan int)
	<-t
}
