package grpc

import (
	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	"github.com/LerianStudio/midaz/components/ledger/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewRouterGRPC registers routes to the grpc.
func NewRouterGRPC(cuc *command.UseCase, quc *query.UseCase) *grpc.Server {
	server := grpc.NewServer()

	_ = service.NewConfig()

	reflection.Register(server)

	ap := &AccountProto{
		Command: cuc,
		Query:   quc,
	}

	proto.RegisterAccountProtoServer(server, ap)

	return server
}
