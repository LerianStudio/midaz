package bootstrap

import (
	"google.golang.org/grpc"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v3/commons/server"
)

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

func NewServerGRPC(cfg *Config, server *grpc.Server, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) *ServerGRPC {
	return &ServerGRPC{
		server:       server,
		protoAddress: cfg.ProtoAddress,
		Logger:       logger,
		Telemetry:    *telemetry,
	}
}

func (sgrpc *ServerGRPC) Run(l *libCommons.Launcher) error {
	libCommonsServer.NewServerManager(nil, &sgrpc.Telemetry, sgrpc.Logger).
		WithGRPCServer(sgrpc.server, sgrpc.protoAddress).
		StartWithGracefulShutdown()

	return nil
}
