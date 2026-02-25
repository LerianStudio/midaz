// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v3/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
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
					"/balance.BalanceProto/CreateBalance":                {Resource: "balances", Action: "post"},
					"/balance.BalanceProto/DeleteAllBalancesByAccountID": {Resource: "balances", Action: "delete"},
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
