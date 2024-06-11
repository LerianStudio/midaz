package service

import (
	"fmt"
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"net"
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

	listener, err := net.Listen("tcp4", cfg.ProtoAddress)
	if err != nil {
		fmt.Println(err.Error())
	}

	return &ServerGRPC{
		listener:     &listener,
		server:       server,
		protoAddress: cfg.ProtoAddress,
		Logger:       logger,
	}
}

// Run start gRPC server.
func (s *ServerGRPC) Run(l *common.Launcher) error {
	err := s.server.Serve(*s.listener)
	if err != nil {
		return errors.Wrap(err, "failed to run the gRPC server")
	}
	info := s.server.GetServiceInfo()

	fmt.Print(info)

	return nil
}
