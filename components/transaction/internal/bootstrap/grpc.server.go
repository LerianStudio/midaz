package bootstrap

import (
	"google.golang.org/grpc"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v2/commons/server"
)

// ServerGRPC wraps a gRPC server with logging, telemetry, and configuration.
type ServerGRPC struct {
	server       *grpc.Server
	protoAddress string
	libLog.Logger
	libOpentelemetry.Telemetry
}

// ProtoAddress returns is a convenience method to return the proto server address.
func (sgrpc *ServerGRPC) ProtoAddress() string {
	return sgrpc.protoAddress
}

// NewServerGRPC creates a new ServerGRPC instance with the provided configuration and dependencies.
func NewServerGRPC(cfg *Config, server *grpc.Server, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) *ServerGRPC {
	return &ServerGRPC{
		server:       server,
		protoAddress: cfg.ProtoAddress,
		Logger:       logger,
		Telemetry:    *telemetry,
	}
}

// Run starts the gRPC server and blocks until graceful shutdown is triggered.
func (sgrpc *ServerGRPC) Run(_ *libCommons.Launcher) error {
	libCommonsServer.NewServerManager(nil, &sgrpc.Telemetry, sgrpc.Logger).
		WithGRPCServer(sgrpc.server, sgrpc.protoAddress).
		StartWithGracefulShutdown()

	return nil
}
