//go:build chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	h "github.com/LerianStudio/reporter/tests/utils"
	chaosutil "github.com/LerianStudio/reporter/tests/utils/chaos"
	"github.com/LerianStudio/reporter/tests/utils/containers"
	"github.com/LerianStudio/reporter/tests/utils/services"
)

var (
	testInfra   *containers.TestInfrastructure
	managerSvc  *services.ManagerService
	workerSvc   *services.WorkerService
	managerAddr string

	// Expose containers for chaos test manipulation
	MongoContainer   *containers.MongoDBContainer
	RabbitContainer  *containers.RabbitMQContainer
	SeaweedContainer *containers.SeaweedFSContainer
	ValkeyContainer  *containers.ValkeyContainer

	// Toxiproxy infrastructure for fault injection without container restart.
	// Initialized in TestMain after all infrastructure containers are running.
	toxiInfra *chaosutil.ToxiproxyInfrastructure
)

func TestMain(m *testing.M) {
	// Check if we should use testcontainers or existing infrastructure
	if os.Getenv("USE_EXISTING_INFRA") == "true" {
		// Use existing infrastructure (docker-compose)
		fmt.Fprintf(os.Stderr, "Using existing infrastructure from docker-compose\n")
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Starting test infrastructure with testcontainers for chaos tests...\n")

	// Start infrastructure containers
	var err error
	testInfra, err = containers.StartInfrastructure(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start infrastructure: %v\n", err)
		os.Exit(1)
	}

	// Store container references for chaos manipulation
	MongoContainer = testInfra.MongoDB
	RabbitContainer = testInfra.RabbitMQ
	SeaweedContainer = testInfra.SeaweedFS
	ValkeyContainer = testInfra.Valkey

	fmt.Fprintf(os.Stderr, "Infrastructure started successfully\n")

	// Start Toxiproxy for fault injection (non-fatal: chaos tests can fall back to container restart)
	fmt.Fprintf(os.Stderr, "Starting Toxiproxy for fault injection...\n")
	if err := testInfra.StartToxiproxy(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to start Toxiproxy: %v (falling back to container restart)\n", err)
	} else {
		toxiInfra = testInfra.Toxiproxy
		fmt.Fprintf(os.Stderr, "Toxiproxy started with %d proxies\n", len(toxiInfra.Proxies))
	}

	// Create service configuration from containers
	cfg := services.NewConfigFromInfrastructure(testInfra)

	// Start Manager service
	fmt.Fprintf(os.Stderr, "Starting Manager service...\n")
	managerSvc, err = services.StartManager(ctx, cfg)
	if err != nil {
		testInfra.Stop(ctx)
		fmt.Fprintf(os.Stderr, "Failed to start manager: %v\n", err)
		os.Exit(1)
	}
	managerAddr = managerSvc.Address()
	fmt.Fprintf(os.Stderr, "Manager started at %s\n", managerAddr)

	// Set environment variable for test helpers
	os.Setenv("MANAGER_URL", managerAddr)
	defer os.Unsetenv("MANAGER_URL")

	// Start Worker service
	fmt.Fprintf(os.Stderr, "Starting Worker service...\n")
	workerSvc, err = services.StartWorker(ctx, cfg)
	if err != nil {
		managerSvc.Stop(ctx)
		testInfra.Stop(ctx)
		fmt.Fprintf(os.Stderr, "Failed to start worker: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Worker started successfully\n")

	// Upload test templates for chaos tests
	fmt.Fprintf(os.Stderr, "Uploading test templates...\n")
	if err := uploadTestTemplates(ctx, managerAddr); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to upload test templates: %v\n", err)
	}

	// Run tests
	fmt.Fprintf(os.Stderr, "Running chaos tests...\n")
	code := m.Run()

	// NOTE: Cleanup is performed in TestMain (not t.Cleanup()) because all tests
	// in this package share a single infrastructure instance. Per-test cleanup
	// would terminate containers prematurely. This is the correct pattern for
	// shared testcontainer infrastructure.
	fmt.Fprintf(os.Stderr, "Cleaning up...\n")
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cleanupCancel()

	if workerSvc != nil {
		workerSvc.Stop(cleanupCtx)
	}
	if managerSvc != nil {
		managerSvc.Stop(cleanupCtx)
	}
	if testInfra != nil {
		testInfra.Stop(cleanupCtx)
	}

	fmt.Fprintf(os.Stderr, "Cleanup complete\n")
	os.Exit(code)
}

// GetManagerAddress returns the Manager service address for tests.
func GetManagerAddress() string {
	if managerAddr != "" {
		return managerAddr
	}
	// Fallback to environment variable or default
	if addr := os.Getenv("MANAGER_URL"); addr != "" {
		return addr
	}
	return "http://127.0.0.1:4005"
}

// RestartMongoDB restarts the MongoDB container for chaos testing.
func RestartMongoDB(delay time.Duration) error {
	if MongoContainer == nil {
		return nil // Using existing infra
	}
	return MongoContainer.Restart(context.Background(), delay)
}

// RestartRabbitMQ restarts the RabbitMQ container for chaos testing.
func RestartRabbitMQ(delay time.Duration) error {
	if RabbitContainer == nil {
		return nil // Using existing infra
	}
	return RabbitContainer.Restart(context.Background(), delay)
}

// RestartValkey restarts the Valkey/Redis container for chaos testing.
func RestartValkey(delay time.Duration) error {
	if ValkeyContainer == nil {
		return nil // Using existing infra
	}
	return ValkeyContainer.Restart(context.Background(), delay)
}

// StopMongoDB stops the MongoDB container for chaos testing.
func StopMongoDB() error {
	if MongoContainer == nil {
		return nil
	}
	return MongoContainer.Stop(context.Background(), nil)
}

// StartMongoDB starts the MongoDB container after being stopped.
func StartMongoDB() error {
	if MongoContainer == nil {
		return nil
	}
	return MongoContainer.Start(context.Background())
}

// StopRabbitMQ stops the RabbitMQ container for chaos testing.
func StopRabbitMQ() error {
	if RabbitContainer == nil {
		return nil
	}
	return RabbitContainer.Stop(context.Background(), nil)
}

// StartRabbitMQ starts the RabbitMQ container after being stopped.
func StartRabbitMQ() error {
	if RabbitContainer == nil {
		return nil
	}
	return RabbitContainer.Start(context.Background())
}

// uploadTestTemplates uploads templates from tests/chaos/templates directory.
func uploadTestTemplates(ctx context.Context, managerURL string) error {
	cli := h.NewHTTPClient(managerURL, 30*time.Second)
	headers := h.AuthHeaders()

	// Find template files
	templatesDir := filepath.Join(findTestRoot(), "tests", "chaos", "templates")
	templateFiles, err := filepath.Glob(filepath.Join(templatesDir, "*.tpl"))
	if err != nil {
		return err
	}

	if len(templateFiles) == 0 {
		fmt.Fprintf(os.Stderr, "No template files found in chaos/templates directory\n")
		return nil
	}

	for _, tplFile := range templateFiles {
		tplContent, err := os.ReadFile(tplFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read template %s: %v\n", tplFile, err)
			continue
		}

		tplName := filepath.Base(tplFile)
		formData := map[string]string{
			"outputFormat": "TXT",
			"description":  "Chaos test template: " + tplName,
		}
		files := map[string][]byte{
			"template": tplContent,
		}

		code, body, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", headers, formData, files)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to upload template %s: %v\n", tplName, err)
			continue
		}

		if code == 200 || code == 201 {
			var resp struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(body, &resp) == nil {
				fmt.Fprintf(os.Stderr, "Uploaded template %s with ID: %s\n", tplName, resp.ID)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Template upload returned %d: %s\n", code, string(body))
		}
	}

	return nil
}

// findTestRoot finds the project root directory for locating test templates.
func findTestRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}

	// Walk up until we find go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
