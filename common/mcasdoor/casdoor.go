package mcasdoor

import (
	_ "embed"
	"errors"
	"fmt"
	"log"

	"go.uber.org/zap"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

//go:embed certificates/token_jwt_key.pem
var jwtPKCertificate []byte

type CasdoorConnection struct {
	Endpoint         string
	ClientID         string
	ClientSecret     string
	Certificate      string
	OrganizationName string
	ApplicationName  string
	EnforcerName     string
	JWKUri           string
	Connected        bool
	Client           *casdoorsdk.Client
}

func (cc *CasdoorConnection) Connect() error {
	fmt.Println("Connecting to casdoor...")

	if len(jwtPKCertificate) == 0 {
		err := errors.New("public key certificate isn't load")
		log.Fatal("public key certificate isn't load", zap.Error(err))

		return err
	}

	conf := &casdoorsdk.AuthConfig{
		Endpoint:         cc.Endpoint,
		ClientId:         cc.ClientID,
		ClientSecret:     cc.ClientSecret,
		Certificate:      string(jwtPKCertificate),
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
