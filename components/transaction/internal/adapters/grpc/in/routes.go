package in

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	balance "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const midazName = "midaz"

// grpcPanicRecoveryInterceptor creates a unary interceptor that recovers from panics.
// NOTE(phase2): Stream interceptor variant needed if streaming gRPC endpoints are added.
// Implementation pattern: grpc.StreamServerInterceptor with similar panic recovery logic.
// Current state: No streaming endpoints exist, so unary-only interceptor is sufficient.
func grpcPanicRecoveryInterceptor(lg libLog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				panicValue := fmt.Sprintf("%v", r)

				// Record panic as a span event for observability
				span := trace.SpanFromContext(ctx)
				span.AddEvent("panic.recovered", trace.WithAttributes(
					attribute.String("panic.value", panicValue),
					attribute.String("panic.stack", string(stack)),
					attribute.String("grpc.method", info.FullMethod),
				))

				lg.WithFields(
					"panic_value", panicValue,
					"stack_trace", string(stack),
					"grpc_method", info.FullMethod,
				).Errorf("gRPC handler panic recovered: %v", r)

				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}

// NewRouterGRPC creates and configures a new gRPC server with all required interceptors.
// It sets up panic recovery, telemetry, logging, and authentication middleware,
// and registers the balance service handler.
func NewRouterGRPC(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, commandUseCase *command.UseCase, queryUseCase *query.UseCase) *grpc.Server {
	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			// Panic recovery interceptor - MUST be first to catch panics from all other interceptors.
			// Note: gRPC recovery is always "keep running" in this phase.
			grpcPanicRecoveryInterceptor(lg),
			tlMid.WithTelemetryInterceptor(tl),
			libHTTP.WithGrpcLogging(libHTTP.WithCustomLogger(lg)),
			middleware.NewGRPCAuthUnaryPolicy(auth, middleware.PolicyConfig{
				MethodPolicies: map[string]middleware.Policy{
					"/balance.BalanceProto/CreateBalance":                {Resource: "balances", Action: "post"},
					"/balance.BalanceProto/GetBalance":                   {Resource: "balances", Action: "get"},
					"/balance.BalanceProto/DeleteBalance":                {Resource: "balances", Action: "delete"},
					"/balance.BalanceProto/DeleteAllBalancesByAccountID": {Resource: "balances", Action: "delete"},
				},
				SubResolver: func(ctx context.Context, _ string, _ any) (string, error) { return midazName, nil },
			}),
		),
	)

	if os.Getenv("GRPC_REFLECTION_ENABLED") == "true" {
		reflection.Register(server)
	}

	balanceProto := &BalanceProto{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	balance.RegisterBalanceProtoServer(server, balanceProto)

	return server
}
