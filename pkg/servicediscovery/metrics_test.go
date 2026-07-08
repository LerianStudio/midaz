// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"testing"
)

// stubRecorder is a local white-box stub used to assert orNop identity passthrough
// and that call sites can drive a plain (non-OTel) MetricsRecorder.
type stubRecorder struct {
	called bool
}

func (s *stubRecorder) RegisterInitiated(_ context.Context) { s.called = true }

func (s *stubRecorder) DeregisterResult(_ context.Context, _ string) { s.called = true }

func (s *stubRecorder) ResolveResult(_ context.Context, _, _ string, _ int64) { s.called = true }

func TestNopMetricsRecorder_SatisfiesInterfaceAndDoesNotPanic(t *testing.T) {
	var r MetricsRecorder = NopMetricsRecorder{}
	ctx := context.Background()

	// All methods must be callable without panic.
	r.RegisterInitiated(ctx)
	r.DeregisterResult(ctx, ResultOK)
	r.ResolveResult(ctx, "midaz-ledger", ResultResolved, 42)
}

func TestOrNop_NilReturnsNopRecorder(t *testing.T) {
	r := orNop(nil)
	if r == nil {
		t.Fatal("orNop(nil) returned nil; want non-nil no-op recorder")
	}

	if _, ok := r.(NopMetricsRecorder); !ok {
		t.Fatalf("orNop(nil) returned %T; want NopMetricsRecorder", r)
	}

	ctx := context.Background()
	r.RegisterInitiated(ctx)
	r.DeregisterResult(ctx, ResultError)
	r.ResolveResult(ctx, "midaz-crm", ResultFallback, 7)
}

func TestOrNop_NonNilReturnsSameRecorder(t *testing.T) {
	stub := &stubRecorder{}

	got := orNop(stub)

	gotStub, ok := got.(*stubRecorder)
	if !ok {
		t.Fatalf("orNop(stub) returned %T; want *stubRecorder", got)
	}

	if gotStub != stub {
		t.Fatal("orNop(stub) returned a different recorder; want the same instance")
	}

	got.RegisterInitiated(context.Background())
	if !stub.called {
		t.Fatal("call through orNop(stub) did not reach the underlying recorder")
	}
}

func TestMetricsResultConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "ResultOK", got: ResultOK, want: "ok"},
		{name: "ResultError", got: ResultError, want: "error"},
		{name: "ResultResolved", got: ResultResolved, want: "resolved"},
		{name: "ResultFallback", got: ResultFallback, want: "fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q; want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}
