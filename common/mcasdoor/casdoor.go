package mcasdoor

import (
	_ "embed"
	"errors"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"go.uber.org/zap"
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
	Logger           mlog.Logger
}

func (cc *CasdoorConnection) Connect() error {
	cc.Logger.Info("Connecting to casdoor...")

	if len(jwtPKCertificate) == 0 {
		err := errors.New("public key certificate isn't load")
		cc.Logger.Fatalf("public key certificate isn't load. error: %v", zap.Error(err))

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
		cc.Logger.Info("Connected to casdoor ✅ \n")

		cc.Connected = true
	}

	cc.Client = client

	return nil
}

func (cc *CasdoorConnection) GetClient() (*casdoorsdk.Client, error) {
	if cc.Client == nil {
		if err := cc.Connect(); err != nil {
			cc.Logger.Infof("ERRCONECT %s", err)

			return nil, err
		}
	}

	return cc.Client, nil
}
