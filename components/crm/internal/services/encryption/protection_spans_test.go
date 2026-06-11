// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// metricsTestService bundles an EncryptionService with the recording metrics spy
// that captures its emissions, so span+metric assertions can share one wiring.
type metricsTestService struct {
	svc EncryptionService
	spy *recordingMetrics
}

// createEncryptionTestServiceWithMetrics mirrors createEncryptionTestService but
// wires a recording metrics spy into the service so encrypt/decrypt emissions are
// captured. Envelope mode forces the envelope path regardless of registry state.
func createEncryptionTestServiceWithMetrics(t *testing.T, state ProtectionState, legacyKeys *LegacyKeyMaterial, mode crypto.EncryptionMode) (*metricsTestService, *mmodel.OrganizationKeyset) {
	t.Helper()

	aeadBytes, macBytes, aeadKeyID, _ := generateServiceTestKeysets(t)

	keyset := &mmodel.OrganizationKeyset{
		TenantID:       state.TenantID,
		OrganizationID: state.OrganizationID,
		KEKPath:        "test-kek",
		WrappedKeyset:  "wrapped-aead",
		KeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
		WrappedHMACKeyset: "wrapped-hmac",
		HMACKeysetInfo: mmodel.KeysetInfo{
			PrimaryKeyID: aeadKeyID,
		},
	}

	keysetRepo := &serviceTestKeysetRepo{
		keysets: map[string]*mmodel.OrganizationKeyset{
			state.OrganizationID: keyset,
		},
	}

	unwrapper := &serviceTestKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	keysetManager := NewKeysetManager(keysetRepo, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			state.OrganizationID: {
				TenantID:       state.TenantID,
				OrganizationID: state.OrganizationID,
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: state.CanReadLegacy,
				CurrentVersion: state.CurrentKeysetVersion,
			},
		},
	}

	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))

	m, spy := newRecordingMetrics(t)
	svc := NewEncryptionService(stateResolver, keysetManager, keysetRepo, legacyKeys, m, mode)

	return &metricsTestService{svc: svc, spy: spy}, keyset
}

// --- Encrypt span + metric ------------------------------------------------

// TestEncrypt_EnvelopeMode_SpanAndMetric asserts encrypt_field span carries the
// org id + field NAME + path=envelope output attr, with a success encrypt metric
// and no sensitive attributes.
func TestEncrypt_EnvelopeMode_SpanAndMetric(t *testing.T) {
	t.Parallel()

	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-enc",
		TenantID:             "tenant-enc",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestServiceWithMetrics(t, state, legacyKeys, crypto.EncryptionModeEnvelope)

	ctx, recorder := newRecordingTracerContext(t, context.Background())

	fieldCtx := FieldContext{
		TenantID:       "tenant-enc",
		OrganizationID: "org-enc",
		RecordID:       "record-1",
		FieldName:      "tax_id",
	}

	const plaintext = "123-45-6789"

	ciphertext, err := svc.svc.Encrypt(ctx, fieldCtx, plaintext)
	require.NoError(t, err)
	require.True(t, HasEnvelopeMarker(ciphertext))

	span := spanByName(recorder, "service.protection.encrypt_field")
	require.NotNil(t, span, "encrypt_field span MUST be recorded")

	org, ok := spanStringAttr(span, "app.request.organization_id")
	require.True(t, ok)
	assert.Equal(t, "org-enc", org)

	field, ok := spanStringAttr(span, "app.request.field")
	require.True(t, ok, "field NAME MUST be present")
	assert.Equal(t, "tax_id", field)

	path, ok := spanStringAttr(span, "app.protection.path")
	require.True(t, ok)
	assert.Equal(t, "envelope", path)

	assert.False(t, spanHasError(span))

	for _, kv := range span.Attributes() {
		v := kv.Value.AsString()
		assert.NotContains(t, v, plaintext, "span value MUST NOT leak plaintext")
		assert.NotContains(t, v, ciphertext, "span value MUST NOT leak ciphertext")
	}

	v, n := svc.spy.counterValue(t, utils.CRMProtectionEncryptDecryptTotal.Name,
		map[string]string{"path": "envelope", "outcome": "success", "error_type": ""})
	assert.Equal(t, int64(1), v)
	assert.Equal(t, 1, n)
}

