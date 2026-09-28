// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestAccountClosingOutcome_MapsEverySentinelToItsReason is the golden table of the
// refusal vocabulary. Every sentinel a closing can answer with has exactly one
// reason, and the two unknown ends of the mapping — a business error outside the
// closing family and a raw driver failure — fall onto their bounded fallbacks
// instead of widening the label with whatever the error carried.
func TestAccountClosingOutcome_MapsEverySentinelToItsReason(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		outcome string
		reason  string
	}{
		{
			name:    "closed",
			err:     nil,
			outcome: accountClosingOutcomeClosed,
			reason:  "",
		},
		{
			name:    "0514 already closed",
			err:     midazpkg.ValidateBusinessError(constant.ErrAccountAlreadyClosed, constant.EntityAccount),
			outcome: accountClosingOutcomeRefused,
			reason:  accountClosingReasonAlreadyClosed,
		},
		{
			name:    "0515 closing in progress",
			err:     midazpkg.ValidateBusinessError(constant.ErrAccountClosingInProgress, constant.EntityAccount),
			outcome: accountClosingOutcomeRefused,
			reason:  accountClosingReasonClosingInProgress,
		},
		{
			name:    "0526 another administrative operation in progress",
			err:     midazpkg.ValidateBusinessError(constant.ErrAccountAdministrativeOperationInProgress, constant.EntityAccount),
			outcome: accountClosingOutcomeRefused,
			reason:  accountClosingReasonOperationInProgress,
		},
		{
			name:    "0516 balance not zero",
			err:     midazpkg.ValidateBusinessError(constant.ErrAccountBalanceNotZero, constant.EntityAccount),
			outcome: accountClosingOutcomeRefused,
			reason:  accountClosingReasonBalanceNotZero,
		},
		{
			name:    "0517 pending transactions",
			err:     midazpkg.ValidateBusinessError(constant.ErrAccountHasPendingTransactions, constant.EntityAccount),
			outcome: accountClosingOutcomeRefused,
			reason:  accountClosingReasonPendingTransactions,
		},
		{
			name:    "0518 persistence pending",
			err:     midazpkg.ValidateBusinessError(constant.ErrAccountClosingPersistencePending, constant.EntityAccount),
			outcome: accountClosingOutcomeRefused,
			reason:  accountClosingReasonPersistencePending,
		},
		{
			name:    "0519 account closed",
			err:     midazpkg.ValidateBusinessError(constant.ErrAccountClosed, constant.EntityAccount),
			outcome: accountClosingOutcomeRefused,
			reason:  accountClosingReasonAccountClosed,
		},
		{
			name:    "0520 protection indeterminate",
			err:     midazpkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount),
			outcome: accountClosingOutcomeIndeterminate,
			reason:  accountClosingReasonProtectionIndeterminate,
		},
		{
			name:    "0074 external account",
			err:     midazpkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, constant.EntityAccount),
			outcome: accountClosingOutcomeRefused,
			reason:  accountClosingReasonExternalAccount,
		},
		{
			name:    "0052 account not found",
			err:     midazpkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount),
			outcome: accountClosingOutcomeRefused,
			reason:  accountClosingReasonAccountNotFound,
		},
		{
			name:    "business error outside the closing family",
			err:     midazpkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAccount),
			outcome: accountClosingOutcomeRefused,
			reason:  accountClosingReasonBusinessOther,
		},
		{
			name:    "raw driver failure",
			err:     errors.New("connection reset by peer"),
			outcome: accountClosingOutcomeTechnical,
			reason:  accountClosingReasonTechnical,
		},
		{
			name:    "wrapped raw driver failure",
			err:     errors.Join(errors.New("dial"), context.DeadlineExceeded),
			outcome: accountClosingOutcomeTechnical,
			reason:  accountClosingReasonTechnical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, reason := accountClosingOutcome(tt.err)

			assert.Equal(t, tt.outcome, outcome)
			assert.Equal(t, tt.reason, reason)
		})
	}
}

