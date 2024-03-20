package main

import (
	distributed_lock "github.com/lquyet/distributed-lock"
)

func main() {
	sampleNode := &distributed_lock.RaftNode{}
	_, baseServer := distributed_lock.NewGrpcLockServer(sampleNode)
	distributed_lock.StartServer(baseServer)
}
