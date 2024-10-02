package mcasdoor

import (
	"fmt"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type CasdoorConnection struct {
	Conf      *casdoorsdk.AuthConfig
	Client    *casdoorsdk.Client
	Connected bool
}

func (cc *CasdoorConnection) Connect() *casdoorsdk.Client {
	cc.Client = casdoorsdk.NewClientWithConf(cc.Conf)
	cc.Connected = true

	fmt.Println("Connected to casdoor ✅ ")

	return cc.Client
}

func (cc *CasdoorConnection) GetClient() *casdoorsdk.Client {
	if cc.Client == nil {
		cc.Connect()
	}

	return cc.Client
}