// TestEncrypt_LegacyMode_SpanAndMetric asserts the legacy path emits path=legacy
// on the span and the encrypt metric.
func TestEncrypt_LegacyMode_SpanAndMetric(t *testing.T) {
	t.Parallel()

	legacyKeys := newTestLegacyKeyMaterial(t)

	m, spy := newRecordingMetrics(t)
	registryRepo := &serviceTestRegistryRepo{records: map[string]*mmodel.OrganizationRegistryRecord{}}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))
	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, m)

	ctx, recorder := newRecordingTracerContext(t, context.Background())

	fieldCtx := FieldContext{
		TenantID:       "tenant-legacy",
		OrganizationID: "org-legacy",
		RecordID:       "record-2",
		FieldName:      "document",
	}

	_, err := svc.Encrypt(ctx, fieldCtx, "ABC123")
	require.NoError(t, err)

	span := spanByName(recorder, "service.protection.encrypt_field")
	require.NotNil(t, span)

	path, ok := spanStringAttr(span, "app.protection.path")
	require.True(t, ok)
	assert.Equal(t, "legacy", path)

	v, n := spy.counterValue(t, utils.CRMProtectionEncryptDecryptTotal.Name,
		map[string]string{"path": "legacy", "outcome": "success", "error_type": ""})
	assert.Equal(t, int64(1), v)
	assert.Equal(t, 1, n)
}

// TestEncrypt_InvalidFieldContext_UnknownPathFailure asserts the pre-branch
// validation failure records path=unknown, outcome=failure,
// error_type=field_context_invalid and a span error.
func TestEncrypt_InvalidFieldContext_UnknownPathFailure(t *testing.T) {
	t.Parallel()

	legacyKeys := newTestLegacyKeyMaterial(t)
	m, spy := newRecordingMetrics(t)
	registryRepo := &serviceTestRegistryRepo{records: map[string]*mmodel.OrganizationRegistryRecord{}}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))
	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, m)

	ctx, recorder := newRecordingTracerContext(t, context.Background())

	// Missing field name -> invalid field context.
	fieldCtx := FieldContext{
		TenantID:       "tenant-x",
		OrganizationID: "org-x",
		RecordID:       "record-x",
		FieldName:      "",
	}

	_, err := svc.Encrypt(ctx, fieldCtx, "plaintext")
	require.ErrorIs(t, err, ErrFieldContextInvalid)

	span := spanByName(recorder, "service.protection.encrypt_field")
	require.NotNil(t, span)
	assert.True(t, spanHasError(span))

	v, n := spy.counterValue(t, utils.CRMProtectionEncryptDecryptTotal.Name,
		map[string]string{"path": "unknown", "outcome": "failure", "error_type": "field_context_invalid"})
	assert.Equal(t, int64(1), v)
	assert.Equal(t, 1, n)
}

// --- Decrypt span + metric ------------------------------------------------

