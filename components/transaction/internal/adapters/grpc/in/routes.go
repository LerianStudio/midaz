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

func NewRouterGRPC(lg libLog.Logger, tl *libOpentelemetry.Telemetry, auth *middleware.AuthClient, commandUseCase *command.UseCase, queryUseCase *query.UseCase) *grpc.Server {
	tlMid := libHTTP.NewTelemetryMiddleware(tl)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			tlMid.WithTelemetryInterceptor(tl),
			libHTTP.WithGrpcLogging(libHTTP.WithCustomLogger(lg)),
			middleware.NewGRPCAuthUnaryPolicy(auth, middleware.PolicyConfig{
				MethodPolicies: map[string]middleware.Policy{
					"/balance.BalanceProto/CreateBalance": {Resource: "balances", Action: "post"},
				},
				SubResolver: func(ctx context.Context, _ string, _ any) (string, error) { return midazName, nil },
			}),
			tlMid.EndTracingSpansInterceptor(),
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
