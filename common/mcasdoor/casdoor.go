package mcasdoor

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

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
	if client == nil || !cc.healthCheck() {
		cc.Connected = false
		err := errors.New("can't connect casdoor")
		log.Printf("CasdoorConnection.Ping %v", zap.Error(err))

		return err
	}

	fmt.Println("Connected to casdoor ✅ ")
	cc.Connected = true
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

func (cc *CasdoorConnection) healthCheck() bool {
	url := fmt.Sprintf("%s/api/health", cc.Endpoint)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Errorf("failed to make GET request: %w", err.Error())
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Errorf("failed to read response body: %w", err.Error())
		return false
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		fmt.Errorf("failed to unmarshal response: %w", err.Error())
		return false
	}

	if status, ok := result["status"].(string); ok && status == "ok" {
		return true
	}

	fmt.Errorf("casdoor unhealthy")
	return false
}
