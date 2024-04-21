package main

import distributed_lock "github.com/lquyet/distributed-lock"

func main() {
	_ = distributed_lock.NewRaftCluster(3)
	c := make(chan struct{})
	<-c
}
