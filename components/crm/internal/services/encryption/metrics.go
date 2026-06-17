// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"

	"github.com/LerianStudio/lib-commons/v5/commons/opentelemetry/metrics"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"go.opentelemetry.io/otel/attribute"
)

// Provider-operation label vocabulary. These are the only values that may appear
// on the provider operation metrics. They are short, stable classifiers and MUST
// NOT be derived from raw provider error text, keysets, credentials, or PII.
const (
	// providerVault is the KMS provider label. Envelope encryption is Vault-only
	// today; this becomes dynamic when other KMS providers land.
	providerVault = "vault"

	// providerOperationWrap / providerOperationUnwrap are the operation labels.
	providerOperationWrap   = "wrap"
	providerOperationUnwrap = "unwrap"

	// Stable failure classifiers for the provider operation failures counter.
	errorCodeUnwrapAEADFailed = "unwrap_aead_failed"
	errorCodeUnwrapPRFFailed  = "unwrap_prf_failed"
	errorCodeWrapAEADFailed   = "wrap_aead_failed"
	errorCodeWrapPRFFailed    = "wrap_prf_failed"
)

// protectionMetrics is the nil-safe emission seam for CRM field-protection
// telemetry. It centralizes the metricsFactory guard pattern (mirroring
// bootstrap.ReadyzHandler) so service methods never touch the factory directly.
//
// Every method MUST be safe on a nil receiver and on a nil factory: when
// telemetry is disabled the factory is nil and all methods no-op without
// allocating. Emission is best-effort and MUST NOT error or block the parent
// operation (encryption/provisioning): factory errors are swallowed exactly as
// readyz does. Labels carry only closed-vocabulary values (mode, status, path,
// outcome, error_type, operation, provider, error_code, organization_status,
// result) and never plaintext, keysets, credentials, PII, or financial values.
type protectionMetrics struct {
	factory *metrics.MetricsFactory
}

// NewProtectionMetrics builds the protection metrics seam. A nil factory
// (telemetry disabled) yields a value whose methods are all no-ops, so callers
// can always inject a non-nil *protectionMetrics regardless of telemetry state.
func NewProtectionMetrics(factory *metrics.MetricsFactory) *protectionMetrics {
	return &protectionMetrics{factory: factory}
}

// emitCounter adds 1 to the named counter with the given attributes, swallowing
// the factory error like readyz. It is the single guarded counter path.
func (m *protectionMetrics) emitCounter(ctx context.Context, metric metrics.Metric, attrs ...attribute.KeyValue) {
	if m == nil || m.factory == nil {
		return
	}

	if counter, err := m.factory.Counter(metric); err == nil {
		_ = counter.WithAttributes(attrs...).Add(ctx, 1)
	}
}

// emitHistogram records value into the named histogram with the given
// attributes, swallowing the factory error like readyz (mirroring emitCounter).
// It is the single guarded histogram path. lib-commons v5 only exposes an
// int64-backed histogram, so value is an int64.
func (m *protectionMetrics) emitHistogram(ctx context.Context, metric metrics.Metric, value int64, attrs ...attribute.KeyValue) {
	if m == nil || m.factory == nil {
		return
	}

	if histogram, err := m.factory.Histogram(metric); err == nil {
		_ = histogram.WithAttributes(attrs...).Record(ctx, value)
	}
}

// recordModeResolution records a protection mode resolution (legacy/envelope).
func (m *protectionMetrics) recordModeResolution(ctx context.Context, mode string) {
	m.emitCounter(ctx, utils.CRMProtectionModeResolutionTotal,
		attribute.String("mode", mode))
}

// recordStatus records a protection status outcome.
func (m *protectionMetrics) recordStatus(ctx context.Context, status string) {
	m.emitCounter(ctx, utils.CRMProtectionStatusTotal,
		attribute.String("status", status))
}

// recordEncryptDecrypt records an encrypt/decrypt operation by path, outcome,
// and error type. errorType is empty on success.
func (m *protectionMetrics) recordEncryptDecrypt(ctx context.Context, path, outcome, errorType string) {
	m.emitCounter(ctx, utils.CRMProtectionEncryptDecryptTotal,
		attribute.String("path", path),
		attribute.String("outcome", outcome),
		attribute.String("error_type", errorType))
}

// recordProviderOperation records the duration of a provider wrap/unwrap
// operation in milliseconds. The factory error is swallowed like readyz; the
// underlying histogram is int64-backed, so milliseconds (not seconds) are
// recorded to avoid truncating sub-second KMS latencies to zero.
//
// provider is kept as a parameter even though every call site passes "vault"
// today: envelope encryption is Vault-only now and this becomes dynamic when
// other KMS providers land.
//
//nolint:unparam // provider is intentionally a parameter for future KMS providers
func (m *protectionMetrics) recordProviderOperation(ctx context.Context, operation, provider string, ms int64) {
	m.emitHistogram(ctx, utils.CRMProtectionProviderOperationMs, ms,
		attribute.String("operation", operation),
		attribute.String("provider", provider))
}

// recordProviderFailure records a provider operation failure by operation and
// error code.
func (m *protectionMetrics) recordProviderFailure(ctx context.Context, operation, errorCode string) {
	m.emitCounter(ctx, utils.CRMProtectionProviderOperationFailuresTotal,
		attribute.String("operation", operation),
		attribute.String("error_code", errorCode))
}

// recordRegistryConflict records a protection registry conflict (no labels).
//
// Emit site is deferred until conflict detection lands on the registry Update
// path (analogous to the deferred rotation_guard metric); the counter stays
// declared so the catalog is complete. Keep the nolint until then.
//
//nolint:unused // emit site deferred to the registry Update conflict path
func (m *protectionMetrics) recordRegistryConflict(ctx context.Context) {
	m.emitCounter(ctx, utils.CRMProtectionRegistryConflictTotal)
}

// recordLegacyRead records a legacy-path read by organization status.
func (m *protectionMetrics) recordLegacyRead(ctx context.Context, organizationStatus string) {
	m.emitCounter(ctx, utils.CRMProtectionLegacyReadTotal,
		attribute.String("organization_status", organizationStatus))
}

// recordCache records a protection cache lookup by operation and result
// (hit/miss).
func (m *protectionMetrics) recordCache(ctx context.Context, operation, result string) {
	m.emitCounter(ctx, utils.CRMProtectionCacheTotal,
		attribute.String("operation", operation),
		attribute.String("result", result))
}
