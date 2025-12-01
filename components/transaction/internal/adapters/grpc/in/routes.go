// Package in provides gRPC inbound adapters for the transaction component.
//
// This package implements the gRPC transport layer for the transaction bounded context,
// exposing RPC endpoints for high-performance balance operations. It follows the hexagonal
// architecture pattern where gRPC handlers adapt external requests to internal use cases.
//
// Architecture Overview:
//
// The gRPC adapter layer provides:
//   - Balance service RPC endpoints for internal service communication
//   - Server reflection for debugging and service discovery
//   - Authentication via gRPC interceptor chain
//   - OpenTelemetry tracing integration
//   - Structured logging for all RPC calls
//
// Why gRPC for Balances:
//
// Balance operations use gRPC instead of HTTP because:
//   - Lower latency for real-time balance checks during transactions
//   - Efficient binary serialization (Protocol Buffers)
//   - Strong typing with code generation
//   - Streaming support for bulk operations
//   - Internal service-to-service communication pattern
//
// Service Definition:
//
// The balance.proto defines:
//   - CreateBalance: Create a new balance for an account
//   - GetBalance: Retrieve balance by ID
//   - DeleteBalance: Soft delete a balance
//
// Security:
//
// All RPC methods require authentication via the gRPC unary interceptor.
// Authorization is enforced per method using policy configuration.
//
// Related Packages:
//   - balance: Protocol Buffer generated code
//   - middleware: gRPC auth interceptor
//   - command/query: Use cases for balance operations
package in

import (
	"context"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	balance "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const midazName = "midaz"

// NewRouterGRPC creates and configures the gRPC server for the transaction component.
//
// This function sets up the complete gRPC infrastructure including:
//   - gRPC server with interceptor chain
//   - OpenTelemetry tracing interceptor
//   - Structured logging interceptor
//   - Authentication/authorization interceptor
//   - Server reflection for debugging
//   - Balance service registration
//
// Server Configuration:
//
//	Interceptor Chain (executed in order):
//	  1. Telemetry: Distributed tracing context propagation
//	  2. Logging: Structured RPC logging
//	  3. Auth: JWT/API key validation and authorization
//
// Authorization Policies:
//
// Method policies define required permissions:
//   - CreateBalance: balances/post
//   - GetBalance: balances/get
//   - DeleteBalance: balances/delete
//
// Parameters:
//   - lg: Structured logger for RPC logging
//   - tl: OpenTelemetry telemetry instance
//   - auth: Authentication client for JWT/API key validation
//   - commandUseCase: Command use cases for write operations
//   - queryUseCase: Query use cases for read operations
//
// Returns:
//   - *grpc.Server: Configured gRPC server ready to serve
//
// Usage:
//
//	server := NewRouterGRPC(logger, telemetry, auth, cmdUC, queryUC)
//	listener, _ := net.Listen("tcp", ":50051")
//	server.Serve(listener)
func NewRouterGRPC(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, commandUseCase *command.UseCase, queryUseCase *query.UseCase) *grpc.Server {
	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			tlMid.WithTelemetryInterceptor(tl),
			libHTTP.WithGrpcLogging(libHTTP.WithCustomLogger(lg)),
			middleware.NewGRPCAuthUnaryPolicy(auth, middleware.PolicyConfig{
				MethodPolicies: map[string]middleware.Policy{
					"/balance.BalanceProto/CreateBalance": {Resource: "balances", Action: "post"},
					"/balance.BalanceProto/GetBalance":    {Resource: "balances", Action: "get"},
					"/balance.BalanceProto/DeleteBalance": {Resource: "balances", Action: "delete"},
				},
				SubResolver: func(ctx context.Context, _ string, _ any) (string, error) { return midazName, nil },
			}),
		),
	)

	reflection.Register(server)

	balanceProto := &BalanceProto{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	balance.RegisterBalanceProtoServer(server, balanceProto)

	return server
}
