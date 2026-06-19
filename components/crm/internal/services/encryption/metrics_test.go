// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/metrics"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestResolve_RepositoryError_Propagated asserts an unexpected repo error is
// returned unchanged.
func TestResolve_RepositoryError_Propagated(t *testing.T) {
	t.Parallel()

	repoErr := errors.New("database connection failed")
	repo := &fakeRegistryRepoForProtection{err: repoErr}

	resolver := NewProtectionStateResolver(repo, NewProtectionMetrics(nil))

	_, err := resolver.Resolve(context.Background(), "org-err")
	require.ErrorIs(t, err, repoErr)
}

// TestResolve_NilFactory_NoPanic asserts the resolve path is a safe no-op for
// metrics when telemetry is disabled (nil factory), keeping behavior identical.
func TestResolve_NilFactory_NoPanic(t *testing.T) {
	t.Parallel()

	repo := &fakeRegistryRepoForProtection{
		record: nil,
		err:    constant.ErrRegistryNotFound,
	}

	resolver := NewProtectionStateResolver(repo, NewProtectionMetrics(nil))

	state, err := resolver.Resolve(context.Background(), "org-nil")
	require.NoError(t, err)
	assert.Equal(t, crypto.EncryptionModeLegacy, state.Mode)
}

// scriptedUnwrapper is a per-call-controllable KeysetUnwrapper. The first call is
// the AEAD unwrap, the second is the PRF unwrap, matching fetchAndCache order.
type scriptedUnwrapper struct {
	aeadKeyset []byte
	prfKeyset  []byte
	aeadErr    error
	macErr     error
	calls      int
}

func (s *scriptedUnwrapper) UnwrapKeyset(_ context.Context, _ string, _ string, _ string) ([]byte, error) {
	s.calls++

	if s.calls == 1 {
		if s.aeadErr != nil {
			return nil, s.aeadErr
		}

		return s.aeadKeyset, nil
	}

	if s.macErr != nil {
		return nil, s.macErr
	}

	return s.prfKeyset, nil
}

// newKeysetManagerForMetrics builds a KeysetManager wired with the recording
// metrics seam and a scripted unwrapper. Provisioner is nil; the keyset is
// pre-seeded so fetchAndCache reaches the unwrap path directly.
func newKeysetManagerForMetrics(unwrapper KeysetUnwrapper, m *protectionMetrics) *KeysetManager {
	repo := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-metrics",
			KEKPath:           "org-org-metrics",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	return NewKeysetManager(repo, unwrapper, nil, DefaultKeysetManagerConfig(), m)
}

// TestFetchAndCache_NilFactory_NoPanic asserts the read path is a safe no-op for
// provider metrics when telemetry is disabled, keeping behavior identical.
func TestFetchAndCache_NilFactory_NoPanic(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	km := newKeysetManagerForMetrics(&scriptedUnwrapper{aeadKeyset: aeadBytes, prfKeyset: prfBytes}, NewProtectionMetrics(nil))

	_, err := km.GetActivePrimitives(context.Background(), "org-metrics")
	require.NoError(t, err)
}
