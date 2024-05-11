package raft

type StateMachine interface {
	Apply(entry CommitEntry) error
}
