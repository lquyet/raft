package main

import (
	"fmt"
	distributed_lock "github.com/lquyet/distributed-lock"
)

func main() {
	rc := distributed_lock.NewRaftCluster(3)
	c := make(chan struct{})
	<-c
	fmt.Println(rc)
}
