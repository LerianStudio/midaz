// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// providerSecretError carries text that resembles a raw KMS provider failure. It
// exists to prove the failure classifier never leaks raw provider error text into
// the error_code label.
var providerSecretError = errors.New("vault: permission denied for transit/keys/org-secret-keymaterial")

// scriptedUnwrapper is a per-call-controllable KeysetUnwrapper. The first call is
// the AEAD unwrap, the second is the MAC unwrap, matching fetchAndCache order.
type scriptedUnwrapper struct {
	aeadKeyset []byte
	macKeyset  []byte
	aeadErr    error
	macErr     error
	calls      int
}

func (s *scriptedUnwrapper) UnwrapKeyset(_ context.Context, _ string, _ string) ([]byte, error) {
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

	return s.macKeyset, nil
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
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	return NewKeysetManager(repo, unwrapper, nil, DefaultKeysetManagerConfig(), m)
}

// TestRecordProviderOperation_RecordsMillisecondValue asserts the provider
// timing histogram records the exact millisecond magnitude it is given, proving
// the metric is in milliseconds and that a non-trivial duration is NOT truncated
// to zero. The duration is injected deterministically (no time.Now / sleeping)
// so the assertion is stable.
func TestRecordProviderOperation_RecordsMillisecondValue(t *testing.T) {
	t.Parallel()

	m, spy := newRecordingMetrics(t)

	const injectedMs int64 = 150

	m.recordProviderOperation(context.Background(), providerOperationWrap, providerVault, injectedMs)

	wantAttr := map[string]string{"operation": "wrap", "provider": "vault"}

	n := spy.histogramCount(t, utils.CRMProtectionProviderOperationMs.Name, wantAttr)
	assert.Equal(t, 1, n, "exactly one timing sample recorded")

	sum := spy.histogramSum(t, utils.CRMProtectionProviderOperationMs.Name, wantAttr)
	assert.Equal(t, injectedMs, sum, "recorded value must equal the injected millisecond duration (not truncated to zero)")
	assert.Positive(t, sum, "a non-trivial sub-second duration must record a positive millisecond value")
}

// TestRecordProviderOperation_ZeroMs records a zero-millisecond duration to prove
// the >= 0 boundary is handled without panic and yields a zero-valued sample.
func TestRecordProviderOperation_ZeroMs(t *testing.T) {
	t.Parallel()

	m, spy := newRecordingMetrics(t)

	m.recordProviderOperation(context.Background(), providerOperationUnwrap, providerVault, 0)

	wantAttr := map[string]string{"operation": "unwrap", "provider": "vault"}

	assert.Equal(t, 1, spy.histogramCount(t, utils.CRMProtectionProviderOperationMs.Name, wantAttr),
		"a zero-millisecond duration still records one sample")
	assert.GreaterOrEqual(t, spy.histogramSum(t, utils.CRMProtectionProviderOperationMs.Name, wantAttr), int64(0))
}

// TestFetchAndCache_UnwrapSuccess_RecordsProviderTiming asserts that a fully
// successful read path records exactly two unwrap timing samples
// (operation=unwrap, provider=vault) and no provider failures.
func TestFetchAndCache_UnwrapSuccess_RecordsProviderTiming(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	m, spy := newRecordingMetrics(t)

	km := newKeysetManagerForMetrics(&scriptedUnwrapper{aeadKeyset: aeadBytes, macKeyset: macBytes}, m)

	_, _, _, _, err := km.GetPrimitives(context.Background(), "org-metrics")
	require.NoError(t, err, "behavior unchanged: successful unwrap returns no error")

	n := spy.histogramCount(t, utils.CRMProtectionProviderOperationMs.Name,
		map[string]string{"operation": "unwrap", "provider": "vault"})
	assert.Equal(t, 2, n, "AEAD + MAC unwrap each record one timing sample")

	assert.Equal(t, 0, spy.counterCount(t, utils.CRMProtectionProviderOperationFailuresTotal.Name),
		"no provider failures on success")
}

