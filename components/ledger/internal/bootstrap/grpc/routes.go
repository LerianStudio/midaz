package grpc

import (
	"github.com/LerianStudio/midaz/common/mcasdoor"
	"github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/query"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewRouterGRPC registers routes to the grpc.
func NewRouterGRPC(lg mlog.Logger, tl *mopentelemetry.Telemetry, cc *mcasdoor.CasdoorConnection, cuc *command.UseCase, quc *query.UseCase) *grpc.Server {
	tlMid := http.NewTelemetryMiddleware(tl)
	jwt := http.NewJWTMiddleware(cc)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			tlMid.WithTelemetryInterceptor(tl),
			http.WithGrpcLogging(http.WithCustomLogger(lg)),
			jwt.ProtectGrpc(),
			jwt.WithPermissionGrpc(),
			tlMid.EndTracingSpansInterceptor(),
		),
	)

	reflection.Register(server)

	accountProto := &AccountProto{
		Command: cuc,
		Query:   quc,
	}

	account.RegisterAccountProtoServer(server, accountProto)

	return server
}
