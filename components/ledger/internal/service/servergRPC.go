package service

import (
	"fmt"
	protoHandler "github.com/LerianStudio/midaz/components/ledger/internal/ports/grcp"
	proto "github.com/LerianStudio/midaz/components/ledger/proto/account"
	"google.golang.org/grpc/reflection"
	"net"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// ServerGRPC represents the gRPC server for Ledger service.
type ServerGRPC struct {
	listener     *net.Listener
	server       *grpc.Server
	protoAddress string
	mlog.Logger
}

// ProtoAddress returns is a convenience method to return the proto server address.
func (s *ServerGRPC) ProtoAddress() string {
	return s.protoAddress
}

// NewServerGRPC creates an instance of gRPC Server.
func NewServerGRPC(cfg *Config, logger mlog.Logger) *ServerGRPC {
	server := grpc.NewServer()

	listen, err := net.Listen("tcp4", cfg.ProtoAddress)
	if err != nil {
		fmt.Println(err.Error())
	}

	reflection.Register(server)

	accountService := protoHandler.NewAccountService()

	proto.RegisterAccountServiceServer(server, accountService)

	return &ServerGRPC{
		listener:     &listen,
		server:       server,
		protoAddress: cfg.ProtoAddress,
		Logger:       logger,
	}
}

// Run gRPC server.
func (s *ServerGRPC) Run(l *common.Launcher) error {
	err := s.server.Serve(*s.listener)
	if err != nil {
		return errors.Wrap(err, "failed to run the gRPC server")
	}

	return nil
}
