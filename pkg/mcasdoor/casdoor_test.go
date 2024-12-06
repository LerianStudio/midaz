package mcasdoor

import (
	_ "embed"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCasdoorConnection_Connect(t *testing.T) {
	t.Run("should return error when jwtPKCertificate is empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)

		mockLogger.EXPECT().
			Info("Connecting to casdoor...").
			Times(1)

		mockLogger.EXPECT().
			Fatalf(
				"public key certificate isn't load. error: %v",
				gomock.Any(),
			).
			Times(1)

		cc := &CasdoorConnection{
			Logger: mockLogger,
		}

		jwtPKCertificate = []byte("")

		err := cc.Connect()

		assert.NotNil(t, err)
		assert.EqualError(t, err, "public key certificate isn't load")
	})

	t.Run("should return error when healthCheck fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)

		mockLogger.EXPECT().
			Info("Connecting to casdoor...").
			Times(1)

		mockLogger.EXPECT().
			Error("casdoor unhealthy...").
			Times(1)

		mockLogger.EXPECT().
			Fatalf(
				"Casdoor.HealthCheck %v",
				gomock.Any(),
			).
			Times(1)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"status": "error"}`))
		}))
		defer server.Close()

		cc := &CasdoorConnection{
			Logger:           mockLogger,
			Endpoint:         server.URL,
			ClientID:         "test-id",
			ClientSecret:     "test-secret",
			OrganizationName: "org",
			ApplicationName:  "app",
		}

		jwtPKCertificate = []byte("valid-cert")

		err := cc.Connect()

		assert.NotNil(t, err)
		assert.EqualError(t, err, "can't connect casdoor")
	})

	t.Run("should connect successfully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)

		mockLogger.EXPECT().
			Info("Connecting to casdoor...").
			Times(1)

		mockLogger.EXPECT().
			Info("Connected to casdoor ✅ ").
			Times(1)

		// Mock do servidor de healthCheck retornando sucesso
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		}))
		defer server.Close()

		cc := &CasdoorConnection{
			Logger:           mockLogger,
			Endpoint:         server.URL, // Usar o servidor mockado
			ClientID:         "test-id",
			ClientSecret:     "test-secret",
			OrganizationName: "org",
			ApplicationName:  "app",
		}

		// Configurar certificado válido
		jwtPKCertificate = []byte("valid-cert")

		// Criar cliente
		cc.Client = &casdoorsdk.Client{}

		err := cc.Connect()

		assert.Nil(t, err)
		assert.True(t, cc.Connected)
		assert.NotNil(t, cc.Client)
	})
}

func TestCasdoorConnection_GetClient(t *testing.T) {
	t.Run("should return error when Connect fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)

		originalJwtPKCertificate := jwtPKCertificate
		defer func() { jwtPKCertificate = originalJwtPKCertificate }()
		jwtPKCertificate = []byte("")

		cc := &CasdoorConnection{
			Logger: mockLogger,
			Client: nil,
		}

		gomock.InOrder(
			mockLogger.EXPECT().Info("Connecting to casdoor...").Times(1),
			mockLogger.EXPECT().Fatalf("public key certificate isn't load. error: %v", gomock.Any()).Times(1),
			mockLogger.EXPECT().Infof("ERRCONECT %s", gomock.Any()).Times(1),
		)

		client, err := cc.GetClient()

		assert.Nil(t, client)
		assert.EqualError(t, err, "public key certificate isn't load")
	})

	t.Run("should return existing client when already connected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)

		existingClient := &casdoorsdk.Client{}
		cc := &CasdoorConnection{
			Logger: mockLogger,
			Client: existingClient,
		}

		client, err := cc.GetClient()

		assert.Nil(t, err)
		assert.Equal(t, existingClient, client)
	})
}

func TestCasdoorConnection_healthCheck(t *testing.T) {
	t.Run("should return false when GET request fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)
		mockLogger.EXPECT().
			Errorf("failed to make GET request: %v", gomock.Any()).
			Times(1)

		cc := &CasdoorConnection{
			Logger:   mockLogger,
			Endpoint: "http://invalid-url",
		}

		result := cc.healthCheck()
		assert.False(t, result)
	})

	t.Run("should return false when response JSON is invalid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)
		mockLogger.EXPECT().
			Errorf("failed to unmarshal response: %v", gomock.Any()).
			Times(1)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("invalid-json"))
		}))
		defer server.Close()

		cc := &CasdoorConnection{
			Logger:   mockLogger,
			Endpoint: server.URL,
		}

		result := cc.healthCheck()
		assert.False(t, result)
	})

	t.Run("should return false when status is not ok", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)
		mockLogger.EXPECT().
			Error("casdoor unhealthy...").
			Times(1)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "error"}`))
		}))
		defer server.Close()

		cc := &CasdoorConnection{
			Logger:   mockLogger,
			Endpoint: server.URL,
		}

		result := cc.healthCheck()
		assert.False(t, result)
	})

	t.Run("should return true when status is ok", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		}))
		defer server.Close()

		cc := &CasdoorConnection{
			Logger:   mockLogger,
			Endpoint: server.URL,
		}

		result := cc.healthCheck()
		assert.True(t, result)
	})
}
