package environment

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type Env struct {
	ClientID     string `mapstructure:"CLIENT_ID"`
	ClientSecret string `mapstructure:"CLIENT_SECRET"`
	URLAPIAuth   string `mapstructure:"URL_API_AUTH"`
}

func LoadEnv() (Env, error) {
	viper.AutomaticEnv()

	if _, err := os.Stat(".env"); err == nil {
		viper.SetConfigFile(".env")

		if err := viper.ReadInConfig(); err != nil {
			fmt.Printf("Error reading config file: %s\n", err)
			return Env{}, err
		}
	}

	e := Env{}
	if err := viper.Unmarshal(&e); err != nil {
		return Env{}, err
	}

	return e, nil
}
