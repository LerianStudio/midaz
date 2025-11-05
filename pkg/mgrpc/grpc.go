package mgrpc

import (
	"log"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// GRPCConnection is a struct which deal with gRPC connections.
type GRPCConnection struct {
	Addr   string
	Conn   *grpc.ClientConn
	Logger libLog.Logger
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
