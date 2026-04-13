// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestLogger creates a real logger for tests (no-op by using high log level filtering)
func newTestLogger() libLog.Logger {
	return libLog.NewNop()
}

// --- Tests for NewBalanceSyncWorker ---

func TestNewBalanceSyncWorker(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	useCase := &command.UseCase{}

	worker := NewBalanceSyncWorker(logger, useCase, BalanceSyncConfig{})

	require.NotNil(t, worker)
	assert.Equal(t, 1*time.Second, worker.idleWait) // 2 * FlushTimeoutMs(500) = 1000ms, clamped to 1s
	assert.Same(t, useCase, worker.useCase)
}

// --- Tests for extractIDsFromMember ---

func TestExtractIDsFromMember(t *testing.T) {
	t.Parallel()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	tests := []struct {
		name           string
		member         string
		wantOrgID      uuid.UUID
		wantLedgerID   uuid.UUID
		wantErr        bool
		errMsgContains string
	}{
		{
			name:         "valid key with standard format",
			member:       "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":@account#key",
			wantOrgID:    orgID,
			wantLedgerID: ledgerID,
			wantErr:      false,
		},
		{
			name:         "valid key with default balance key",
			member:       "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":default",
			wantOrgID:    orgID,
			wantLedgerID: ledgerID,
			wantErr:      false,
		},
		{
			name:           "empty string",
			member:         "",
			wantErr:        true,
			errMsgContains: "missing two UUIDs",
		},
		{
			name:           "only one UUID",
			member:         "balance:{transactions}:" + orgID.String() + ":notauuid:@account",
			wantErr:        true,
			errMsgContains: "missing two UUIDs",
		},
		{
			name:           "no UUIDs at all",
			member:         "balance:{transactions}:invalid:also-invalid:@account",
			wantErr:        true,
			errMsgContains: "missing two UUIDs",
		},
		{
			name:           "malformed UUID format",
			member:         "balance:{transactions}:not-a-valid-uuid-format:also-not-valid:@account",
			wantErr:        true,
			errMsgContains: "missing two UUIDs",
		},
		{
			name:         "UUIDs at different positions",
			member:       "prefix:" + orgID.String() + ":middle:" + ledgerID.String() + ":suffix",
			wantOrgID:    orgID,
			wantLedgerID: ledgerID,
			wantErr:      false,
		},
		{
			name:         "UUIDs with no prefix",
			member:       orgID.String() + ":" + ledgerID.String(),
			wantOrgID:    orgID,
			wantLedgerID: ledgerID,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			worker := &BalanceSyncWorker{}

			gotOrgID, gotLedgerID, err := worker.extractIDsFromMember(tt.member)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsgContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOrgID, gotOrgID)
			assert.Equal(t, tt.wantLedgerID, gotLedgerID)
		})
	}
}

// --- Tests for waitOrDone ---

func TestWaitOrDone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		duration       time.Duration
		cancelBefore   bool
		expectedResult bool
	}{
		{
			name:           "zero duration returns immediately",
			duration:       0,
			cancelBefore:   false,
			expectedResult: false,
		},
		{
			name:           "negative duration returns immediately",
			duration:       -1 * time.Second,
			cancelBefore:   false,
			expectedResult: false,
		},
		{
			name:           "cancelled context returns true",
			duration:       1 * time.Hour, // Long duration to ensure context cancellation wins
			cancelBefore:   true,
			expectedResult: true,
		},
		{
			name:           "short wait completes normally",
			duration:       1 * time.Millisecond,
			cancelBefore:   false,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelBefore {
				cancel()
			}

			result := waitOrDone(ctx, tt.duration, newTestLogger())

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// --- Property-based test for extractIDsFromMember ---

func TestProperty_ExtractIDsFromMember_ValidKeys(t *testing.T) {
	t.Parallel()

	// Property: For any valid org/ledger UUID pair in a properly formatted key,
	// extractIDsFromMember should return those exact UUIDs
	testCases := []struct {
		prefix string
		suffix string
	}{
		{"balance:{transactions}:", ":default"},
		{"balance:{transactions}:", ":@account#key"},
		{"prefix:", ":suffix"},
		{"", ""},
		{"a:b:c:", ":d:e:f"},
	}

	for i := 0; i < 10; i++ {
		orgID := uuid.Must(libCommons.GenerateUUIDv7())
		ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%d_%s%s", i, tc.prefix, tc.suffix), func(t *testing.T) {
				t.Parallel()

				member := tc.prefix + orgID.String() + ":" + ledgerID.String() + tc.suffix
				worker := &BalanceSyncWorker{}

				gotOrg, gotLedger, err := worker.extractIDsFromMember(member)

				require.NoError(t, err)
				assert.Equal(t, orgID, gotOrg)
				assert.Equal(t, ledgerID, gotLedger)
			})
		}
	}
}
