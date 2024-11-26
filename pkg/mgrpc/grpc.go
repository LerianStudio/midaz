package mgrpc

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	gmtdt "google.golang.org/grpc/metadata"
	"log"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCConnection is a struct which deal with gRPC connections.
type GRPCConnection struct {
	Addr   string
	Conn   *grpc.ClientConn
	Logger mlog.Logger
}

// Connect keeps a singleton connection with gRPC.
func (c *GRPCConnection) Connect() error {
	conn, err := grpc.NewClient(c.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("Failed to connect on gRPC", zap.Error(err))
		return nil
	}

	c.Logger.Info("Connected to gRPC ✅ ")

	c.Conn = conn

	return nil
}

// GetNewClient returns a connection to gRPC, reconnect it if necessary.
func (c *GRPCConnection) GetNewClient() (*grpc.ClientConn, error) {
	if c.Conn == nil {
		if err := c.Connect(); err != nil {
			log.Printf("ERRCONECT %s", err)
			return nil, err
		}
	}

	return c.Conn, nil
}

func (c *GRPCConnection) ContextMetadataInjection(ctx context.Context, token string) context.Context {
	md := gmtdt.Join(
		gmtdt.Pairs(constant.MDMidazID, pkg.NewMidazIDFromContext(ctx)),
		gmtdt.Pairs(constant.MDAuthorization, "Bearer "+token),
	)

	ctx = gmtdt.NewOutgoingContext(ctx, md)

	ctx = mopentelemetry.InjectContext(ctx)

	return ctx
}
