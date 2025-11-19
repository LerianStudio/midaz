package server_test

import (
	"testing"

	"github.com/LerianStudio/lib-commons/v2/commons/server"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestGracefulShutdownWithGRPCServer(t *testing.T) {
	// Create a new gRPC server for testing
	grpcServer := grpc.NewServer()
	
	// Create a graceful shutdown handler with the gRPC server
	gs := server.NewGracefulShutdown(nil, grpcServer, nil, nil, nil)
	
	// Assert that the graceful shutdown handler was created successfully
	assert.NotNil(t, gs, "NewGracefulShutdown should return a non-nil instance with gRPC server")
	
	// Test that we can create the shutdown handler without panicking
	// We don't test the actual signal handling as that would require OS signals
	assert.NotPanics(t, func() {
		// Just ensure the shutdown handler can be created and doesn't panic
		_ = server.NewGracefulShutdown(nil, grpcServer, nil, nil, nil)
	}, "Creating GracefulShutdown with gRPC server should not panic")
}

func TestServerManagerWithGRPCServer(t *testing.T) {
	grpcServer := grpc.NewServer()
	
	sm := server.NewServerManager(nil, nil, nil).
		WithGRPCServer(grpcServer, ":50051")
	
	assert.NotNil(t, sm, "ServerManager with gRPC server should not be nil")
}
