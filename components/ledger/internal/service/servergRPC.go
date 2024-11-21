package service

import (
	"github.com/LerianStudio/midaz/common/mopentelemetry"
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
	mopentelemetry.Telemetry
}

// ProtoAddress returns is a convenience method to return the proto server address.
func (sgrpc *ServerGRPC) ProtoAddress() string {
	return sgrpc.protoAddress
}

// NewServerGRPC creates an instance of gRPC Server.
func NewServerGRPC(cfg *Config, server *grpc.Server, logger mlog.Logger, telemetry *mopentelemetry.Telemetry) *ServerGRPC {
	return &ServerGRPC{
		server:       server,
		protoAddress: cfg.ProtoAddress,
		Logger:       logger,
		Telemetry:    *telemetry,
	}
}

// Run gRPC server.
func (sgrpc *ServerGRPC) Run(l *common.Launcher) error {
	sgrpc.InitializeTelemetry(sgrpc.Logger)
	defer sgrpc.ShutdownTelemetry()

	defer func() {
		if err := sgrpc.Logger.Sync(); err != nil {
			sgrpc.Logger.Fatalf("Failed to sync logger: %s", err)
		}
	}()

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
