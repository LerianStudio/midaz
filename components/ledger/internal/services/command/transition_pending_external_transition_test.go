// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// transactionRowRepository answers the row-only read of the primary separately
// from the indexed engine view, so a test can make the two disagree the way a
// writer without the engine guard leaves them.
type transactionRowRepository struct {
	transaction.Repository
	row          *transaction.Transaction
	rowErr       error
	rowReads     int
	rowOnPrimary bool
}

func (repo *transactionRowRepository) Find(ctx context.Context, _, _, _ uuid.UUID) (*transaction.Transaction, error) {
	repo.rowReads++
	repo.rowOnPrimary = readrouting.IsPrimaryRead(ctx)

	return repo.row, repo.rowErr
}

type pendingTransitionEntry struct {
	name   string
	status string
	call   func(*UseCase, context.Context, PendingTransitionInput) error
}

func pendingTransitionEntries() []pendingTransitionEntry {
	return []pendingTransitionEntry{
		{"commit v1", constant.APPROVED, func(uc *UseCase, ctx context.Context, in PendingTransitionInput) error {
			_, err := uc.CommitTransactionV1(ctx, in)
			return err
		}},
		{"cancel v1", constant.CANCELED, func(uc *UseCase, ctx context.Context, in PendingTransitionInput) error {
			_, err := uc.CancelTransactionV1(ctx, in)
			return err
		}},
		{"commit v2", constant.APPROVED, func(uc *UseCase, ctx context.Context, in PendingTransitionInput) error {
			_, err := uc.CommitTransactionV2(ctx, in)
			return err
		}},
		{"cancel v2", constant.CANCELED, func(uc *UseCase, ctx context.Context, in PendingTransitionInput) error {
			_, err := uc.CancelTransactionV2(ctx, in)
			return err
		}},
	}
}

func newPrimaryRowTransition(t *testing.T, status string, executionID uuid.UUID, durable bool) (*UseCase, *transactionRowRepository, *transitionEngineExecutor, PendingTransitionInput) {
	t.Helper()

	uc, reader, executor, _, in := newTransitionEngineUseCase(t, status)
	uc.TransactionReader = &pendingProjectionReader{transitionEngineReader: reader, executionID: executionID, durable: durable}
	row := *reader.persisted
	row.Operations = nil
	rows := &transactionRowRepository{row: &row}
	uc.TransactionRepo = rows

	return uc, rows, executor, in
}

// TestPendingTransitionRefusesDurableHoldTransitionedOutsideTheEngine covers a
// hold whose durable row a writer without the engine guard already moved out of
// PENDING: the engine index and guard still read PENDING, so applying the second
// transition would release or credit funds a second time.
func TestPendingTransitionRefusesDurableHoldTransitionedOutsideTheEngine(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	for _, entry := range pendingTransitionEntries() {
		for _, primaryStatus := range []string{constant.APPROVED, constant.CANCELED} {
			t.Run(entry.name+" over "+primaryStatus+" primary", func(t *testing.T) {
				uc, reader, executor, in := newPrimaryRowTransition(t, entry.status, uuid.New(), true)
				reader.row.Status.Code = primaryStatus
				uc.TransactionRedisRepo.(*txRedis.MockRedisRepository).EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

				err := entry.call(uc, tmcore.ContextWithTenantID(t.Context(), "tenant-external-transition"), in)

				assertBusinessCode(t, err, constant.ErrCommitTransactionNotPending.Error())
				assert.Equal(t, 1, reader.rowReads)
				assert.True(t, reader.rowOnPrimary, "the row must be read from the primary, not a replica")
				assert.Empty(t, executor.guardCalls, "a refused transition must not bootstrap the guard")
				assert.Empty(t, executor.requests, "a refused transition must not reach the engine")
			})
		}
	}
}

// TestPendingTransitionConsultsThePrimaryOnlyForDurableIndexedExecutions keeps
// the transitions that must not depend on the row: before projection the row
// does not exist yet, and without an index the view already came from the
// primary.
func TestPendingTransitionConsultsThePrimaryOnlyForDurableIndexedExecutions(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	missingRow := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransaction)

	for _, test := range []struct {
		name         string
		executionID  uuid.UUID
		durable      bool
		rowErr       error
		wantRowReads int
	}{
		{name: "durable execution with a pending primary", executionID: uuid.New(), durable: true, wantRowReads: 1},
		{name: "execution not yet projected", executionID: uuid.New(), durable: false, rowErr: missingRow},
		{name: "no indexed execution", executionID: uuid.Nil, durable: true, rowErr: missingRow},
	} {
		for _, entry := range pendingTransitionEntries() {
			t.Run(test.name+" "+entry.name, func(t *testing.T) {
				uc, reader, executor, in := newPrimaryRowTransition(t, entry.status, test.executionID, test.durable)
				reader.rowErr = test.rowErr

				err := entry.call(uc, tmcore.ContextWithTenantID(t.Context(), "tenant-external-transition"), in)

				require.NoError(t, err)
				assert.Equal(t, test.wantRowReads, reader.rowReads)
				require.Len(t, executor.requests, 1)
			})
		}
	}
}

// TestPendingTransitionFailsClosedWhenTheDurableRowCannotBeConfirmed covers a
// durable index whose row the primary does not return: a vanished durable row is
// an anomaly, not evidence that the hold is still open.
func TestPendingTransitionFailsClosedWhenTheDurableRowCannotBeConfirmed(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	unavailable := errors.New("primary unavailable")

	for _, test := range []struct {
		name    string
		rowErr  error
		noRow   bool
		wantErr error
	}{
		{name: "row not found", rowErr: pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransaction), wantErr: ErrInvalidTransactionCompletionRecord},
		{name: "empty row", noRow: true, wantErr: ErrInvalidTransactionCompletionRecord},
		{name: "primary read failure", rowErr: unavailable, wantErr: unavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			uc, reader, executor, in := newPrimaryRowTransition(t, constant.APPROVED, uuid.New(), true)
			reader.rowErr = test.rowErr
			uc.TransactionRedisRepo.(*txRedis.MockRedisRepository).EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			if test.noRow {
				reader.row = nil
			}

			_, err := uc.CommitTransactionV1(tmcore.ContextWithTenantID(t.Context(), "tenant-external-transition"), in)

			require.ErrorIs(t, err, test.wantErr)
			var notFound pkg.EntityNotFoundError
			assert.False(t, errors.As(err, &notFound), "a durable row that cannot be read must not answer as a missing transaction")
			assert.Empty(t, executor.guardCalls)
			assert.Empty(t, executor.requests)
		})
	}
}
