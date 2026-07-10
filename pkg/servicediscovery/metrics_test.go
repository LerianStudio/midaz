// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"testing"
)

// stubRecorder is a local white-box stub used to assert orNop identity passthrough
// and that call sites can drive a plain (non-OTel) MetricsRecorder. It tracks each
// method independently so future call-site tests can assert which method fired and
// with what arguments.
type stubRecorder struct {
	registerInitiatedCalls int
	deregisterResults      []string
	resolveResults         []resolveCall
}

type resolveCall struct {
	service    string
	result     string
	durationMs int64
}

func (s *stubRecorder) RegisterInitiated(_ context.Context) { s.registerInitiatedCalls++ }

func (s *stubRecorder) DeregisterResult(_ context.Context, result string) {
	s.deregisterResults = append(s.deregisterResults, result)
}

func (s *stubRecorder) ResolveResult(_ context.Context, service, result string, durationMs int64) {
	s.resolveResults = append(s.resolveResults, resolveCall{service: service, result: result, durationMs: durationMs})
}

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

	ctx := context.Background()
	got.RegisterInitiated(ctx)
	got.DeregisterResult(ctx, ResultError)
	got.ResolveResult(ctx, "plugin-auth", ResultFallback, 7)

	if stub.registerInitiatedCalls != 1 {
		t.Errorf("RegisterInitiated calls = %d; want 1 (call must reach the same instance)", stub.registerInitiatedCalls)
	}

	if len(stub.deregisterResults) != 1 || stub.deregisterResults[0] != ResultError {
		t.Errorf("DeregisterResult calls = %v; want [%q]", stub.deregisterResults, ResultError)
	}

	wantResolve := resolveCall{service: "plugin-auth", result: ResultFallback, durationMs: 7}
	if len(stub.resolveResults) != 1 || stub.resolveResults[0] != wantResolve {
		t.Errorf("ResolveResult calls = %v; want [%+v]", stub.resolveResults, wantResolve)
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
