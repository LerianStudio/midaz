// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolve_ModeAndStatusMetrics asserts that Resolve emits the mode-resolution
// and status counters for every resolved branch with the documented label values,
// while keeping the existing return values and branch behavior identical.
func TestResolve_ModeAndStatusMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		nilRepo        bool
		record         *mmodel.OrganizationRegistryRecord
		repoErr        error
		wantMode       crypto.EncryptionMode
		wantModeLabel  string
		wantStatus     string
		wantErr        bool
		wantErrUnknown bool
	}{
		{
			name:          "nil registry repo -> legacy/none",
			nilRepo:       true,
			wantMode:      crypto.EncryptionModeLegacy,
			wantModeLabel: "legacy",
			wantStatus:    "none",
		},
		{
			name:          "registry not found -> legacy/none",
			record:        nil,
			repoErr:       constant.ErrRegistryNotFound,
			wantMode:      crypto.EncryptionModeLegacy,
			wantModeLabel: "legacy",
			wantStatus:    "none",
		},
		{
			name: "active -> envelope/active",
			record: &mmodel.OrganizationRegistryRecord{
				TenantID:       "tenant-1",
				OrganizationID: "org-active",
				Status:         mmodel.RegistryStatusActive,
				CurrentVersion: 3,
				LegacyReadable: true,
			},
			wantMode:      crypto.EncryptionModeEnvelope,
			wantModeLabel: "envelope",
			wantStatus:    "active",
		},
		{
			name: "unknown status -> error unchanged + status unknown",
			record: &mmodel.OrganizationRegistryRecord{
				OrganizationID: "org-unknown",
				Status:         "weird_status",
			},
			wantErr:        true,
			wantErrUnknown: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, spy := newRecordingMetrics(t)

			var repo *fakeRegistryRepoForProtection
			if !tt.nilRepo {
				repo = &fakeRegistryRepoForProtection{record: tt.record, err: tt.repoErr}
			}

			var resolver *ProtectionStateResolver
			if tt.nilRepo {
				resolver = NewProtectionStateResolver(nil, m)
			} else {
				resolver = NewProtectionStateResolver(repo, m)
			}

			state, err := resolver.Resolve(context.Background(), "org-active")

			if tt.wantErr {
				require.Error(t, err)

				if tt.wantErrUnknown {
					// Original error returned unchanged.
					assert.Contains(t, err.Error(), "unknown registry status")
				}

				// Unknown-status branch records status=unknown and no mode resolution.
				v, n := spy.counterValue(t, utils.CRMProtectionStatusTotal.Name, map[string]string{"status": "unknown"})
				assert.Equal(t, int64(1), v, "status=unknown counter")
				assert.Equal(t, 1, n)
				spy.assertNoCounter(t, utils.CRMProtectionModeResolutionTotal.Name)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantMode, state.Mode, "resolved mode unchanged")

			modeV, modeN := spy.counterValue(t, utils.CRMProtectionModeResolutionTotal.Name, map[string]string{"mode": tt.wantModeLabel})
			assert.Equal(t, int64(1), modeV, "mode_resolution_total{mode=%s}", tt.wantModeLabel)
			assert.Equal(t, 1, modeN)

			statusV, statusN := spy.counterValue(t, utils.CRMProtectionStatusTotal.Name, map[string]string{"status": tt.wantStatus})
			assert.Equal(t, int64(1), statusV, "status_total{status=%s}", tt.wantStatus)
			assert.Equal(t, 1, statusN)
		})
	}
}

// TestResolve_EmitsResolveModeSpan asserts the resolve_mode span is opened with
// the request org id input attribute and the resolved_mode output attribute.
func TestResolve_EmitsResolveModeSpan(t *testing.T) {
	t.Parallel()

	ctx, recorder := newRecordingTracerContext(t, context.Background())

	repo := &fakeRegistryRepoForProtection{
		record: &mmodel.OrganizationRegistryRecord{
			OrganizationID: "org-span",
			Status:         mmodel.RegistryStatusActive,
			CurrentVersion: 1,
		},
	}

	resolver := NewProtectionStateResolver(repo, NewProtectionMetrics(nil))

	_, err := resolver.Resolve(ctx, "org-span")
	require.NoError(t, err)

	span := spanByName(recorder, "service.protection.resolve_mode")
	require.NotNil(t, span, "resolve_mode span MUST be recorded")

	org, ok := spanStringAttr(span, "app.request.organization_id")
	require.True(t, ok, "span MUST carry app.request.organization_id")
	assert.Equal(t, "org-span", org)

	mode, ok := spanStringAttr(span, "app.protection.resolved_mode")
	require.True(t, ok, "span MUST carry app.protection.resolved_mode")
	assert.Equal(t, "envelope", mode)
}

