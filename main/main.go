package main

import (
	"fmt"
	raft "github.com/lquyet/raft"
)

func main() {
	rc := raft.NewRaftCluster(3, nil)
	c := make(chan struct{})
	<-c
	fmt.Println(rc)
}
