package main

import "github.com/lquyet/distributed-lock"

func main() {
	server := distributed_lock.NewServer(1, []int32{}, map[int32]string{}, make(chan interface{}), "localhost:8080")
	server.Serve()
}