// TestFetchAndCache_AEADSuccessMACFail_RecordsTimingAndFailure asserts the named
// edge case: AEAD unwrap succeeds (one success timing) and MAC unwrap fails
// (timing still recorded + one failure with the stable classifier, never raw
// provider text). Return value/behavior unchanged.
func TestFetchAndCache_AEADSuccessMACFail_RecordsTimingAndFailure(t *testing.T) {
	t.Parallel()

	aeadBytes, _ := generateTestKeysets(t)

	m, spy := newRecordingMetrics(t)

	km := newKeysetManagerForMetrics(&scriptedUnwrapper{aeadKeyset: aeadBytes, macErr: providerSecretError}, m)

	_, _, _, _, err := km.GetPrimitives(context.Background(), "org-metrics")
	require.Error(t, err, "behavior unchanged: MAC unwrap failure propagates")
	assert.Contains(t, err.Error(), "failed to unwrap MAC keyset", "wrapped error message unchanged")

	// Timing recorded for both calls (success AEAD + failed MAC).
	n := spy.histogramCount(t, utils.CRMProtectionProviderOperationMs.Name,
		map[string]string{"operation": "unwrap", "provider": "vault"})
	assert.Equal(t, 2, n, "timing recorded even for the failed MAC unwrap call")

	// Exactly one failure, classified as unwrap_mac_failed.
	v, fn := spy.counterValue(t, utils.CRMProtectionProviderOperationFailuresTotal.Name,
		map[string]string{"operation": "unwrap", "error_code": "unwrap_mac_failed"})
	assert.Equal(t, int64(1), v, "one unwrap_mac_failed failure")
	assert.Equal(t, 1, fn)

	// Stable classifier only: the raw provider error string MUST NOT appear as a label.
	for _, s := range spy.collect(t) {
		if s.name != utils.CRMProtectionProviderOperationFailuresTotal.Name {
			continue
		}

		assert.NotContains(t, s.attrs["error_code"], "permission denied",
			"raw provider error text MUST NOT be used as a label")
		assert.NotContains(t, s.attrs["error_code"], "transit/keys",
			"raw provider error text MUST NOT be used as a label")
	}
}

// TestFetchAndCache_AEADFail_RecordsTimingAndFailure asserts an AEAD unwrap
// failure records one timing sample and one unwrap_aead_failed failure.
func TestFetchAndCache_AEADFail_RecordsTimingAndFailure(t *testing.T) {
	t.Parallel()

	m, spy := newRecordingMetrics(t)

	km := newKeysetManagerForMetrics(&scriptedUnwrapper{aeadErr: providerSecretError}, m)

	_, _, _, _, err := km.GetPrimitives(context.Background(), "org-metrics")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unwrap AEAD keyset")

	n := spy.histogramCount(t, utils.CRMProtectionProviderOperationMs.Name,
		map[string]string{"operation": "unwrap", "provider": "vault"})
	assert.Equal(t, 1, n, "timing recorded for the failed AEAD unwrap; MAC unwrap never runs")

	v, fn := spy.counterValue(t, utils.CRMProtectionProviderOperationFailuresTotal.Name,
		map[string]string{"operation": "unwrap", "error_code": "unwrap_aead_failed"})
	assert.Equal(t, int64(1), v, "one unwrap_aead_failed failure")
	assert.Equal(t, 1, fn)
}

// TestFetchAndCache_NilFactory_NoPanic asserts the read path is a safe no-op for
// provider metrics when telemetry is disabled, keeping behavior identical.
func TestFetchAndCache_NilFactory_NoPanic(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	km := newKeysetManagerForMetrics(&scriptedUnwrapper{aeadKeyset: aeadBytes, macKeyset: macBytes}, NewProtectionMetrics(nil))

	_, _, _, _, err := km.GetPrimitives(context.Background(), "org-metrics")
	require.NoError(t, err)
}