// TestDecrypt_EnvelopeMode_SpanAndMetric asserts decrypt_field span carries
// path=envelope and the success metric for a round-trip envelope decrypt.
func TestDecrypt_EnvelopeMode_SpanAndMetric(t *testing.T) {
	t.Parallel()

	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-dec",
		TenantID:             "tenant-dec",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestServiceWithMetrics(t, state, legacyKeys, crypto.EncryptionModeEnvelope)

	fieldCtx := FieldContext{
		TenantID:       "tenant-dec",
		OrganizationID: "org-dec",
		RecordID:       "record-3",
		FieldName:      "tax_id",
	}

	const plaintext = "987-65-4321"

	ciphertext, err := svc.svc.Encrypt(context.Background(), fieldCtx, plaintext)
	require.NoError(t, err)

	ctx, recorder := newRecordingTracerContext(t, context.Background())

	got, err := svc.svc.Decrypt(ctx, fieldCtx, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)

	span := spanByName(recorder, "service.protection.decrypt_field")
	require.NotNil(t, span, "decrypt_field span MUST be recorded")

	path, ok := spanStringAttr(span, "app.protection.path")
	require.True(t, ok)
	assert.Equal(t, "envelope", path)

	assert.False(t, spanHasError(span))

	for _, kv := range span.Attributes() {
		v := kv.Value.AsString()
		assert.NotContains(t, v, plaintext, "span value MUST NOT leak plaintext")
		assert.NotContains(t, v, ciphertext, "span value MUST NOT leak ciphertext")
	}

	v, n := svc.spy.counterValue(t, utils.CRMProtectionEncryptDecryptTotal.Name,
		map[string]string{"path": "envelope", "outcome": "success", "error_type": ""})
	// Encrypt + Decrypt both emit success for envelope path.
	assert.GreaterOrEqual(t, v, int64(1))
	assert.GreaterOrEqual(t, n, 1)
}

// TestDecrypt_EnvelopeDecryptFailed_Classifier asserts a corrupt envelope payload
// records error_type=envelope_decrypt_failed on the envelope path with a span error.
func TestDecrypt_EnvelopeDecryptFailed_Classifier(t *testing.T) {
	t.Parallel()

	state := ProtectionState{
		Mode:                 crypto.EncryptionModeEnvelope,
		CanReadLegacy:        false,
		CurrentKeysetVersion: 1,
		OrganizationID:       "org-fail",
		TenantID:             "tenant-fail",
	}

	legacyKeys := newTestLegacyKeyMaterial(t)
	svc, _ := createEncryptionTestServiceWithMetrics(t, state, legacyKeys, crypto.EncryptionModeEnvelope)

	fieldCtx := FieldContext{
		TenantID:       "tenant-fail",
		OrganizationID: "org-fail",
		RecordID:       "record-4",
		FieldName:      "tax_id",
	}

	ciphertext, err := svc.svc.Encrypt(context.Background(), fieldCtx, "secret")
	require.NoError(t, err)

	// Corrupt the marker payload so envelope decrypt fails (AEAD auth error).
	corrupt := ciphertext + "AAAA"

	ctx, recorder := newRecordingTracerContext(t, context.Background())

	_, err = svc.svc.Decrypt(ctx, fieldCtx, corrupt)
	require.ErrorIs(t, err, ErrEnvelopeDecryptFailed)

	span := spanByName(recorder, "service.protection.decrypt_field")
	require.NotNil(t, span)
	assert.True(t, spanHasError(span))

	v, n := svc.spy.counterValue(t, utils.CRMProtectionEncryptDecryptTotal.Name,
		map[string]string{"path": "envelope", "outcome": "failure", "error_type": "envelope_decrypt_failed"})
	assert.Equal(t, int64(1), v)
	assert.Equal(t, 1, n)
}

