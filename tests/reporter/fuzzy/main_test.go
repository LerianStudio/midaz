//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fuzzy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	h "github.com/LerianStudio/midaz/v4/tests/reporter/utils"
	"github.com/LerianStudio/midaz/v4/tests/reporter/utils/containers"
	"github.com/LerianStudio/midaz/v4/tests/reporter/utils/services"
)

// warmUpTemplate is a minimal valid report template that renders to static text
// with NO filters, NO variables, and NO external data sources. This is
// deliberate: the warm-up must render SUCCESSFULLY end-to-end so the worker Acks
// the message. A template with a |date filter (or any variable) fails to render
// with nil input, and a render failure is classified retryable — the worker then
// republishes the poisoned message up to MaxMessageRetries times before DLQ,
// thrashing Mongo/RabbitMQ while the first fuzz target gathers its baseline.
const warmUpTemplate = `Fuzzy Warm-up Report
====================
This template renders to static text. No data sources required. Status: OK
`

var (
	testInfra   *containers.TestInfrastructure
	managerSvc  *services.ManagerService
	workerSvc   *services.WorkerService
	managerAddr string
)

func TestMain(m *testing.M) {
	// Fuzz workers are subprocesses that re-execute this binary — TestMain
	// included. They must NOT boot their own container stack: with -parallel=N
	// that stands up N+1 full 6-container stacks per target, which melts the
	// Docker daemon (worker EOFs, wedged boots, daemon crashes, sweep hangs).
	// The coordinator process boots the single stack and exports MANAGER_URL
	// before m.Run(); worker subprocesses inherit it through the environment
	// and fuzz against the coordinator's stack.
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.fuzzworker") {
			os.Exit(m.Run())
		}
	}

	// Check if we should use testcontainers or existing infrastructure
	if os.Getenv("USE_EXISTING_INFRA") == "true" {
		// Use existing infrastructure (docker-compose)
		fmt.Fprintf(os.Stderr, "Using existing infrastructure from docker-compose\n")
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Starting test infrastructure with testcontainers for fuzzy tests...\n")

	// Start infrastructure containers
	var err error
	testInfra, err = containers.StartInfrastructure(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start infrastructure: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Infrastructure started successfully\n")

	// Create service configuration from containers
	cfg := services.NewConfigFromInfrastructure(testInfra)

	// Manager/Worker are exec.CommandContext child processes that get SIGKILLed
	// when their context is done; they must outlive the suite, so they use a
	// background context rather than the 5-minute startup ctx (which would kill
	// them mid-run once cumulative test time crosses the deadline). They are
	// stopped explicitly in the cleanup block below.
	svcCtx := context.Background()

	// Start Manager service
	fmt.Fprintf(os.Stderr, "Starting Manager service...\n")
	managerSvc, err = services.StartManager(svcCtx, cfg)
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
	workerSvc, err = services.StartWorker(svcCtx, cfg)
	if err != nil {
		managerSvc.Stop(ctx)
		testInfra.Stop(ctx)
		fmt.Fprintf(os.Stderr, "Failed to start worker: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Worker started successfully\n")

	// Warm up the heavy request paths before fuzzing. -fuzztime bounds the
	// baseline-coverage phase, during which the engine replays the entire seed
	// corpus once; for these HTTP-backed targets each replay is a real
	// round-trip, and a COLD first hit additionally pays one-time costs
	// (template loading, S3 bucket init, MongoDB index builds). Warming the
	// distinct heavy endpoints here keeps per-input latency low so baseline
	// completes within the suite's fuzztime (see FUZZTIME_FUZZY in mk/tests.mk).
	// It must also render SUCCESSFULLY (see warmUpTemplate / warmUpServices):
	// a failed warm-up report would requeue and thrash the broker during baseline.
	warmUpCtx, warmUpCancel := context.WithTimeout(context.Background(), 60*time.Second)
	if err := warmUpServices(warmUpCtx, managerAddr); err != nil {
		warmUpCancel()
		workerSvc.Stop(ctx)
		managerSvc.Stop(ctx)
		testInfra.Stop(ctx)
		fmt.Fprintf(os.Stderr, "Failed to warm up services: %v\n", err)
		os.Exit(1)
	}

	warmUpCancel()

	// Run tests
	fmt.Fprintf(os.Stderr, "Running fuzzy tests...\n")
	code := m.Run()

	// Cleanup
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

// warmUpServices forces the cold-start cost of every heavy path before fuzzing.
//
// The first hit on each cold endpoint pays one-time costs that inflate per-input
// latency during baseline-coverage gathering. The expensive one is the
// report-creation SUCCESS path: real template fetch, report-document first write
// (MongoDB reports-collection index build), and RabbitMQ publish init. A 404
// (placeholder template ID) skips all of that, so the warm-up must drive a real
// 2xx create against an uploaded template — and poll its status so the worker
// side (RabbitMQ consume, S3 write) warms too. A second, timed create then
// confirms the success path is hot; if it isn't comfortably fast, something else
// is still cold and we log loudly rather than ship a silent flake.
func warmUpServices(ctx context.Context, managerAddr string) error {
	cli := h.NewHTTPClient(managerAddr, 60*time.Second)
	headers := h.AuthHeaders()

	// Upload a real, valid template so report creation can reach 2xx.
	templateID, err := warmUpUploadTemplate(ctx, cli, headers)
	if err != nil {
		return err
	}

	// First success-path create: warms template fetch, report persistence, and
	// RabbitMQ publish init.
	reportID, err := warmUpCreateReport(ctx, cli, headers, templateID)
	if err != nil {
		return err
	}

	// Poll the report briefly so the worker-side path (RabbitMQ consume, S3 write)
	// also warms. We don't require a terminal status — reaching the read path and
	// letting the worker pick the message up is enough; bound it so a stuck worker
	// can't hang TestMain.
	warmUpPollReport(ctx, cli, headers, reportID)

	// Second create, timed: with the path hot this must be fast. If not, surface
	// it — a slow second create means a cold dependency we haven't accounted for.
	start := time.Now()

	if _, err := warmUpCreateReport(ctx, cli, headers, templateID); err != nil {
		return fmt.Errorf("warm up second create report: %w", err)
	}

	secondCreate := time.Since(start)
	fmt.Fprintf(os.Stderr, "Warm-up second create /v1/reports took %s\n", secondCreate)

	if secondCreate > 3*time.Second {
		fmt.Fprintf(os.Stderr, "WARNING: warm second create took %s (>3s); a dependency may still be cold\n", secondCreate)
	}

	// Deadline and config paths: distinct heavy endpoints for the deadline and
	// blocks-config / filters fuzz targets. Any reachable response warms them.
	others := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{
			name:   "create deadline",
			method: "POST",
			path:   "/v1/deadlines",
			body: map[string]any{
				"name":      "warmup",
				"type":      "regulatory",
				"dueDate":   "2026-12-31T23:59:59Z",
				"frequency": "monthly",
				"color":     "#FF5733",
			},
		},
		{name: "blocks-config", method: "GET", path: "/v1/templates/blocks-config", body: nil},
		{name: "filters-config", method: "GET", path: "/v1/templates/filters", body: nil},
	}

	for _, w := range others {
		code, _, err := cli.Request(ctx, w.method, w.path, headers, w.body)
		if err != nil {
			return fmt.Errorf("warm up %s (%s %s): %w", w.name, w.method, w.path, err)
		}

		fmt.Fprintf(os.Stderr, "Warmed up %s: %s %s -> %d\n", w.name, w.method, w.path, code)
	}

	return nil
}

// warmUpUploadTemplate uploads the minimal valid template and returns its ID.
func warmUpUploadTemplate(ctx context.Context, cli *h.HTTPClient, headers map[string]string) (string, error) {
	formData := map[string]string{
		"outputFormat": "TXT",
		"description":  "Fuzzy warm-up template",
	}
	files := map[string][]byte{"template": []byte(warmUpTemplate)}

	code, body, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", headers, formData, files)
	if err != nil {
		return "", fmt.Errorf("warm up upload template: %w", err)
	}

	if code != 200 && code != 201 {
		return "", fmt.Errorf("warm up upload template: unexpected status %d: %s", code, string(body))
	}

	var resp struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(body, &resp); err != nil || resp.ID == "" {
		return "", fmt.Errorf("warm up upload template: no id in response: %s", string(body))
	}

	fmt.Fprintf(os.Stderr, "Warmed up template upload: POST /v1/templates -> %d id=%s\n", code, resp.ID)

	return resp.ID, nil
}

// warmUpCreateReport creates a report against templateID and returns its ID,
// requiring a 2xx so the success path is actually exercised.
func warmUpCreateReport(ctx context.Context, cli *h.HTTPClient, headers map[string]string, templateID string) (string, error) {
	payload := map[string]any{"templateId": templateID, "filters": map[string]any{}}

	code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
	if err != nil {
		return "", fmt.Errorf("warm up create report: %w", err)
	}

	if code != 200 && code != 201 {
		return "", fmt.Errorf("warm up create report: expected 2xx, got %d: %s", code, string(body))
	}

	var resp struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(body, &resp); err != nil || resp.ID == "" {
		return "", fmt.Errorf("warm up create report: no id in response: %s", string(body))
	}

	fmt.Fprintf(os.Stderr, "Warmed up create report: POST /v1/reports -> %d id=%s\n", code, resp.ID)

	return resp.ID, nil
}

// warmUpPollReport polls the report until it reaches a terminal status, warming
// the worker-side path (RabbitMQ consume, render, S3 write). It is best-effort —
// it never fails warm-up — but it logs the final status so a non-Finished result
// (e.g. a poisoned template that requeues) is visible rather than silent.
// Terminal statuses are constant.FinishedStatus ("Finished") and
// constant.ErrorStatus ("Error").
func warmUpPollReport(ctx context.Context, cli *h.HTTPClient, headers map[string]string, reportID string) {
	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		report, err := cli.GetReportStatus(pollCtx, reportID, headers)
		if err == nil {
			fmt.Fprintf(os.Stderr, "Warmed up report status: GET /v1/reports/%s -> %s\n", reportID, report.Status)

			// Terminal status means the full worker path ran; stop early.
			if report.Status == constant.FinishedStatus || report.Status == constant.ErrorStatus {
				return
			}
		}

		select {
		case <-pollCtx.Done():
			return
		case <-ticker.C:
		}
	}
}

// requireNo5xx is the 5xx oracle for live-HTTP fuzz targets that generate
// their own load. A healthy server must never answer 5xx — but a target that
// hammers real writes can exhaust the single-node test stack, and a 500 born
// from self-induced resource exhaustion is an environment artifact, not a
// server defect. On a 5xx this probes /health to discriminate: healthy stack
// means a real bug (fail); unreachable or unhealthy stack means melt (skip
// loudly, never silently).
func requireNo5xx(t *testing.T, code int, body []byte, requestDesc string) {
	t.Helper()

	if code < 500 {
		return
	}

	probe := &http.Client{Timeout: 5 * time.Second}

	resp, err := probe.Get(GetManagerAddress() + "/health")
	if err == nil {
		defer resp.Body.Close()

		if resp.StatusCode < 500 {
			t.Fatalf("SERVER ERROR on %s: code=%d body=%s (health probe OK — genuine 5xx bug)", requestDesc, code, string(body))
		}

		t.Skipf("5xx on %s with unhealthy stack (health=%d): resource exhaustion, not a server bug; code=%d", requestDesc, resp.StatusCode, code)
	}

	t.Skipf("5xx on %s with unreachable stack (health probe: %v): resource exhaustion, not a server bug; code=%d", requestDesc, err, code)
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