// TestAccountClosingReason_StaysWithinItsVocabulary pins the label set itself: a
// reason that is not one of these values would be a new series, which is exactly
// what a finite vocabulary exists to prevent.
func TestAccountClosingReason_StaysWithinItsVocabulary(t *testing.T) {
	vocabulary := map[string]struct{}{
		accountClosingReasonAlreadyClosed:           {},
		accountClosingReasonClosingInProgress:       {},
		accountClosingReasonOperationInProgress:     {},
		accountClosingReasonBalanceNotZero:          {},
		accountClosingReasonPendingTransactions:     {},
		accountClosingReasonPersistencePending:      {},
		accountClosingReasonAccountClosed:           {},
		accountClosingReasonProtectionIndeterminate: {},
		accountClosingReasonExternalAccount:         {},
		accountClosingReasonAccountNotFound:         {},
		accountClosingReasonBusinessOther:           {},
		accountClosingReasonTechnical:               {},
	}

	require.Len(t, vocabulary, 12)

	for _, sentinel := range []error{
		constant.ErrAccountAlreadyClosed,
		constant.ErrAccountClosingInProgress,
		constant.ErrAccountAdministrativeOperationInProgress,
		constant.ErrAccountBalanceNotZero,
		constant.ErrAccountHasPendingTransactions,
		constant.ErrAccountClosingPersistencePending,
		constant.ErrAccountClosed,
		constant.ErrAccountClosingProtectionIndeterminate,
		constant.ErrForbiddenExternalAccountManipulation,
		constant.ErrAccountIDNotFound,
		constant.ErrEntityNotFound,
		constant.ErrBalanceUpdateFailed,
	} {
		reason := accountClosingReason(midazpkg.ValidateBusinessError(sentinel, constant.EntityAccount))

		assert.Contains(t, vocabulary, reason, sentinel.Error())
	}
}

// TestAccountClosingTelemetry_IsNilFactorySafe covers the deployment with metrics
// disabled and every unit test: emission is skipped, nothing panics and no caller
// learns the difference.
func TestAccountClosingTelemetry_IsNilFactorySafe(t *testing.T) {
	ctx := context.Background()

	assert.NotPanics(t, func() {
		recordAccountClosingOutcome(ctx, nil, nil, nil)
		recordAccountClosingOutcome(ctx, nil, nil, errors.New("boom"))
		recordAccountClosingReconciliation(ctx, nil, nil, AccountClosingReconciliationStats{}, time.Second, true, closeInstant)
	})
}

// accountClosingSeries collects the data points of one metric, keyed by the single
// label the series carries. A key of "" is the unlabelled series.
func accountClosingSeries(t *testing.T, reader *sdkmetric.ManualReader, name, label string) map[string]int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	values := make(map[string]int64)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, dp := range data.DataPoints {
					assertBoundedAccountClosingLabels(t, dp.Attributes.ToSlice())

					values[accountClosingLabelValue(dp.Attributes.ToSlice(), label)] = dp.Value
				}
			case metricdata.Gauge[int64]:
				for _, dp := range data.DataPoints {
					assertBoundedAccountClosingLabels(t, dp.Attributes.ToSlice())

					values[accountClosingLabelValue(dp.Attributes.ToSlice(), label)] = dp.Value
				}
			default:
				t.Fatalf("unexpected data type %T for %s", m.Data, name)
			}
		}
	}

	return values
}

func accountClosingLabelValue(attrs []attribute.KeyValue, label string) string {
	for _, kv := range attrs {
		if string(kv.Key) == label {
			return kv.Value.AsString()
		}
	}

	return ""
}

// assertBoundedAccountClosingLabels pins the D6 prohibition on the wire itself: the
// only labels a closing series may carry name a bounded vocabulary, never an
// account, an organization, a ledger or a tenant.
func assertBoundedAccountClosingLabels(t *testing.T, attrs []attribute.KeyValue) {
	t.Helper()

	for _, kv := range attrs {
		assert.Contains(t, []string{"outcome", "reason", "stage", "kind"}, string(kv.Key))
	}
}

