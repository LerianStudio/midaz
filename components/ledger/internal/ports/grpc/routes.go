package grpc

import (
	"github.com/LerianStudio/midaz/common/mcasdoor"
	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	lib "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewRouterGRPC registers routes to the grpc.
func NewRouterGRPC(lg mlog.Logger, tl *mopentelemetry.Telemetry, cc *mcasdoor.CasdoorConnection, cuc *command.UseCase, quc *query.UseCase) *grpc.Server {
	tlMid := lib.NewTelemetryMiddleware(tl)
	jwt := lib.NewJWTMiddleware(cc)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			tlMid.WithTelemetryInterceptor(tl),
			lib.WithGrpcLogging(lib.WithCustomLogger(lg)),
			jwt.ProtectGrpc(),
			jwt.WithPermissionGrpc(),
			tlMid.EndTracingSpansInterceptor(),
		),
	)

	reflection.Register(server)

	ap := &AccountProto{
		Command: cuc,
		Query:   quc,
	}

	proto.RegisterAccountProtoServer(server, ap)

	return server
}
