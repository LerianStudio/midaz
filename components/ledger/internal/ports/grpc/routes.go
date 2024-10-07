package grpc

import (
	"github.com/LerianStudio/midaz/common/mcasdoor"
	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
	lib "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewRouterGRPC registers routes to the grpc.
func NewRouterGRPC(cc *mcasdoor.CasdoorConnection, cuc *command.UseCase, quc *query.UseCase) *grpc.Server {
	jwt := lib.NewJWTMiddleware(cc)
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			jwt.ProtectGrpc(),
			jwt.WithPermissionGrpc(),
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
