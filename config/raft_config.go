package config

type RaftConfig struct {
	ElectionTimeoutBase int `mapstructure:"election_timeout_base"`
	HeartbeatInterval   int `mapstructure:"heartbeat_interval"`
}
