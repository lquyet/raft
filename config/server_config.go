package config

import (
	"fmt"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Id            int32            `mapstructure:"id"`
	Address       string           `mapstructure:"address"`
	PeerIds       []int32          `mapstructure:"peerIds"`
	PeerAddresses map[int32]string `mapstructure:"peerAddresses"`
}

func Load(cfgName string, cfgType string) Config {
	vip := viper.New()
	vip.SetConfigName(cfgName)
	vip.SetConfigType(cfgType)
	vip.AddConfigPath(".")

	return loadConfigWithViper(vip)
}

func loadConfigWithViper(vip *viper.Viper) Config {
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
