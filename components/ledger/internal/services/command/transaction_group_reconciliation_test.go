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
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

var reconcileNow = time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)

type transactionGroupReconcileReader struct {
	TransactionReader
	members map[uuid.UUID][]*transaction.Transaction
	err     error
}

func (reader *transactionGroupReconcileReader) FindTransactionsByGroupID(_ context.Context, groupID uuid.UUID) ([]*transaction.Transaction, error) {
	if reader.err != nil {
		return nil, reader.err
	}

	return reader.members[groupID], nil
}

func reconcileTestGroup(t *testing.T, id uuid.UUID, createdAt time.Time) *transactiongroup.TransactionGroup {
	t.Helper()

	parts, err := decomposeCrossLedgerTransaction(
		crossLedgerTestTransaction("100",
			[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "100", true)},
			[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "100", false)}),
		crossLedgerTransactionScopes{
			from: []atomicTransactionBatchLedgerRef{{organizationID: groupEventOrganization, ledgerID: groupEventLedgerA}},
			to:   []atomicTransactionBatchLedgerRef{{organizationID: groupEventOrganization, ledgerID: groupEventLedgerB}},
		},
	)
	require.NoError(t, err)

	intent, err := buildCrossLedgerGroupIntent("BRL", parts)
	require.NoError(t, err)

	raw, err := encodeCrossLedgerGroupIntent(intent)
	require.NoError(t, err)

	return &transactiongroup.TransactionGroup{
		ID: id, OrganizationID: groupEventOrganization, LedgerID: groupEventLedgerA,
		Status: constant.PENDING, AssetCode: "BRL", Intent: raw,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

// reconcileOrigin and reconcileDestination build members as the group read
// returns them: source and destination are not persisted columns, so a row read
// back from the repository carries neither, and only the intent knows its role.
func reconcileOrigin(groupID uuid.UUID, status string, updatedAt time.Time) *transaction.Transaction {
	member := crossLedgerGroupEventMember(groupEventLedgerA, groupID, status, nil, nil)
	member.CreatedAt, member.UpdatedAt = updatedAt, updatedAt

	return member
}

func reconcileDestination(groupID uuid.UUID, status string, updatedAt time.Time) *transaction.Transaction {
	member := crossLedgerGroupEventMember(groupEventLedgerB, groupID, status, nil, nil)
	member.CreatedAt, member.UpdatedAt = updatedAt, updatedAt

	return member
}

func newReconcileUseCase(
	t *testing.T,
	repo *transactiongroup.MockRepository,
	reader *transactionGroupReconcileReader,
) (*UseCase, *pkgStreaming.MockEmitter, *sdkmetric.ManualReader) {
	t.Helper()

	metricsReader, factory := newReaderFactory(t)
	emitter := pkgStreaming.NewMockEmitter()

	return &UseCase{
		TransactionGroupRepo: repo,
		TransactionReader:    reader,
		Streaming:            emitter,
		MetricsFactory:       factory,
		Clock:                func() time.Time { return reconcileNow },
	}, emitter, metricsReader
}

func collectReconcileResults(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))

	result := make(map[string]int64)

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "cross_ledger_group_reconcile_total" {
				continue
			}

			sum, ok := metric.Data.(metricdata.Sum[int64])
			require.True(t, ok)

			for _, point := range sum.DataPoints {
				require.Equal(t, 1, point.Attributes.Len(), "the reconcile counter carries only the result label")
				value, _ := point.Attributes.Value("result")
				result[value.AsString()] += point.Value
			}
		}
	}

	return result
}

func expectReconcilePage(repo *transactiongroup.MockRepository, afterID uuid.UUID, groups ...*transactiongroup.TransactionGroup) *gomock.Call {
	return repo.EXPECT().ListByStatusOlderThan(
		gomock.Any(), constant.PENDING, reconcileNow.Add(-defaultTransactionGroupReconcileMinAge), afterID, transactionGroupReconcilePageSize,
	).Return(groups, nil)
}

func TestReconcileTransactionGroups_DeletesOnlyOldOrphans(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	oldOrphan := reconcileTestGroup(t, uuid.MustParse("0199a800-0000-7000-8000-000000000001"), reconcileNow.Add(-25*time.Hour))
	youngOrphan := reconcileTestGroup(t, uuid.MustParse("0199a800-0000-7000-8000-000000000002"), reconcileNow.Add(-time.Hour))
	uc, emitter, metricsReader := newReconcileUseCase(t, repo, &transactionGroupReconcileReader{})

	expectReconcilePage(repo, uuid.Nil, oldOrphan, youngOrphan)
	repo.EXPECT().Delete(gomock.Any(), oldOrphan.ID).Return(nil)

	stats := uc.ReconcileTransactionGroups(context.Background())

	assert.Equal(t, TransactionGroupReconciliationStats{Scanned: 2, Deleted: 1, Skipped: 1}, stats)
	assert.Equal(t, map[string]int64{"deleted": 1, "skipped": 1}, collectReconcileResults(t, metricsReader))
	requireNoGroupEvent(t, emitter)
}

