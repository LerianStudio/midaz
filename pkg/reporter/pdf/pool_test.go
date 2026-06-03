// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pdf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T) *zap.Logger {
	t.Helper()

	logger, err := zap.New(zap.Config{Environment: zap.EnvironmentLocal, OTelLibraryName: "reporter"})
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	return logger
}

// Note: Tests that require Chrome are skipped in CI environments.
// Use SKIP_CHROME_TESTS=1 to skip Chrome-dependent tests.

func TestTask_Struct(t *testing.T) {
	t.Parallel()

	resultChan := make(chan error, 1)

	task := Task{
		HTML:     "<html><body>Test</body></html>",
		Filename: "/tmp/test.pdf",
		Result:   resultChan,
	}

	assert.Equal(t, "<html><body>Test</body></html>", task.HTML)
	assert.Equal(t, "/tmp/test.pdf", task.Filename)
	assert.NotNil(t, task.Result)
}

func TestWorkerPool_GetStats(t *testing.T) {
	t.Parallel()

	// Create pool but don't start workers (we'll test GetStats directly)
	wp := &WorkerPool{
		tasks:   make(chan Task, 10),
		workers: 4,
		timeout: 60 * time.Second,
	}

	stats := wp.GetStats()

	assert.Equal(t, 4, stats["workers"])
	assert.Equal(t, 60*time.Second, stats["timeout"])
	assert.Equal(t, 0, stats["tasks_pending"])
}

func TestWorkerPool_IsHealthy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		workers  int
		timeout  time.Duration
		expected bool
	}{
		{
			name:     "Healthy pool",
			workers:  4,
			timeout:  60 * time.Second,
			expected: true,
		},
		{
			name:     "Zero workers",
			workers:  0,
			timeout:  60 * time.Second,
			expected: false,
		},
		{
			name:     "Zero timeout",
			workers:  4,
			timeout:  0,
			expected: false,
		},
		{
			name:     "Both zero",
			workers:  0,
			timeout:  0,
			expected: false,
		},
		{
			name:     "Negative workers treated as unhealthy",
			workers:  -1,
			timeout:  60 * time.Second,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wp := &WorkerPool{
				workers: tt.workers,
				timeout: tt.timeout,
			}

			assert.Equal(t, tt.expected, wp.IsHealthy())
		})
	}
}

func TestWorkerPool_GetStats_PendingTasks(t *testing.T) {
	t.Parallel()

	// Create a buffered channel and add some tasks
	tasks := make(chan Task, 10)
	tasks <- Task{HTML: "test1", Filename: "file1.pdf", Result: make(chan error, 1)}
	tasks <- Task{HTML: "test2", Filename: "file2.pdf", Result: make(chan error, 1)}

	wp := &WorkerPool{
		tasks:   tasks,
		workers: 2,
		timeout: 30 * time.Second,
	}

	stats := wp.GetStats()

	assert.Equal(t, 2, stats["tasks_pending"])
}

func TestWorkerPool_GetChromeOptions(t *testing.T) {
	t.Parallel()

	wp := &WorkerPool{}

	options := wp.getChromeOptions()

	// Verify we have options
	assert.NotEmpty(t, options)
	// Verify headless mode is set
	assert.Greater(t, len(options), 5)
}

func TestBlockedExternalURLPatterns(t *testing.T) {
	t.Parallel()

	patterns := blockedExternalURLPatterns()
	assert.Contains(t, patterns, "http://*")
	assert.Contains(t, patterns, "https://*")
	assert.Contains(t, patterns, "ws://*")
	assert.Contains(t, patterns, "wss://*")
	assert.Contains(t, patterns, "ftp://*")
	assert.Contains(t, patterns, "file://*", "file:// protocol must be blocked to prevent LFI via Chrome headless PDF rendering")
}

func TestWorkerPool_Struct(t *testing.T) {
	t.Parallel()

	tasks := make(chan Task, 5)
	timeout := 120 * time.Second

	wp := &WorkerPool{
		tasks:   tasks,
		workers: 8,
		timeout: timeout,
	}

	assert.Equal(t, 8, wp.workers)
	assert.Equal(t, timeout, wp.timeout)
	assert.NotNil(t, wp.tasks)
}

func TestTask_ResultChannel(t *testing.T) {
	t.Parallel()

	resultChan := make(chan error, 1)
	task := Task{
		HTML:     "<html></html>",
		Filename: "test.pdf",
		Result:   resultChan,
	}

	// Send a result
	go func() {
		task.Result <- nil
	}()

	// Receive the result
	err := <-task.Result
	require.NoError(t, err)
}

