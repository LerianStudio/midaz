package mgrpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/stretchr/testify/assert"

	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestGRPCConnection_Connect(t *testing.T) {
	t.Run("should connect successfully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)

		mockLogger.EXPECT().
			Info("Connected to gRPC ✅ ").
			Times(1)

		// Mock do servidor gRPC
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := &GRPCConnection{
			Addr:   server.URL,
			Logger: mockLogger,
		}

		err := c.Connect()

		assert.Nil(t, err)
		assert.NotNil(t, c.Conn)
	})
}

func TestGRPCConnection_GetNewClient(t *testing.T) {
	t.Run("should return existing connection when already connected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)

		// Conexão simulada
		existingConn := &grpc.ClientConn{}

		c := &GRPCConnection{
			Conn:   existingConn,
			Logger: mockLogger,
		}

		conn, err := c.GetNewClient()

		assert.Nil(t, err)
		assert.Equal(t, existingConn, conn)
	})

	t.Run("should connect successfully and return connection", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockLogger := mlog.NewMockLogger(ctrl)

		mockLogger.EXPECT().
			Info("Connected to gRPC ✅ ").
			Times(1)

		// Mock do servidor gRPC
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := &GRPCConnection{
			Addr:   server.URL,
			Logger: mockLogger,
		}

		conn, err := c.GetNewClient()

		assert.Nil(t, err)
		assert.NotNil(t, conn)
	})
}

func TestGRPCConnection_ContextMetadataInjection(t *testing.T) {
	t.Run("should inject metadata and return a new context", func(t *testing.T) {
		ctx := context.Background()
		token := "test-token"

		c := &GRPCConnection{}
		newCtx := c.ContextMetadataInjection(ctx, token)

		md, ok := metadata.FromOutgoingContext(newCtx)
		assert.True(t, ok, "metadata should exist in context")

		assert.Equal(t, "Bearer "+token, md[constant.MDAuthorization][0])

		otelCtx := mopentelemetry.ExtractContext(newCtx)
		assert.NotNil(t, otelCtx, "context should have OpenTelemetry data")
	})
}