// TestRecordAccountClosingOutcome_EmitsBoundedSeries walks the emission with a real
// meter: a refusal raises its outcome and its reason, a closing raises only the
// outcome, and no series carries anything but the bounded label.
func TestRecordAccountClosingOutcome_EmitsBoundedSeries(t *testing.T) {
	reader, factory := newReaderFactory(t)
	ctx := context.Background()

	recordAccountClosingOutcome(ctx, factory, nil, nil)
	recordAccountClosingOutcome(ctx, factory, nil,
		midazpkg.ValidateBusinessError(constant.ErrAccountBalanceNotZero, constant.EntityAccount))
	recordAccountClosingOutcome(ctx, factory, nil,
		midazpkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount))

	outcomes := accountClosingSeries(t, reader, accountClosingRequests.Name, "outcome")
	assert.Equal(t, int64(1), outcomes[accountClosingOutcomeClosed])
	assert.Equal(t, int64(1), outcomes[accountClosingOutcomeRefused])
	assert.Equal(t, int64(1), outcomes[accountClosingOutcomeIndeterminate])

	reasons := accountClosingSeries(t, reader, accountClosingRefusals.Name, "reason")
	assert.Equal(t, int64(1), reasons[accountClosingReasonBalanceNotZero])
	assert.Equal(t, int64(1), reasons[accountClosingReasonProtectionIndeterminate])
	assert.NotContains(t, reasons, "", "a closing must not raise a refusal series")
}

// TestRecordAccountClosingReconciliation_EmitsBacklogAndAge pins the D5 measures of
// one pass: what it resolved, what failed, the backlog it left and the instant an
// age alert subtracts from now. An incomplete pass must NOT advance that instant.
func TestRecordAccountClosingReconciliation_EmitsBacklogAndAge(t *testing.T) {
	reader, factory := newReaderFactory(t)
	ctx := context.Background()

	stats := AccountClosingReconciliationStats{Scanned: 4, Completed: 1, Released: 1, Retained: 2, Unreadable: 1, Ownerships: 3}
	stats.fail(accountClosingStageEvictBalance)

	recordAccountClosingReconciliation(ctx, factory, nil, stats, 250*time.Millisecond, true, closeInstant)

	markers := accountClosingSeries(t, reader, accountClosingReconcileMarkers.Name, "outcome")
	assert.Equal(t, int64(4), markers[accountClosingReconcileScanned])
	assert.Equal(t, int64(1), markers[accountClosingReconcileCompleted])
	assert.Equal(t, int64(2), markers[accountClosingReconcileRetained])

	failures := accountClosingSeries(t, reader, accountClosingReconcileFailures.Name, "stage")
	assert.Equal(t, int64(1), failures[accountClosingStageEvictBalance])
	assert.NotContains(t, failures, accountClosingStageScanMarkers)

	backlog := accountClosingSeries(t, reader, accountClosingReconcileBacklog.Name, "kind")
	assert.Equal(t, int64(3), backlog[accountClosingBacklogOwnership])
	assert.Equal(t, int64(2), backlog[accountClosingBacklogRetained])

	age := accountClosingSeries(t, reader, accountClosingReconcileLastSuccess.Name, "")
	assert.Equal(t, closeInstant.Unix(), age[""])
}

