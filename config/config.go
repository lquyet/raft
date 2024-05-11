package config

type Config struct {
	Server ServerConfig `mapstructure:"server"`
	Raft   RaftConfig   `mapstructure:"raft"`
	Mode   string       `mapstructure:"mode"`
}