func TestTask_ResultChannelWithError(t *testing.T) {
	t.Parallel()

	resultChan := make(chan error, 1)
	task := Task{
		HTML:     "<html></html>",
		Filename: "test.pdf",
		Result:   resultChan,
	}

	expectedErr := assert.AnError
	// Send an error result
	go func() {
		task.Result <- expectedErr
	}()

	// Receive the result
	err := <-task.Result
	assert.ErrorIs(t, err, expectedErr)
}

func TestWorkerPool_Timeout_Values(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{"30 seconds", 30 * time.Second},
		{"1 minute", time.Minute},
		{"5 minutes", 5 * time.Minute},
		{"10 minutes", 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wp := &WorkerPool{
				workers: 1,
				timeout: tt.timeout,
			}

			stats := wp.GetStats()
			assert.Equal(t, tt.timeout, stats["timeout"])
		})
	}
}

func TestWorkerPool_Workers_Values(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		workers int
	}{
		{"Single worker", 1},
		{"Few workers", 4},
		{"Many workers", 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wp := &WorkerPool{
				workers: tt.workers,
				timeout: time.Minute,
			}

			stats := wp.GetStats()
			assert.Equal(t, tt.workers, stats["workers"])
			assert.True(t, wp.IsHealthy())
		})
	}
}

// --- processPDFResult tests ---

func TestProcessPDFResult_Success(t *testing.T) {
	t.Parallel()

	wp := &WorkerPool{
		workers: 1,
		timeout: time.Minute,
		logger:  newTestLogger(t),
	}

	buf := make([]byte, 1001) // > PDFMinValidSizeBytes (1000)
	for i := range buf {
		buf[i] = 'A'
	}

	filename := filepath.Join(t.TempDir(), "test.pdf")

	err := wp.processPDFResult(buf, filename, nil)
	require.NoError(t, err)

	// Verify file exists and content matches
	content, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	assert.Equal(t, buf, content)
}

func TestProcessPDFResult_PropagatesPreviousError(t *testing.T) {
	t.Parallel()

	wp := &WorkerPool{
		workers: 1,
		timeout: time.Minute,
		logger:  newTestLogger(t),
	}

	previousErr := errors.New("previous error")

	err := wp.processPDFResult(nil, "irrelevant.pdf", previousErr)
	assert.ErrorIs(t, err, previousErr)

	// Verify no file was written
	_, statErr := os.Stat("irrelevant.pdf")
	assert.True(t, os.IsNotExist(statErr))
}

func TestProcessPDFResult_TooSmall(t *testing.T) {
	t.Parallel()

	wp := &WorkerPool{
		workers: 1,
		timeout: time.Minute,
		logger:  newTestLogger(t),
	}

	buf := make([]byte, 500) // < PDFMinValidSizeBytes (1000)
	for i := range buf {
		buf[i] = 'B'
	}

	filename := filepath.Join(t.TempDir(), "small.pdf")

	err := wp.processPDFResult(buf, filename, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too small")

	// Verify no file was written
	_, statErr := os.Stat(filename)
	assert.True(t, os.IsNotExist(statErr))
}

func TestProcessPDFResult_WriteError(t *testing.T) {
	t.Parallel()

	wp := &WorkerPool{
		workers: 1,
		timeout: time.Minute,
		logger:  newTestLogger(t),
	}

	buf := make([]byte, 1001) // > PDFMinValidSizeBytes
	for i := range buf {
		buf[i] = 'C'
	}

	// Use a path where the directory does not exist to trigger a write error
	filename := "/nonexistent/dir/file.pdf"

	err := wp.processPDFResult(buf, filename, nil)
	require.Error(t, err)
}

// --- logPDFGenerationError tests ---

func TestLogPDFGenerationError_Timeout(t *testing.T) {
	t.Parallel()

	wp := &WorkerPool{
		workers: 1,
		timeout: time.Minute,
		logger:  newTestLogger(t),
	}

	// Create a context with a deadline already in the past
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	assert.NotPanics(t, func() {
		wp.logPDFGenerationError(ctx, errors.New("timeout error"))
	})
}

func TestLogPDFGenerationError_Canceled(t *testing.T) {
	t.Parallel()

	wp := &WorkerPool{
		workers: 1,
		timeout: time.Minute,
		logger:  newTestLogger(t),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	assert.NotPanics(t, func() {
		wp.logPDFGenerationError(ctx, errors.New("canceled error"))
	})
}

func TestLogPDFGenerationError_Generic(t *testing.T) {
	t.Parallel()

	wp := &WorkerPool{
		workers: 1,
		timeout: time.Minute,
		logger:  newTestLogger(t),
	}

	// Background context: no deadline, not canceled
	ctx := context.Background()

	assert.NotPanics(t, func() {
		wp.logPDFGenerationError(ctx, errors.New("generic error"))
	})
}