func TestReconcileTransactionGroups_AlignsSettledGroupsFromTheirMembers(t *testing.T) {
	committedID := uuid.MustParse("0199a800-0000-7000-8000-000000000011")
	canceledID := uuid.MustParse("0199a800-0000-7000-8000-000000000012")
	settledAt := reconcileNow.Add(-time.Hour)

	for _, test := range []struct {
		name    string
		groupID uuid.UUID
		members func(uuid.UUID) []*transaction.Transaction
		status  string
		key     string
	}{
		{
			name:    "every part approved",
			groupID: committedID,
			members: func(id uuid.UUID) []*transaction.Transaction {
				return []*transaction.Transaction{
					reconcileOrigin(id, constant.APPROVED, settledAt),
					reconcileDestination(id, constant.APPROVED, settledAt),
				}
			},
			status: constant.APPROVED,
			key:    events.TransactionGroupCommittedDefinition.Key(),
		},
		{
			name:    "every origin canceled",
			groupID: canceledID,
			members: func(id uuid.UUID) []*transaction.Transaction {
				return []*transaction.Transaction{reconcileOrigin(id, constant.CANCELED, settledAt)}
			},
			status: constant.CANCELED,
			key:    events.TransactionGroupCanceledDefinition.Key(),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := transactiongroup.NewMockRepository(ctrl)
			group := reconcileTestGroup(t, test.groupID, reconcileNow.Add(-2*time.Hour))
			members := test.members(test.groupID)
			uc, emitter, metricsReader := newReconcileUseCase(t, repo, &transactionGroupReconcileReader{
				members: map[uuid.UUID][]*transaction.Transaction{test.groupID: members},
			})

			expectReconcilePage(repo, uuid.Nil, group)
			repo.EXPECT().UpdateStatus(gomock.Any(), test.groupID, constant.PENDING, test.status).Return(true, nil)

			stats := uc.ReconcileTransactionGroups(context.Background())

			assert.Equal(t, TransactionGroupReconciliationStats{Scanned: 1, Repaired: 1}, stats)
			assert.Equal(t, map[string]int64{"repaired": 1}, collectReconcileResults(t, metricsReader))

			payload := requireOneGroupEvent(t, emitter, test.key)
			assert.Equal(t, test.groupID.String(), payload.GroupID)
			assert.Equal(t, test.status, payload.Status)
			require.Len(t, payload.Parts, len(members))
			assert.Equal(t, members[0].ID, payload.Parts[0].TransactionID)
			assert.Equal(t, events.TransactionGroupRoleOrigin, payload.Parts[0].Role, "roles come from the intent, not from legs the row does not carry")

			if len(payload.Parts) == 2 {
				assert.Equal(t, events.TransactionGroupRoleDestination, payload.Parts[1].Role)
			}
		})
	}
}

func TestReconcileTransactionGroups_NeverWritesAnInconsistentOrLiveGroup(t *testing.T) {
	settledAt := reconcileNow.Add(-time.Hour)
	recent := reconcileNow.Add(-time.Minute)
	mixedID := uuid.MustParse("0199a800-0000-7000-8000-000000000021")
	heldID := uuid.MustParse("0199a800-0000-7000-8000-000000000022")
	inFlightID := uuid.MustParse("0199a800-0000-7000-8000-000000000023")
	missingDestinationID := uuid.MustParse("0199a800-0000-7000-8000-000000000024")

	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	uc, emitter, metricsReader := newReconcileUseCase(t, repo, &transactionGroupReconcileReader{
		members: map[uuid.UUID][]*transaction.Transaction{
			mixedID: {
				reconcileOrigin(mixedID, constant.CANCELED, settledAt),
				reconcileDestination(mixedID, constant.APPROVED, settledAt),
			},
			heldID: {reconcileOrigin(heldID, constant.PENDING, settledAt)},
			inFlightID: {
				reconcileOrigin(inFlightID, constant.APPROVED, recent),
			},
			missingDestinationID: {reconcileOrigin(missingDestinationID, constant.APPROVED, settledAt)},
		},
	})

	expectReconcilePage(
		repo, uuid.Nil,
		reconcileTestGroup(t, mixedID, reconcileNow.Add(-2*time.Hour)),
		reconcileTestGroup(t, heldID, reconcileNow.Add(-2*time.Hour)),
		reconcileTestGroup(t, inFlightID, reconcileNow.Add(-2*time.Hour)),
		reconcileTestGroup(t, missingDestinationID, reconcileNow.Add(-2*time.Hour)),
	)

	stats := uc.ReconcileTransactionGroups(context.Background())

	assert.Equal(t, TransactionGroupReconciliationStats{Scanned: 4, Inconsistent: 2, Skipped: 2}, stats,
		"mixed and half-materialized groups are reported; a held group and one with recent activity are left alone")
	assert.Equal(t, map[string]int64{"inconsistent": 2, "skipped": 2}, collectReconcileResults(t, metricsReader))
	requireNoGroupEvent(t, emitter)
}

