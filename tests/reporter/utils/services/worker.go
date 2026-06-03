// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const workerShutdownTimeout = 10 * time.Second

// WorkerService wraps a Worker subprocess for testing.
type WorkerService struct {
	cmd     *exec.Cmd
	waitCh  chan error
	started bool
	mu      sync.Mutex
}

// StartWorker builds and starts a Worker service as a subprocess.
func StartWorker(ctx context.Context, cfg *ServiceConfig) (*WorkerService, error) {
	// Build the worker binary if needed
	binaryPath := "./.bin/worker-test"
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, "./components/worker/cmd/app")
	buildCmd.Dir = findProjectRoot()
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		return nil, fmt.Errorf("build worker: %w", err)
	}

	if cfg == nil {
		return nil, fmt.Errorf("start worker: nil ServiceConfig")
	}

	// Create command with environment
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Dir = findProjectRoot()
	cmd.Env = buildWorkerEnv(cfg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	ws := &WorkerService{
		cmd:    cmd,
		waitCh: make(chan error, 1),
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start worker: %w", err)
	}

	go func() {
		err := cmd.Wait()

		ws.mu.Lock()
		ws.started = false
		ws.mu.Unlock()

		ws.waitCh <- err
	}()

	ws.mu.Lock()
	ws.started = true
	ws.mu.Unlock()

	// Give the worker time to connect to RabbitMQ and start consuming,
	// but fail fast if the process exits during startup.
	select {
	case err := <-ws.waitCh:
		if err != nil {
			return nil, fmt.Errorf("worker exited during startup: %w", err)
		}

		return nil, fmt.Errorf("worker exited during startup with no error (unexpected exit code 0)")
	case <-time.After(5 * time.Second):
	}

	return ws, nil
}

// Stop gracefully shuts down the Worker service.
func (w *WorkerService) Stop(ctx context.Context) error {
	w.mu.Lock()
	started := w.started
	w.mu.Unlock()

	if !started || w.cmd == nil || w.cmd.Process == nil {
		return nil
	}

	// Send SIGTERM for graceful shutdown
	if err := w.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// If SIGTERM fails, try SIGKILL
		_ = w.cmd.Process.Kill()
	}

	// Wait for process to exit with timeout
	select {
	case <-w.waitCh:
		return nil
	case <-time.After(workerShutdownTimeout):
		_ = w.cmd.Process.Kill()
		return fmt.Errorf("timeout waiting for worker shutdown, killed")
	case <-ctx.Done():
		_ = w.cmd.Process.Kill()
		return ctx.Err()
	}
}

// IsRunning returns whether the worker is currently running.
func (w *WorkerService) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.started
}

// buildWorkerEnv creates environment variables for the Worker process.
func buildWorkerEnv(cfg *ServiceConfig) []string {
	env := os.Environ()

	// Service
	env = append(env, "ENV_NAME=test")
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
	env = append(env, "RABBITMQ_HEALTH_CHECK_URL=http://"+cfg.RabbitHost+":"+cfg.RabbitMgmtPort)
	env = append(env, "RABBITMQ_NUMBERS_OF_WORKERS=2") // Fewer workers for tests

	// S3/SeaweedFS
	env = append(env, "OBJECT_STORAGE_ENDPOINT="+cfg.S3Endpoint)
	env = append(env, "OBJECT_STORAGE_REGION="+cfg.S3Region)
	env = append(env, "OBJECT_STORAGE_ACCESS_KEY_ID="+cfg.S3AccessKey)
	env = append(env, "OBJECT_STORAGE_SECRET_KEY="+cfg.S3SecretKey)
	env = append(env, "OBJECT_STORAGE_BUCKET="+cfg.S3Bucket)
	env = append(env, "OBJECT_STORAGE_USE_PATH_STYLE=true")
	env = append(env, "OBJECT_STORAGE_DISABLE_SSL=true")

	// PDF Pool (minimal for tests)
	env = append(env, "PDF_POOL_WORKERS=1")
	env = append(env, "PDF_TIMEOUT_SECONDS=30")

	// Telemetry (disabled for tests)
	env = append(env, "ENABLE_TELEMETRY=false")
	env = append(env, "OTEL_LIBRARY_NAME=reporter")

	return env
}
