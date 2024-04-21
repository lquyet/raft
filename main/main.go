package main

import distributed_lock "github.com/lquyet/distributed-lock"

func main() {
	ready := make(chan interface{})
	server := distributed_lock.NewServer(1, []int32{}, map[int32]string{}, ready, "localhost:8080")
	server.Serve()
}
