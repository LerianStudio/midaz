package mcasdoor

import (
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"go.uber.org/zap"
	"io"
	"net/http"
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
	if client == nil || !cc.healthCheck() {
		cc.Connected = false
		err := errors.New("can't connect casdoor")
		cc.Logger.Fatalf("Casdoor.HealthCheck %v", zap.Error(err))

		return err
	}

	cc.Logger.Info("Connected to casdoor ✅ ")
	cc.Connected = true
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

func (cc *CasdoorConnection) healthCheck() bool {
	resp, err := http.Get(cc.Endpoint + "/api/health")

	if err != nil {
		cc.Logger.Errorf("failed to make GET request: %v", err.Error())

		return false
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		cc.Logger.Errorf("failed to read response body: %v", err.Error())

		return false
	}

	result := make(map[string]any)

	err = json.Unmarshal(body, &result)
	if err != nil {
		cc.Logger.Errorf("failed to unmarshal response: %v", err.Error())

		return false
	}

	if status, ok := result["status"].(string); ok && status == "ok" {
		return true
	}

	cc.Logger.Error("casdoor unhealthy...")

	return false
}
