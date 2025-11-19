package server_test

import (
	"testing"

	"github.com/LerianStudio/lib-commons/v2/commons/server"
	"github.com/stretchr/testify/assert"
	"github.com/gofiber/fiber/v2"
	"google.golang.org/grpc"
)

func TestNewGracefulShutdown(t *testing.T) {
	gs := server.NewGracefulShutdown(nil, nil, nil, nil, nil)
	assert.NotNil(t, gs, "NewGracefulShutdown should return a non-nil instance")
}

func TestNewGracefulShutdownWithGRPC(t *testing.T) {
	gs := server.NewGracefulShutdown(nil, nil, nil, nil, nil)
	assert.NotNil(t, gs, "NewGracefulShutdown should return a non-nil instance with gRPC server")
}

func TestNewServerManager(t *testing.T) {
	sm := server.NewServerManager(nil, nil, nil)
	assert.NotNil(t, sm, "NewServerManager should return a non-nil instance")
}

func TestServerManagerWithHTTPOnly(t *testing.T) {
	app := fiber.New()
	sm := server.NewServerManager(nil, nil, nil).
		WithHTTPServer(app, ":8080")
	assert.NotNil(t, sm, "ServerManager with HTTP server should return a non-nil instance")
}

func TestServerManagerWithGRPCOnly(t *testing.T) {
	grpcServer := grpc.NewServer()
	sm := server.NewServerManager(nil, nil, nil).
		WithGRPCServer(grpcServer, ":50051")
	assert.NotNil(t, sm, "ServerManager with gRPC server should return a non-nil instance")
}

func TestServerManagerWithBothServers(t *testing.T) {
	app := fiber.New()
	grpcServer := grpc.NewServer()
	sm := server.NewServerManager(nil, nil, nil).
		WithHTTPServer(app, ":8080").
		WithGRPCServer(grpcServer, ":50051")
	assert.NotNil(t, sm, "ServerManager with both servers should return a non-nil instance")
}

func TestServerManagerChaining(t *testing.T) {
	app := fiber.New()
	grpcServer := grpc.NewServer()
	
	// Test method chaining
	sm1 := server.NewServerManager(nil, nil, nil).WithHTTPServer(app, ":8080")
	sm2 := sm1.WithGRPCServer(grpcServer, ":50051")
	
	assert.Equal(t, sm1, sm2, "Method chaining should return the same instance")
}