// TestRecordAccountClosingReconciliation_WithholdsAgeFromAnIncompletePass is the
// other side: a pass that did not walk the namespace reports its failures and its
// backlog, but leaves the last-success instant where it was, so the age keeps
// growing instead of reporting coverage that never happened.
func TestRecordAccountClosingReconciliation_WithholdsAgeFromAnIncompletePass(t *testing.T) {
	reader, factory := newReaderFactory(t)
	ctx := context.Background()

	stats := AccountClosingReconciliationStats{Ownerships: 1}
	stats.fail(accountClosingStageScanMarkers)

	recordAccountClosingReconciliation(ctx, factory, nil, stats, time.Second, false, closeInstant)

	failures := accountClosingSeries(t, reader, accountClosingReconcileFailures.Name, "stage")
	assert.Equal(t, int64(1), failures[accountClosingStageScanMarkers])

	age := accountClosingSeries(t, reader, accountClosingReconcileLastSuccess.Name, "")
	assert.Empty(t, age, "an incomplete pass must not advance the last-success instant")
}

// TestAccountClosingReconciliationStats_RecordsFailuresByStage pins the failure
// bookkeeping: the counters are keyed by the bounded stage vocabulary and the zero
// value of the struct stays the empty pass, so a reconciliation that fails nothing
// allocates nothing.
func TestAccountClosingReconciliationStats_RecordsFailuresByStage(t *testing.T) {
	var stats AccountClosingReconciliationStats

	require.Nil(t, stats.failures)
	require.Equal(t, AccountClosingReconciliationStats{}, stats)

	stats.fail(accountClosingStageEvictBalance)
	stats.fail(accountClosingStageEvictBalance)
	stats.fail(accountClosingStageReleaseOwnership)

	assert.Equal(t, 2, stats.failures[accountClosingStageEvictBalance])
	assert.Equal(t, 1, stats.failures[accountClosingStageReleaseOwnership])

	for stage := range stats.failures {
		assert.Contains(t, accountClosingFailureStages, stage)
	}
}

// TestReconcileAccountClosings_RecordsTheStageThatFailed walks one pass whose two
// scans both fail, and pins that each is attributed to its own stage. Those are the
// stages that decide whether the pass may advance its last-success instant: a pass
// that could not walk the namespace has not reconciled it, however few markers it
// happened to see.
func TestReconcileAccountClosings_RecordsTheStageThatFailed(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.redis.EXPECT().ScanAccountClosingMarkers(gomock.Any(), uint64(0), gomock.Any()).
		Return(txRedis.AccountProtectionScanPage{}, errors.New("cache unavailable"))
	m.redis.EXPECT().ScanAccountAdminOwnerships(gomock.Any(), uint64(0), gomock.Any()).
		Return(txRedis.AccountProtectionScanPage{}, errors.New("cache unavailable"))

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.failures[accountClosingStageScanMarkers])
	assert.Equal(t, 1, stats.failures[accountClosingStageScanOwnerships])
	assert.Zero(t, stats.Scanned)
}

// TestReconcileAccountClosings_AttributesAFailedEvictionToItsStage pins the same
// attribution one level down: the eviction of a confirmed closing is a step of the
// pass, and its failure is counted under that step rather than under the scan.
func TestReconcileAccountClosings_AttributesAFailedEvictionToItsStage(t *testing.T) {
	m := newCloseAccountMocks(t)

	recorded := closeInstant

	m.expectMarkerDiscovered()
	m.expectAttemptRead(true)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: &recorded}, nil)
	m.balance.EXPECT().ListByAccountID(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(nil, errors.New("cache unavailable"))

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.failures[accountClosingStageListBalances])
	assert.Equal(t, 1, stats.Retained)
	assert.Zero(t, stats.failures[accountClosingStageScanMarkers])
}

// TestAccountClosingFailureStages_AreUnique guards the emission list itself: a
// duplicated stage would emit the same series twice in one pass, doubling a failure
// rate that an operator reads as a worsening outage.
func TestAccountClosingFailureStages_AreUnique(t *testing.T) {
	seen := make(map[string]struct{}, len(accountClosingFailureStages))

	for _, stage := range accountClosingFailureStages {
		_, duplicate := seen[stage]
		assert.False(t, duplicate, stage)

		seen[stage] = struct{}{}
	}

	assert.Len(t, accountClosingFailureStages, 10)
}
