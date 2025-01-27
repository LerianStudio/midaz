package mfusionauth

import (
	"encoding/json"
	"errors"
	"github.com/FusionAuth/go-client/pkg/fusionauth"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/url"
	"time"
)

type FusionAuthConnection struct {
	BaseUrl   string
	APIKey    string
	JWKSUrl   string
	Timeout   string
	Connected bool
	Client    *fusionauth.FusionAuthClient
	Logger    mlog.Logger
}

func (fc *FusionAuthConnection) Connect() error {
	fc.Logger.Info("Connecting to FusionAuth...")

	timeout, err := time.ParseDuration(fc.Timeout + "s")
	if err != nil {
		fc.Logger.Warnf("Invalid timeout duration: %v, using default 30s", zap.Error(err))
		timeout = 30 * time.Second
	}

	httpClient := &http.Client{
		Timeout: timeout,
	}

	baseUrl, _ := url.Parse(fc.BaseUrl)

	client := fusionauth.NewClient(httpClient, baseUrl, fc.APIKey)
	if client == nil || !fc.healthCheck() {
		fc.Connected = false
		err := errors.New("can't connect FusionAuth")
		fc.Logger.Fatalf("FusionAuth.HealthCheck %v", zap.Error(err))

		return err
	}

	fc.Logger.Info("Connected to FusionAuth ✅ ")
	fc.Connected = true
	fc.Client = client

	return nil
}

func (fc *FusionAuthConnection) GetClient() (*fusionauth.FusionAuthClient, error) {
	if fc.Client == nil {
		if err := fc.Connect(); err != nil {
			fc.Logger.Infof("ERRCONECT %s", err)

			return nil, err
		}
	}

	return fc.Client, nil
}

func (fc *FusionAuthConnection) healthCheck() bool {
	resp, err := http.Get(fc.BaseUrl + "/api/status")

	if err != nil {
		fc.Logger.Errorf("failed to make GET request: %v", err.Error())

		return false
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fc.Logger.Errorf("failed to read response body: %v", err.Error())

		return false
	}

	result := make(map[string]any)

	err = json.Unmarshal(body, &result)
	if err != nil {
		fc.Logger.Errorf("failed to unmarshal response: %v", err.Error())

		return false
	}

	if status, ok := result["status"].(string); ok && status == "ok" {
		return true
	}

	fc.Logger.Error("fusionauth unhealthy...")

	return false
}