// TestProvision_WrapSuccess_RecordsProviderTiming asserts that the provisioning
// path records two wrap timing samples (operation=wrap, provider=vault), one per
// keyset generation (AEAD + MAC), and no failures.
func TestProvision_WrapSuccess_RecordsProviderTiming(t *testing.T) {
	t.Parallel()

	m, spy := newRecordingMetrics(t)

	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	gen := newFakeKeysetGenerator()

	svc := NewProvisioningService(keysetRepo, registryRepo, gen, DefaultProvisioningConfig(), newSpyAuditWriter(), m)

	_, err := svc.Provision(context.Background(), ProvisionInput{
		TenantID:       "tenant-1",
		OrganizationID: "org-metrics",
		Actor:          "tester",
		Reason:         "unit test",
	})
	require.NoError(t, err, "behavior unchanged: provisioning succeeds")

	n := spy.histogramCount(t, utils.CRMProtectionProviderOperationMs.Name,
		map[string]string{"operation": "wrap", "provider": "vault"})
	assert.Equal(t, 2, n, "AEAD + MAC wrap each record one timing sample")

	assert.Equal(t, 0, spy.counterCount(t, utils.CRMProtectionProviderOperationFailuresTotal.Name),
		"no provider failures on success")
}

// TestProvision_AEADWrapFail_RecordsTimingAndFailure asserts the provisioning
// path records a wrap failure with the stable classifier when AEAD generation
// fails, and never leaks raw provider error text. Return value unchanged.
func TestProvision_AEADWrapFail_RecordsTimingAndFailure(t *testing.T) {
	t.Parallel()

	m, spy := newRecordingMetrics(t)

	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	gen := newFakeKeysetGenerator()
	gen.aeadErr = providerSecretError

	svc := NewProvisioningService(keysetRepo, registryRepo, gen, DefaultProvisioningConfig(), newSpyAuditWriter(), m)

	_, err := svc.Provision(context.Background(), ProvisionInput{
		TenantID:       "tenant-1",
		OrganizationID: "org-metrics",
		Actor:          "tester",
		Reason:         "unit test",
	})
	require.Error(t, err, "behavior unchanged: provisioning failure propagates")

	n := spy.histogramCount(t, utils.CRMProtectionProviderOperationMs.Name,
		map[string]string{"operation": "wrap", "provider": "vault"})
	assert.Equal(t, 1, n, "timing recorded for the failed AEAD wrap; MAC wrap never runs")

	v, fn := spy.counterValue(t, utils.CRMProtectionProviderOperationFailuresTotal.Name,
		map[string]string{"operation": "wrap", "error_code": "wrap_aead_failed"})
	assert.Equal(t, int64(1), v, "one wrap_aead_failed failure")
	assert.Equal(t, 1, fn)

	for _, s := range spy.collect(t) {
		if s.name != utils.CRMProtectionProviderOperationFailuresTotal.Name {
			continue
		}

		assert.NotContains(t, s.attrs["error_code"], "permission denied",
			"raw provider error text MUST NOT be used as a label")
	}
}

// TestProvision_MACWrapFail_RecordsTimingAndFailure asserts a MAC generation
// failure records two wrap timings (AEAD success + failed MAC) and one
// wrap_mac_failed failure.
func TestProvision_MACWrapFail_RecordsTimingAndFailure(t *testing.T) {
	t.Parallel()

	m, spy := newRecordingMetrics(t)

	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	gen := newFakeKeysetGenerator()
	gen.macErr = providerSecretError

	svc := NewProvisioningService(keysetRepo, registryRepo, gen, DefaultProvisioningConfig(), newSpyAuditWriter(), m)

	_, err := svc.Provision(context.Background(), ProvisionInput{
		TenantID:       "tenant-1",
		OrganizationID: "org-metrics",
		Actor:          "tester",
		Reason:         "unit test",
	})
	require.Error(t, err)

	n := spy.histogramCount(t, utils.CRMProtectionProviderOperationMs.Name,
		map[string]string{"operation": "wrap", "provider": "vault"})
	assert.Equal(t, 2, n, "AEAD success + failed MAC each record a timing sample")

	v, fn := spy.counterValue(t, utils.CRMProtectionProviderOperationFailuresTotal.Name,
		map[string]string{"operation": "wrap", "error_code": "wrap_mac_failed"})
	assert.Equal(t, int64(1), v, "one wrap_mac_failed failure")
	assert.Equal(t, 1, fn)
}
