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
	level  libLog.Level
	msg    string
	fields []libLog.Field
}

func (c *captureLogger) Log(_ context.Context, level libLog.Level, msg string, fields ...libLog.Field) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = append(c.entries, captureEntry{
		level:  level,
		msg:    msg,
		fields: append([]libLog.Field(nil), fields...),
	})
}

func (c *captureLogger) With(_ ...libLog.Field) libLog.Logger { return c }
func (c *captureLogger) WithGroup(_ string) libLog.Logger     { return c }
func (c *captureLogger) Enabled(_ libLog.Level) bool          { return true }
func (c *captureLogger) Sync(_ context.Context) error         { return nil }

func (c *captureLogger) warnCount() int {
	return len(c.warnMessages())
}

func (c *captureLogger) warnMessages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var msgs []string

	for _, e := range c.entries {
		if e.level == libLog.LevelWarn {
			msgs = append(msgs, e.msg)
		}
	}

	return msgs
}

// warnErrMessages returns the `.Error()` string of every WARN entry's "error"
// field, in order. It lets a test assert WHICH error drove a graceful-
// degradation WARN — e.g. the guard's raw ctx error ("context canceled")
// versus the wrapped registry error the without-guard path would produce.
func (c *captureLogger) warnErrMessages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var msgs []string

	for _, e := range c.entries {
		if e.level != libLog.LevelWarn {
			continue
		}

		for _, f := range e.fields {
			if f.Key != "error" {
				continue
			}

			if err, ok := f.Value.(error); ok && err != nil {
				msgs = append(msgs, err.Error())
			}
		}
	}

	return msgs
}

// TestBuildBillingSerializer exercises the network-free decision core across its
// graceful-degradation branches: every branch yields a nil serializer, never a
// propagated error or a panic. The disabled case additionally proves the builder
// neither warns nor contacts the registry (wantWarnCount 0); the empty-URL case
// proves the fail-closed registry-client error is swallowed into a single, named
// WARN; the nil-logger case proves the graceful branch tolerates a nil logger.
func TestBuildBillingSerializer(t *testing.T) {
	t.Parallel()

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name          string
		ctx           context.Context
		cfg           libStreaming.Config
		logger        libLog.Logger
		wantWarnCount int
		wantWarnMsg   string
		wantWarnErr   string
	}{
		{
			name:          "disabled short-circuits to nil without registry contact",
			cfg:           libStreaming.Config{Enabled: false},
			logger:        &captureLogger{},
			wantWarnCount: 0,
		},
		{
			name:          "empty registry url degrades to nil with one warn",
			cfg:           libStreaming.Config{Enabled: true, SchemaRegistryURL: ""},
			logger:        &captureLogger{},
			wantWarnCount: 1,
			wantWarnMsg:   "Billing serializer disabled",
		},
		{
			name:   "nil logger does not panic on graceful branch",
			cfg:    libStreaming.Config{Enabled: true, SchemaRegistryURL: ""},
			logger: nil,
		},
		{
			name:          "canceled context short-circuits before registry contact",
			ctx:           canceledCtx,
			cfg:           libStreaming.Config{Enabled: true, SchemaRegistryURL: "http://schema-registry.invalid:8081"},
			logger:        &captureLogger{},
			wantWarnCount: 1,
			wantWarnMsg:   "Billing serializer disabled",
			// The guard warns with the raw ctx.Err() ("context canceled"),
			// proving it short-circuited BEFORE any registry contact. Without
			// the guard the same inputs still degrade to nil+1 WARN, but the
			// error is the wrapped registry round-trip failure — so this exact
			// message is what distinguishes the guarded path.
			wantWarnErr: context.Canceled.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			got := buildBillingSerializer(ctx, tt.cfg, tt.logger)
			if got != nil {
				t.Fatalf("expected nil serializer, got %v", got)
			}

			cl, ok := tt.logger.(*captureLogger)
			if !ok {
				// nil-logger case: the nil-serializer assertion above already
				// proves no panic on the graceful branch.
				return
			}

			if cl.warnCount() != tt.wantWarnCount {
				t.Fatalf("expected %d WARN(s), got %d", tt.wantWarnCount, cl.warnCount())
			}

			if tt.wantWarnMsg != "" {
				msgs := cl.warnMessages()
				if len(msgs) != 1 || msgs[0] != tt.wantWarnMsg {
					t.Fatalf("expected single WARN %q, got %v", tt.wantWarnMsg, msgs)
				}
			}

			if tt.wantWarnErr != "" {
				errMsgs := cl.warnErrMessages()
				if len(errMsgs) != 1 || errMsgs[0] != tt.wantWarnErr {
					t.Fatalf("expected single WARN error %q (guard short-circuit), got %v",
						tt.wantWarnErr, errMsgs)
				}
			}
		})
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
