package distributed_lock

import (
	"fmt"
	"github.com/spf13/viper"
	"strings"
)

type ServerConfig struct {
	Address []string `mapstructure:"address"`
}

type ClientConfig struct {
	ClientId string `mapstructure:"id"`
}

type ParamsConfig struct {
	GrpcTimeout int `mapstructure:"grpc_timeout"`
}

type Config struct {
	Servers ServerConfig `mapstructure:"servers"`
	Client  ClientConfig `mapstructure:"client"`
	Params  ParamsConfig `mapstructure:"params"`
}

// Load config from config.yml
func Load() Config {
	vip := viper.New()

	vip.SetConfigName("config")
	vip.SetConfigType("yml")
	vip.AddConfigPath(".")

	return loadConfigWithViper(vip)
}

func loadConfigWithViper(vip *viper.Viper) Config {
	vip.SetEnvPrefix("docker")
	vip.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	vip.AutomaticEnv()

	err := vip.ReadInConfig()
	if err != nil {
		panic(err)
	}

	// workaround https://github.com/spf13/viper/issues/188#issuecomment-399518663
	// to allow read from environment variables when Unmarshal
	for _, key := range vip.AllKeys() {
		val := vip.Get(key)
		vip.Set(key, val)
	}

	fmt.Println("Config file used:", vip.ConfigFileUsed())

	cfg := Config{}
	err = vip.Unmarshal(&cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}
