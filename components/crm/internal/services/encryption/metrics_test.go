// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-commons/v5/commons/opentelemetry/metrics"
)

// exerciseAllMethods calls every protectionMetrics emitter once. It is shared by
// the nil-receiver and nil-factory cases to assert the seam is a total no-op
// (never panics, never blocks) when no usable metrics factory is wired.
func exerciseAllMethods(ctx context.Context, m *protectionMetrics) {
	m.recordModeResolution(ctx, "envelope")
	m.recordStatus(ctx, "active")
	m.recordEncryptDecrypt(ctx, "envelope", "success", "")
	m.recordProviderOperation(ctx, "wrap", "vault", 12)
	m.recordProviderFailure(ctx, "wrap", "ErrProvisioningFailed")
	m.recordRegistryConflict(ctx)
	m.recordLegacyRead(ctx, "active")
	m.recordCache(ctx, "get_primitives", "hit")
}

// TestProtectionMetrics_NilReceiver_NoOp asserts every method is a safe no-op
// when invoked on a nil *protectionMetrics receiver. This is the telemetry-disabled
// path where the seam must never panic or block the parent operation.
func TestProtectionMetrics_NilReceiver_NoOp(t *testing.T) {
	t.Parallel()

	var m *protectionMetrics // nil receiver

	exerciseAllMethods(context.Background(), m)
}

// TestProtectionMetrics_NilFactory_NoOp asserts NewProtectionMetrics(nil) returns
// a usable value whose methods are all safe no-ops (no panic, no allocations beyond
// the struct itself). This is the canonical telemetry-disabled wiring.
func TestProtectionMetrics_NilFactory_NoOp(t *testing.T) {
	t.Parallel()

	m := NewProtectionMetrics(nil)
	if m == nil {
		t.Fatal("NewProtectionMetrics(nil) returned nil; expected a usable no-op value")
	}

	exerciseAllMethods(context.Background(), m)
}

// TestProtectionMetrics_NopFactory_NeverErrorsOrBlocks asserts that with a
// real (no-op-backed) MetricsFactory present, every emit path executes the
// counter/histogram branch without panicking or blocking. This exercises the
// emit branches the disabled-telemetry tests cannot reach, and proves emission
// stays best-effort and non-blocking even when a factory is wired.
func TestProtectionMetrics_NopFactory_NeverErrorsOrBlocks(t *testing.T) {
	t.Parallel()

	m := NewProtectionMetrics(metrics.NewNopFactory())
	if m == nil {
		t.Fatal("NewProtectionMetrics(factory) returned nil")
	}

	exerciseAllMethods(context.Background(), m)
}
