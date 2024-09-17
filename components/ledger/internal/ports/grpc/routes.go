package grpc

import (
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	"github.com/LerianStudio/midaz/components/ledger/internal/service"
	proto "github.com/LerianStudio/midaz/components/ledger/proto/account"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewRouterGRPC registers routes to the grpc.
func NewRouterGRPC(command *command.UseCase, query *query.UseCase) *grpc.Server {
	server := grpc.NewServer()

	_ = service.NewConfig()

	ap := &AccountProto{
		Command: command,
		Query:   query,
	}

	reflection.Register(server)
	proto.RegisterAccountProtoServer(server, ap)

	return server
}
