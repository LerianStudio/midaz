// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"google.golang.org/grpc"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v2/commons/server"
)

// ServerGRPC wraps a gRPC server with address and telemetry information.
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

// NewServerGRPC creates a new ServerGRPC from the given config and dependencies.
func NewServerGRPC(cfg *Config, server *grpc.Server, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) *ServerGRPC {
	return &ServerGRPC{
		server:       server,
		protoAddress: cfg.ProtoAddress,
		Logger:       logger,
		Telemetry:    *telemetry,
	}
}

// Run starts the gRPC server and blocks until the server shuts down gracefully.
func (sgrpc *ServerGRPC) Run(l *libCommons.Launcher) error {
	libCommonsServer.NewServerManager(nil, &sgrpc.Telemetry, sgrpc.Logger).
		WithGRPCServer(sgrpc.server, sgrpc.protoAddress).
		StartWithGracefulShutdown()

	return nil
}
