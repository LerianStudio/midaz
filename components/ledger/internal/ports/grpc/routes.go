package grpc

import (
	"github.com/LerianStudio/midaz/components/ledger/internal/service"
	proto "github.com/LerianStudio/midaz/components/ledger/proto/account"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewRouterGRPC registers routes to the grpc.
func NewRouterGRPC(ap *AccountProto) *grpc.Server {
	server := grpc.NewServer()

	_ = service.NewConfig()

	reflection.Register(server)
	proto.RegisterAccountProtoServer(server, ap)

	return server
}
