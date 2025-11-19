package server

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/LerianStudio/lib-commons/v2/commons/license"
	"github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/gofiber/fiber/v2"
	"google.golang.org/grpc"
)

// ServerManager handles the graceful shutdown of multiple server types.
// It can manage HTTP servers, gRPC servers, or both simultaneously.
type ServerManager struct {
	httpServer     *fiber.App
	grpcServer     *grpc.Server
	licenseClient  *license.ManagerShutdown
	telemetry      *opentelemetry.Telemetry
	logger         log.Logger
	httpAddress    string
	grpcAddress    string
	serversStarted chan struct{}
}

// NewServerManager creates a new instance of ServerManager.
func NewServerManager(
	licenseClient *license.ManagerShutdown,
	telemetry *opentelemetry.Telemetry,
	logger log.Logger,
) *ServerManager {
	return &ServerManager{
		licenseClient:  licenseClient,
		telemetry:      telemetry,
		logger:         logger,
		serversStarted: make(chan struct{}),
	}
}

// WithHTTPServer configures the HTTP server for the ServerManager.
func (sm *ServerManager) WithHTTPServer(app *fiber.App, address string) *ServerManager {
	sm.httpServer = app
	sm.httpAddress = address

	return sm
}

// WithGRPCServer configures the gRPC server for the ServerManager.
func (sm *ServerManager) WithGRPCServer(server *grpc.Server, address string) *ServerManager {
	sm.grpcServer = server
	sm.grpcAddress = address

	return sm
}

// StartWithGracefulShutdown initializes all configured servers and sets up graceful shutdown.
func (sm *ServerManager) StartWithGracefulShutdown() {
	// Run everything in a recover block
	defer func() {
		if r := recover(); r != nil {
			if sm.logger != nil {
				sm.logger.Errorf("Fatal error (panic): %v", r)
			} else {
				// Fallback to standard log if logger is nil
				fmt.Printf("Fatal error (panic): %v\n", r)
			}

			sm.executeShutdown()

			os.Exit(1)
		}
	}()

	// Start configured servers
	sm.startServers()

	// Handle graceful shutdown
	sm.handleShutdown()
}

// startServers starts all configured servers in separate goroutines.
func (sm *ServerManager) startServers() {
	started := 0
	total := 0

	// Count total servers to start
	if sm.httpServer != nil {
		total++
	}

	if sm.grpcServer != nil {
		total++
	}

	if total == 0 {
		sm.logger.Fatal("No servers configured. Use WithHTTPServer() or WithGRPCServer() to configure servers.")
		return
	}

	// Start HTTP server if configured
	if sm.httpServer != nil {
		go func() {
			sm.logger.Infof("Starting HTTP server on %s", sm.httpAddress)

			if err := sm.httpServer.Listen(sm.httpAddress); err != nil {
				// During normal shutdown, Listen() will return an error
				// We only want to log unexpected errors
				sm.logger.Errorf("HTTP server error: %v", err)
			}
		}()

		started++
	}

	// Start gRPC server if configured
	if sm.grpcServer != nil {
		go func() {
			sm.logger.Infof("Starting gRPC server on %s", sm.grpcAddress)

			listener, err := net.Listen("tcp", sm.grpcAddress)
			if err != nil {
				sm.logger.Errorf("Failed to listen on gRPC address: %v", err)
				return
			}

			if err := sm.grpcServer.Serve(listener); err != nil {
				// During normal shutdown, Serve() will return an error
				// We only want to log unexpected errors
				sm.logger.Errorf("gRPC server error: %v", err)
			}
		}()

		started++
	}

	sm.logger.Infof("Started %d server(s)", started)

	close(sm.serversStarted)
}

// logInfo safely logs an info message if logger is available
func (sm *ServerManager) logInfo(msg string) {
	if sm.logger != nil {
		sm.logger.Info(msg)
	}
}

// logErrorf safely logs an error message if logger is available
func (sm *ServerManager) logErrorf(format string, args ...any) {
	if sm.logger != nil {
		sm.logger.Errorf(format, args...)
	}
}

// handleShutdown sets up signal handling and executes the shutdown sequence
// when a termination signal is received.
func (sm *ServerManager) handleShutdown() {
	// Create channel for shutdown signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Block until we receive a signal
	<-c
	sm.logInfo("Gracefully shutting down all servers...")

	// Execute shutdown sequence
	sm.executeShutdown()
}