// TestDecrypt_LegacyReadNotAllowed_Classifier asserts a rejected legacy read
// records error_type=legacy_read_not_allowed on the legacy path with a span error.
func TestDecrypt_LegacyReadNotAllowed_Classifier(t *testing.T) {
	t.Parallel()

	legacyKeys := newTestLegacyKeyMaterial(t)

	cipherBytes, err := legacyKeys.aead.Encrypt([]byte("secret-value"), nil)
	require.NoError(t, err)
	legacyCiphertext := encodeBase64(cipherBytes)

	m, spy := newRecordingMetrics(t)
	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-noleg": {
				TenantID:       "tenant-noleg",
				OrganizationID: "org-noleg",
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: false,
				CurrentVersion: 1,
			},
		},
	}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))
	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, m)

	ctx, recorder := newRecordingTracerContext(t, context.Background())

	fieldCtx := FieldContext{
		TenantID:       "tenant-noleg",
		OrganizationID: "org-noleg",
		RecordID:       "record-5",
		FieldName:      "document",
	}

	_, err = svc.Decrypt(ctx, fieldCtx, legacyCiphertext)
	require.ErrorIs(t, err, ErrLegacyReadNotAllowed)

	span := spanByName(recorder, "service.protection.decrypt_field")
	require.NotNil(t, span)
	assert.True(t, spanHasError(span))

	v, n := spy.counterValue(t, utils.CRMProtectionEncryptDecryptTotal.Name,
		map[string]string{"path": "legacy", "outcome": "failure", "error_type": "legacy_read_not_allowed"})
	assert.Equal(t, int64(1), v)
	assert.Equal(t, 1, n)
}

// TestDecrypt_InvalidFieldContext_UnknownPathFailure asserts pre-branch validation
// failure on decrypt records path=unknown, error_type=field_context_invalid.
func TestDecrypt_InvalidFieldContext_UnknownPathFailure(t *testing.T) {
	t.Parallel()

	legacyKeys := newTestLegacyKeyMaterial(t)
	m, spy := newRecordingMetrics(t)
	registryRepo := &serviceTestRegistryRepo{records: map[string]*mmodel.OrganizationRegistryRecord{}}
	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))
	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, m)

	ctx, recorder := newRecordingTracerContext(t, context.Background())

	fieldCtx := FieldContext{
		TenantID:       "tenant-x",
		OrganizationID: "org-x",
		RecordID:       "record-x",
		FieldName:      "",
	}

	_, err := svc.Decrypt(ctx, fieldCtx, "anything")
	require.ErrorIs(t, err, ErrFieldContextInvalid)

	span := spanByName(recorder, "service.protection.decrypt_field")
	require.NotNil(t, span)
	assert.True(t, spanHasError(span))

	v, n := spy.counterValue(t, utils.CRMProtectionEncryptDecryptTotal.Name,
		map[string]string{"path": "unknown", "outcome": "failure", "error_type": "field_context_invalid"})
	assert.Equal(t, int64(1), v)
	assert.Equal(t, 1, n)
}

// --- Cache metric ----------------------------------------------------------

// TestGetPrimitives_CacheMissThenHit_RecordsExactlyOnce asserts the first fetch
// records exactly one miss and the second (cached) call records exactly one hit,
// with no double-counting on the double-check path.
func TestGetPrimitives_CacheMissThenHit_RecordsExactlyOnce(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-cache",
			KEKPath:           "org-org-cache",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}
	unwrapper := &fakeKeysetUnwrapper{aeadKeyset: aeadBytes, macKeyset: macBytes}

	m, spy := newRecordingMetrics(t)
	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), m)

	// First call: cache miss.
	_, _, _, _, err := manager.GetPrimitives(context.Background(), "org-cache")
	require.NoError(t, err)

	missV, missN := spy.counterValue(t, utils.CRMProtectionCacheTotal.Name,
		map[string]string{"operation": "get_primitives", "result": "miss"})
	assert.Equal(t, int64(1), missV, "exactly one miss on first fetch")
	assert.Equal(t, 1, missN)

	// Second call: cache hit (fast path).
	_, _, _, _, err = manager.GetPrimitives(context.Background(), "org-cache")
	require.NoError(t, err)

	hitV, hitN := spy.counterValue(t, utils.CRMProtectionCacheTotal.Name,
		map[string]string{"operation": "get_primitives", "result": "hit"})
	assert.Equal(t, int64(1), hitV, "exactly one hit on cached fetch")
	assert.Equal(t, 1, hitN, "hit MUST NOT be double-counted")
}