func TestReconcileTransactionGroups_LosingTheStatusRaceIsNotARepair(t *testing.T) {
	groupID := uuid.MustParse("0199a800-0000-7000-8000-000000000031")
	settledAt := reconcileNow.Add(-time.Hour)
	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	uc, emitter, _ := newReconcileUseCase(t, repo, &transactionGroupReconcileReader{
		members: map[uuid.UUID][]*transaction.Transaction{
			groupID: {
				reconcileOrigin(groupID, constant.APPROVED, settledAt),
				reconcileDestination(groupID, constant.APPROVED, settledAt),
			},
		},
	})

	expectReconcilePage(repo, uuid.Nil, reconcileTestGroup(t, groupID, reconcileNow.Add(-2*time.Hour)))
	repo.EXPECT().UpdateStatus(gomock.Any(), groupID, constant.PENDING, constant.APPROVED).Return(false, nil)

	stats := uc.ReconcileTransactionGroups(context.Background())

	assert.Equal(t, TransactionGroupReconciliationStats{Scanned: 1, Skipped: 1}, stats)
	requireNoGroupEvent(t, emitter)
}

func TestReconcileTransactionGroups_WalksEveryPageByKeyset(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	reader := &transactionGroupReconcileReader{members: map[uuid.UUID][]*transaction.Transaction{}}
	uc, _, _ := newReconcileUseCase(t, repo, reader)

	firstPage := make([]*transactiongroup.TransactionGroup, transactionGroupReconcilePageSize)
	for index := range firstPage {
		id := uuid.Must(uuid.NewV7())
		firstPage[index] = reconcileTestGroup(t, id, reconcileNow.Add(-2*time.Hour))
		reader.members[id] = []*transaction.Transaction{reconcileOrigin(id, constant.PENDING, reconcileNow.Add(-time.Hour))}
	}

	last := firstPage[len(firstPage)-1].ID
	gomock.InOrder(
		expectReconcilePage(repo, uuid.Nil, firstPage...),
		expectReconcilePage(repo, last),
	)

	stats := uc.ReconcileTransactionGroups(context.Background())

	assert.Equal(t, transactionGroupReconcilePageSize, stats.Scanned)
	assert.Equal(t, transactionGroupReconcilePageSize, stats.Skipped)
}

func TestReconcileTransactionGroups_FailuresAreCountedAndNeverWrite(t *testing.T) {
	t.Run("listing fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := transactiongroup.NewMockRepository(ctrl)
		uc, _, metricsReader := newReconcileUseCase(t, repo, &transactionGroupReconcileReader{})

		repo.EXPECT().ListByStatusOlderThan(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("connection refused"))

		stats := uc.ReconcileTransactionGroups(context.Background())

		assert.Equal(t, TransactionGroupReconciliationStats{Failed: 1}, stats)
		assert.Equal(t, map[string]int64{"failed": 1}, collectReconcileResults(t, metricsReader))
	})

	t.Run("member read fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := transactiongroup.NewMockRepository(ctrl)
		uc, _, _ := newReconcileUseCase(t, repo, &transactionGroupReconcileReader{err: errors.New("connection refused")})

		expectReconcilePage(repo, uuid.Nil, reconcileTestGroup(t, uuid.New(), reconcileNow.Add(-25*time.Hour)))

		stats := uc.ReconcileTransactionGroups(context.Background())

		assert.Equal(t, TransactionGroupReconciliationStats{Scanned: 1, Failed: 1}, stats,
			"an unreadable member set must never look like an orphan")
	})

	t.Run("without the group surface the pass is inert", func(t *testing.T) {
		stats := (&UseCase{}).ReconcileTransactionGroups(context.Background())

		assert.Equal(t, TransactionGroupReconciliationStats{}, stats)
	})
}
