// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"sync"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libStreaming "github.com/LerianStudio/lib-streaming/v2"
)

// captureLogger is a minimal libLog.Logger that records every Log call so a
// test can assert that a graceful-degradation WARN was emitted. It is safe for
// concurrent use so t.Parallel tests can share the type.
type captureLogger struct {
	mu      sync.Mutex
	entries []captureEntry
}

type captureEntry struct {
	level libLog.Level
	msg   string
}

func (c *captureLogger) Log(_ context.Context, level libLog.Level, msg string, _ ...libLog.Field) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = append(c.entries, captureEntry{level: level, msg: msg})
}

func (c *captureLogger) With(_ ...libLog.Field) libLog.Logger { return c }
func (c *captureLogger) WithGroup(_ string) libLog.Logger     { return c }
func (c *captureLogger) Enabled(_ libLog.Level) bool          { return true }
func (c *captureLogger) Sync(_ context.Context) error         { return nil }

func (c *captureLogger) warnCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := 0
	for _, e := range c.entries {
		if e.level == libLog.LevelWarn {
			n++
		}
	}

	return n
}

// TestBuildBillingSerializer_DisabledShortCircuitsToNil asserts that when the
// streaming master switch is off the builder returns a nil serializer without
// contacting the registry, propagating no error and never panicking.
func TestBuildBillingSerializer_DisabledShortCircuitsToNil(t *testing.T) {
	t.Parallel()

	got := buildBillingSerializer(context.Background(), libStreaming.Config{Enabled: false}, nil)
	if got != nil {
		t.Fatalf("expected nil serializer when streaming disabled, got %v", got)
	}
}

// TestBuildBillingSerializer_EmptyRegistryURLGracefullyNil asserts that an
// enabled config with no Schema Registry URL degrades gracefully: the fail-closed
// registry-client constructor error is swallowed into a nil serializer plus a
// single WARN, never a propagated error or a panic.
func TestBuildBillingSerializer_EmptyRegistryURLGracefullyNil(t *testing.T) {
	t.Parallel()

	logger := &captureLogger{}

	got := buildBillingSerializer(
		context.Background(),
		libStreaming.Config{Enabled: true, SchemaRegistryURL: ""},
		logger,
	)
	if got != nil {
		t.Fatalf("expected nil serializer when registry URL empty, got %v", got)
	}

	if logger.warnCount() != 1 {
		t.Fatalf("expected exactly one WARN on the graceful branch, got %d", logger.warnCount())
	}
}

// TestBuildBillingSerializerFromEnv_DisabledReturnsNil covers the env-reading
// wrapper: with the streaming master switch off it loads the config, sees
// Enabled=false, and degrades to a nil serializer without touching a registry.
// Not parallel: it sets an environment variable via t.Setenv.
func TestBuildBillingSerializerFromEnv_DisabledReturnsNil(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "false")

	got := buildBillingSerializerFromEnv(context.Background(), nil)
	if got != nil {
		t.Fatalf("expected nil serializer when streaming disabled via env, got %v", got)
	}
}

// TestBuildBillingSerializer_NilLoggerNoPanic asserts the builder tolerates a
// nil logger on its graceful branches without dereferencing it.
func TestBuildBillingSerializer_NilLoggerNoPanic(t *testing.T) {
	t.Parallel()

	got := buildBillingSerializer(
		context.Background(),
		libStreaming.Config{Enabled: true, SchemaRegistryURL: ""},
		nil,
	)
	if got != nil {
		t.Fatalf("expected nil serializer, got %v", got)
	}
}
