package bootstrap

import (
	"net"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// ServerGRPC represents the gRPC server for Ledger service.
type ServerGRPC struct {
	server       *grpc.Server
	protoAddress string
	mlog.Logger
}

// ProtoAddress returns is a convenience method to return the proto server address.
func (sgrpc *ServerGRPC) ProtoAddress() string {
	return sgrpc.protoAddress
}

// NewServerGRPC creates an instance of gRPC Server.
func NewServerGRPC(cfg *Config, server *grpc.Server, logger mlog.Logger) *ServerGRPC {
	return &ServerGRPC{
		server:       server,
		protoAddress: cfg.ProtoAddress,
		Logger:       logger,
	}
}

// Run gRPC server.
func (sgrpc *ServerGRPC) Run(l *common.Launcher) error {
	listen, err := net.Listen("tcp4", sgrpc.protoAddress)
	if err != nil {
		return errors.Wrap(err, "failed to listen tcp4 server")
	}

	err = sgrpc.server.Serve(listen)
	if err != nil {
		return errors.Wrap(err, "failed to run the gRPC server")
	}

	return nil
}