// TestResolve_UnknownStatus_SpanError asserts the unknown-status branch records a
// span error and returns the original error unchanged.
func TestResolve_UnknownStatus_SpanError(t *testing.T) {
	t.Parallel()

	ctx, recorder := newRecordingTracerContext(t, context.Background())

	repo := &fakeRegistryRepoForProtection{
		record: &mmodel.OrganizationRegistryRecord{
			OrganizationID: "org-unknown",
			Status:         "weird_status",
		},
	}

	resolver := NewProtectionStateResolver(repo, NewProtectionMetrics(nil))

	_, err := resolver.Resolve(ctx, "org-unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown registry status")

	span := spanByName(recorder, "service.protection.resolve_mode")
	require.NotNil(t, span)
	assert.True(t, spanHasError(span), "unknown-status branch MUST record span error")
}

// TestResolve_RepositoryError_Propagated asserts an unexpected repo error is
// returned unchanged and does NOT emit a status counter (not a resolved outcome).
func TestResolve_RepositoryError_Propagated(t *testing.T) {
	t.Parallel()

	m, spy := newRecordingMetrics(t)

	repoErr := errors.New("database connection failed")
	repo := &fakeRegistryRepoForProtection{err: repoErr}

	resolver := NewProtectionStateResolver(repo, m)

	_, err := resolver.Resolve(context.Background(), "org-err")
	require.ErrorIs(t, err, repoErr)

	spy.assertNoCounter(t, utils.CRMProtectionStatusTotal.Name)
	spy.assertNoCounter(t, utils.CRMProtectionModeResolutionTotal.Name)
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

// TestDecryptLegacy_AllowedRead_RecordsLegacyReadMetric asserts the legacy-read
// counter is recorded exactly once on an allowed legacy decrypt, labeled with the
// organization's resolved mode string.
func TestDecryptLegacy_AllowedRead_RecordsLegacyReadMetric(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	legacyKeys := newTestLegacyKeyMaterial(t)

	cipherBytes, err := legacyKeys.aead.Encrypt([]byte("secret-value"), nil)
	require.NoError(t, err)
	legacyCiphertext := base64.StdEncoding.EncodeToString(cipherBytes)

	m, spy := newRecordingMetrics(t)

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: true,
				CurrentVersion: 1,
			},
		},
	}

	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))
	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, m)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "document",
	}

	decrypted, err := svc.Decrypt(ctx, fieldCtx, legacyCiphertext)
	require.NoError(t, err)
	assert.Equal(t, "secret-value", decrypted)

	// Active registry -> envelope mode, so organization_status label = "envelope".
	v, n := spy.counterValue(t, utils.CRMProtectionLegacyReadTotal.Name, map[string]string{"organization_status": "envelope"})
	assert.Equal(t, int64(1), v, "legacy_read_total{organization_status=envelope}")
	assert.Equal(t, 1, n, "legacy read recorded exactly once")
}

// TestDecryptLegacy_Rejected_DoesNotRecordLegacyReadMetric asserts the legacy-read
// counter is NOT recorded when the read is rejected with ErrLegacyReadNotAllowed.
func TestDecryptLegacy_Rejected_DoesNotRecordLegacyReadMetric(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	legacyKeys := newTestLegacyKeyMaterial(t)

	cipherBytes, err := legacyKeys.aead.Encrypt([]byte("secret-value"), nil)
	require.NoError(t, err)
	legacyCiphertext := base64.StdEncoding.EncodeToString(cipherBytes)

	m, spy := newRecordingMetrics(t)

	registryRepo := &serviceTestRegistryRepo{
		records: map[string]*mmodel.OrganizationRegistryRecord{
			"org-123": {
				TenantID:       "tenant-abc",
				OrganizationID: "org-123",
				Status:         mmodel.RegistryStatusActive,
				LegacyReadable: false,
				CurrentVersion: 1,
			},
		},
	}

	stateResolver := NewProtectionStateResolver(registryRepo, NewProtectionMetrics(nil))
	svc := NewEncryptionService(stateResolver, nil, nil, legacyKeys, m)

	fieldCtx := FieldContext{
		TenantID:       "tenant-abc",
		OrganizationID: "org-123",
		RecordID:       "record-456",
		FieldName:      "document",
	}

	_, err = svc.Decrypt(ctx, fieldCtx, legacyCiphertext)
	require.ErrorIs(t, err, ErrLegacyReadNotAllowed)

	assert.Equal(t, 0, spy.counterCount(t, utils.CRMProtectionLegacyReadTotal.Name), "legacy read MUST NOT be recorded on rejection")
}