// executeShutdown performs the actual shutdown operations in the correct order for ServerManager.
func (sm *ServerManager) executeShutdown() {
	// Use a non-blocking read to check if servers have started.
	// This prevents a deadlock if a panic occurs before startServers() completes.
	select {
	case <-sm.serversStarted:
		// Servers started, proceed with normal shutdown.
	default:
		// Servers did not start (or start was interrupted).
		sm.logInfo("Shutdown initiated before servers were fully started.")
	}

	// Shutdown the HTTP server if available
	if sm.httpServer != nil {
		sm.logInfo("Shutting down HTTP server...")

		if err := sm.httpServer.Shutdown(); err != nil {
			sm.logErrorf("Error during HTTP server shutdown: %v", err)
		}
	}

	// Shutdown telemetry BEFORE gRPC server to allow metrics export
	if sm.telemetry != nil {
		sm.logInfo("Shutting down telemetry...")
		sm.telemetry.ShutdownTelemetry()
	}

	// Shutdown the gRPC server if available
	if sm.grpcServer != nil {
		sm.logInfo("Shutting down gRPC server...")

		// Use GracefulStop which waits for all RPCs to finish
		sm.grpcServer.GracefulStop()
		sm.logInfo("gRPC server stopped gracefully")
	}

	// Sync logger if available
	if sm.logger != nil {
		sm.logInfo("Syncing logger...")

		if err := sm.logger.Sync(); err != nil {
			sm.logErrorf("Failed to sync logger: %v", err)
		}
	}

	// Shutdown license background refresh if available
	if sm.licenseClient != nil {
		sm.logInfo("Shutting down license background refresh...")
		sm.licenseClient.Terminate("shutdown")
	}

	sm.logInfo("Graceful shutdown completed")
}

// GracefulShutdown handles the graceful shutdown of application components.
// It's designed to be reusable across different services.
// Deprecated: Use ServerManager instead for better coordination.
type GracefulShutdown struct {
	app           *fiber.App
	grpcServer    *grpc.Server
	licenseClient *license.ManagerShutdown
	telemetry     *opentelemetry.Telemetry
	logger        log.Logger
}

// NewGracefulShutdown creates a new instance of GracefulShutdown.
// Deprecated: Use NewServerManager instead for better coordination.
func NewGracefulShutdown(
	app *fiber.App,
	grpcServer *grpc.Server,
	licenseClient *license.ManagerShutdown,
	telemetry *opentelemetry.Telemetry,
	logger log.Logger,
) *GracefulShutdown {
	return &GracefulShutdown{
		app:           app,
		grpcServer:    grpcServer,
		licenseClient: licenseClient,
		telemetry:     telemetry,
		logger:        logger,
	}
}

// HandleShutdown sets up signal handling and executes the shutdown sequence
// when a termination signal is received.
// Deprecated: Use ServerManager.StartWithGracefulShutdown() instead.
func (gs *GracefulShutdown) HandleShutdown() {
	// Create channel for shutdown signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Block until we receive a signal
	<-c
	gs.logger.Info("Gracefully shutting down...")

	// Execute shutdown sequence
	gs.executeShutdown()
}

// executeShutdown performs the actual shutdown operations in the correct order.
// Deprecated: Use ServerManager.executeShutdown() for better coordination.
func (gs *GracefulShutdown) executeShutdown() {
	// Shutdown the HTTP server if available
	if gs.app != nil {
		gs.logger.Info("Shutting down HTTP server...")

		if err := gs.app.Shutdown(); err != nil {
			gs.logger.Errorf("Error during HTTP server shutdown: %v", err)
		}
	}

	// Shutdown the gRPC server if available
	if gs.grpcServer != nil {
		gs.logger.Info("Shutting down gRPC server...")

		// Use GracefulStop which waits for all RPCs to finish
		gs.grpcServer.GracefulStop()
		gs.logger.Info("gRPC server stopped gracefully")
	}

	// Shutdown telemetry if available
	if gs.telemetry != nil {
		gs.logger.Info("Shutting down telemetry...")
		gs.telemetry.ShutdownTelemetry()
	}

	// Sync logger if available
	if gs.logger != nil {
		gs.logger.Info("Syncing logger...")

		if err := gs.logger.Sync(); err != nil {
			gs.logger.Errorf("Failed to sync logger: %v", err)
		}
	}

	// Shutdown license background refresh if available
	if gs.licenseClient != nil {
		gs.logger.Info("Shutting down license background refresh...")
		gs.licenseClient.Terminate("shutdown")
	}

	gs.logger.Info("Graceful shutdown completed")
}
