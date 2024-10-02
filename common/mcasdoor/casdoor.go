package mcasdoor

import (
	_ "embed"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"log"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

//go:embed certificates/token_jwt_key.pem
var jwtPKCertificate []byte

type CasdoorConnection struct {
	Endpoint         string
	ClientId         string
	ClientSecret     string
	Certificate      string
	OrganizationName string
	ApplicationName  string
	JWKUri           string
	Connected        bool
	Client           *casdoorsdk.Client
}

func (cc *CasdoorConnection) Connect() error {
	fmt.Println("Connecting to casdoor...")

	if len(jwtPKCertificate) == 0 {
		err := errors.New("public Key Certificate isn't load")
		log.Fatal("public Key Certificate isn't load", zap.Error(err))
		return err
	}

	conf := &casdoorsdk.AuthConfig{
		Endpoint:         cc.Endpoint,
		ClientId:         cc.ClientId,
		ClientSecret:     cc.ClientSecret,
		Certificate:      string(jwtPKCertificate[:]),
		OrganizationName: cc.OrganizationName,
		ApplicationName:  cc.ApplicationName,
	}

	client := casdoorsdk.NewClientWithConf(conf)
	if client != nil {
		fmt.Println("Connected to casdoor ✅ ")
		cc.Connected = true
	}

	cc.Client = client

	return nil
}

func (cc *CasdoorConnection) GetClient() (*casdoorsdk.Client, error) {
	if cc.Client == nil {
		if err := cc.Connect(); err != nil {
			log.Printf("ERRCONECT %s", err)
			return nil, err
		}

	}

	return cc.Client, nil
}
