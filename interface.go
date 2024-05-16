package distributed_lock

// StateMachine is an interface representing a particular service defined by users
// In order to achieve the simplicity in our architecture, we only require 1 method `Apply` to be implemented
type StateMachine interface {
	Apply(command interface{}) error
}
