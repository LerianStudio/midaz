package mgrpc

import (
	"context"
	"fmt"
	"log"
	"strings"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// GRPCError wraps a gRPC-related error with context
type GRPCError struct {
	Message string
	Cause   error
}

// Error implements the error interface
func (e GRPCError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}

	return e.Message
}

// Unwrap returns the underlying error
func (e GRPCError) Unwrap() error {
	return e.Cause
}

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
		return GRPCError{Message: "failed to create gRPC client", Cause: err}
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
