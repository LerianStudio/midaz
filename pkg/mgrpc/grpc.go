package mgrpc

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// GRPCConnection is a struct which deal with gRPC connections.
type GRPCConnection struct {
	Addr   string
	Conn   *grpc.ClientConn
	Logger libLog.Logger
}

// Connect keeps a singleton connection with gRPC.
func (c *GRPCConnection) Connect() error {
	conn, err := grpc.NewClient(c.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		c.Logger.Error("Failed to connect on gRPC", zap.Error(err))
		return err
	}

	c.Logger.Info("Connected to gRPC ✅ ")

	c.Conn = conn

	return nil
}

// GetNewClient returns a connection to gRPC, reconnect it if necessary.
func (c *GRPCConnection) GetNewClient() (*grpc.ClientConn, error) {
	if c.Conn == nil {
		if err := c.Connect(); err != nil {
			log.Printf("ERRCONECT %s", err)
			return nil, err
		}
	}

	return c.Conn, nil
}

// ContextMetadataInjection injects OpenTelemetry trace context and optional authorization
// into the outgoing gRPC context. It preserves existing metadata and appends:
// - traceparent/tracestate (W3C propagated via OpenTelemetry)
// - authorization (JWT), when provided
func (c *GRPCConnection) ContextMetadataInjection(ctx context.Context, token string) context.Context {
	// Inject W3C trace context into gRPC metadata
	ctx = libOpentelemetry.InjectGRPCContext(ctx)

	pairs := []string{}

	// Optionally propagate authorization token
	if strings.TrimSpace(token) != "" {
		pairs = append(pairs, constant.MetadataAuthorization, token)
	}

	if len(pairs) == 0 {
		return ctx
	}

	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

// defaultHealthCheckTimeout is the default timeout for gRPC health checks.
const defaultHealthCheckTimeout = 5 * time.Second

// ErrGRPCConnectionNotReady is returned when the gRPC connection is not in a ready state.
var ErrGRPCConnectionNotReady = errors.New("gRPC connection is not ready")

// getHealthCheckTimeout returns the configured health check timeout from environment variable
// GRPC_HEALTH_CHECK_TIMEOUT. If the variable is not set or has an invalid value,
// returns the default timeout of 5 seconds.
func getHealthCheckTimeout() time.Duration {
	timeoutStr := os.Getenv("GRPC_HEALTH_CHECK_TIMEOUT")
	if timeoutStr == "" {
		return defaultHealthCheckTimeout
	}

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		log.Printf("Warning: invalid GRPC_HEALTH_CHECK_TIMEOUT value %q, using default %v", timeoutStr, defaultHealthCheckTimeout)

		return defaultHealthCheckTimeout
	}

	if timeout <= 0 {
		log.Printf("Warning: non-positive GRPC_HEALTH_CHECK_TIMEOUT value %q, using default %v", timeoutStr, defaultHealthCheckTimeout)

		return defaultHealthCheckTimeout
	}

	return timeout
}

// CheckHealth verifies that the gRPC connection is healthy and ready to accept requests.
// It loops through gRPC connectivity state transitions (Idle → Connecting → Ready) within
// a configurable timeout from GRPC_HEALTH_CHECK_TIMEOUT environment variable (default: 5 seconds).
func (c *GRPCConnection) CheckHealth(ctx context.Context) error {
	if c.Conn == nil {
		c.Logger.Warn("gRPC connection is nil, attempting to establish connection")

		if err := c.Connect(); err != nil {
			c.Logger.Error("Failed to establish gRPC connection during health check", zap.Error(err))

			return ErrGRPCConnectionNotReady
		}
	}

	timeout := getHealthCheckTimeout()

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		state := c.Conn.GetState()

		if state == connectivity.Ready {
			return nil
		}

		if state == connectivity.Shutdown {
			c.Logger.Warn("gRPC connection is shut down")

			return ErrGRPCConnectionNotReady
		}

		if state == connectivity.Idle {
			c.Conn.Connect()
		}

		if !c.Conn.WaitForStateChange(timeoutCtx, state) {
			c.Logger.Warn("gRPC connection failed to become ready within timeout",
				zap.String("lastState", state.String()),
				zap.Duration("timeout", timeout))

			return ErrGRPCConnectionNotReady
		}
	}
}
