// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	managerHealthTimeout      = 60 * time.Second
	managerShutdownTimeout    = 10 * time.Second
	managerHealthCheckTimeout = 2 * time.Second
)

// ManagerService wraps a Manager subprocess for testing.
type ManagerService struct {
	cmd     *exec.Cmd
	addr    string
	port    int
	started bool
	mu      sync.Mutex
}

// StartManager builds and starts a Manager service as a subprocess.
func StartManager(ctx context.Context, cfg *ServiceConfig) (*ManagerService, error) {
	// Find available port
	port, err := findAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("find available port: %w", err)
	}

	cfg.ServerAddress = fmt.Sprintf("127.0.0.1:%d", port)

	// Build the manager binary if needed
	binaryPath := "./.bin/manager-test"
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, "./components/reporter-manager/cmd/app")
	buildCmd.Dir = findProjectRoot()
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		return nil, fmt.Errorf("build manager: %w", err)
	}

	// Create command with environment
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Dir = findProjectRoot()
	cmd.Env = buildManagerEnv(cfg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	ms := &ManagerService{
		cmd:  cmd,
		addr: "http://" + cfg.ServerAddress,
		port: port,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start manager: %w", err)
	}

	ms.mu.Lock()
	ms.started = true
	ms.mu.Unlock()

	// Wait for server to be ready
	if err := waitForHealth(ctx, ms.addr, managerHealthTimeout); err != nil {
		_ = ms.Stop(ctx)
		return nil, fmt.Errorf("wait for manager health: %w", err)
	}

	return ms, nil
}

// Address returns the HTTP address of the Manager service.
func (m *ManagerService) Address() string {
	return m.addr
}

// Port returns the port the Manager is listening on.
func (m *ManagerService) Port() int {
	return m.port
}

// Stop gracefully shuts down the Manager service.
func (m *ManagerService) Stop(ctx context.Context) error {
	m.mu.Lock()
	started := m.started
	m.mu.Unlock()

	if !started || m.cmd == nil || m.cmd.Process == nil {
		return nil
	}

	// Send SIGTERM for graceful shutdown
	if err := m.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// If SIGTERM fails, try SIGKILL
		_ = m.cmd.Process.Kill()
	}

	// Wait for process to exit with timeout
	done := make(chan error, 1)

	go func() {
		done <- m.cmd.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-time.After(managerShutdownTimeout):
		_ = m.cmd.Process.Kill()
		return fmt.Errorf("timeout waiting for manager shutdown, killed")
	case <-ctx.Done():
		_ = m.cmd.Process.Kill()
		return ctx.Err()
	}
}

// buildManagerEnv creates environment variables for the Manager process.
func buildManagerEnv(cfg *ServiceConfig) []string {
	env := os.Environ()

	// Service
	env = append(env, "ENV_NAME=test")
	env = append(env, "SERVER_ADDRESS="+cfg.ServerAddress)
	env = append(env, "LOG_LEVEL=error")

	// MongoDB
	env = append(env, "MONGO_URI=mongodb")
	env = append(env, "MONGO_HOST="+cfg.MongoHost)
	env = append(env, "MONGO_PORT="+cfg.MongoPort)
	env = append(env, "MONGO_USER="+cfg.MongoUser)
	env = append(env, "MONGO_PASSWORD="+cfg.MongoPassword)
	env = append(env, "MONGO_NAME="+cfg.MongoDatabase)

	// RabbitMQ
	env = append(env, "RABBITMQ_URI=amqp")
	env = append(env, "RABBITMQ_HOST="+cfg.RabbitHost)
	env = append(env, "RABBITMQ_PORT_AMQP="+cfg.RabbitPort)
	env = append(env, "RABBITMQ_PORT_HOST="+cfg.RabbitMgmtPort)
	env = append(env, "RABBITMQ_DEFAULT_USER="+cfg.RabbitUser)
	env = append(env, "RABBITMQ_DEFAULT_PASS="+cfg.RabbitPassword)
	env = append(env, "RABBITMQ_GENERATE_REPORT_QUEUE=reporter.generate-report.queue")
	env = append(env, "RABBITMQ_EXCHANGE=reporter.generate-report.exchange")
	env = append(env, "RABBITMQ_GENERATE_REPORT_KEY=reporter.generate-report.key")
	env = append(env, "RABBITMQ_HEALTH_CHECK_URL=http://"+cfg.RabbitHost+":"+cfg.RabbitMgmtPort)

	// S3/SeaweedFS
	env = append(env, "OBJECT_STORAGE_ENDPOINT="+cfg.S3Endpoint)
	env = append(env, "OBJECT_STORAGE_REGION="+cfg.S3Region)
	env = append(env, "OBJECT_STORAGE_ACCESS_KEY_ID="+cfg.S3AccessKey)
	env = append(env, "OBJECT_STORAGE_SECRET_KEY="+cfg.S3SecretKey)
	env = append(env, "OBJECT_STORAGE_BUCKET="+cfg.S3Bucket)
	env = append(env, "OBJECT_STORAGE_USE_PATH_STYLE=true")
	env = append(env, "OBJECT_STORAGE_DISABLE_SSL=true")

	// Redis/Valkey
	env = append(env, "REDIS_HOST="+cfg.RedisHost+":"+cfg.RedisPort)
	env = append(env, "REDIS_PASSWORD="+cfg.RedisPassword)
	env = append(env, "REDIS_DB=0")

	// Auth (disabled for tests)
	env = append(env, "PLUGIN_AUTH_ENABLED=false")

	// Telemetry (disabled for tests)
	env = append(env, "ENABLE_TELEMETRY=false")
	env = append(env, "OTEL_LIBRARY_NAME=reporter")

	// Onboarding datasource so report-create filter validation and the
	// generate-report message see midaz_onboarding as a registered datasource.
	env = append(env, cfg.onboardingDatasourceEnv()...)

	return env
}

// findAvailablePort finds an available TCP port.
func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)

	return addr.Port, nil
}

// waitForHealth polls the health endpoint until it returns 200 or timeout.
func waitForHealth(ctx context.Context, baseURL string, timeout time.Duration) error {
	healthURL := baseURL + "/health"
	deadline := time.Now().Add(timeout)

	client := &http.Client{Timeout: managerHealthCheckTimeout}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("health check timeout after %v", timeout)
}

// findProjectRoot finds the project root directory.
func findProjectRoot() string {
	// Start from current directory and look for go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}

	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir
		}

		parent := dir[:len(dir)-len(dir[len(dir)-1:])]
		if parent == dir {
			return "."
		}

		dir = parent
	}
}
