//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// =============================================================================
// UpdateStatusFromPending (STATUS CAS) INTEGRATION TESTS
// =============================================================================
// The compare-and-set is the DURABLE backstop of the commit/cancel transition:
// the Redis cross-transition gate evaporates with its marker's TTL, this does
// not. A transition flips the row only while it is still PENDING, so a commit
// and a cancel of the same transaction cannot both settle it — which is what let
// the incident flip a row twice and create money.
//
// Zero rows affected is deliberately NOT an error: the row's existence was
// established when the transition loaded it, so zero rows is a lost race and the
// caller decides what it means.
//
// Scope is the adapter layer only — a real Postgres from testcontainers.

// pendingTransactionBody is the persisted body a pending create leaves behind, so
// a seeded PENDING row is shaped like a real one.
func pendingTransactionBody() mtransaction.Transaction {
	amount := decimal.NewFromInt(100)

	return mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "USD",
			Value: amount,
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{{
					AccountAlias: "@payer",
					IsFrom:       true,
					Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
				}},
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{{
					AccountAlias: "@payee",
					Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
				}},
			},
		},
	}
}

// seedTransactionWithStatus persists a transaction in the given status. A PENDING
// row carries the body the create persisted; a terminal one is seeded the way the
// direct path writes it.
func seedTransactionWithStatus(t *testing.T, infra *integrationTestInfra, statusCode string, deletedAt *time.Time) *Transaction {
	t.Helper()

	description := statusCode

	tx := &Transaction{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		Description:    "seeded " + statusCode,
		Status:         Status{Code: statusCode, Description: &description},
		Amount:         decimalPtr(1000),
		AssetCode:      "USD",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
		DeletedAt:      deletedAt,
	}

	if statusCode == constant.PENDING {
		tx.Body = pendingTransactionBody()
	}

	created, err := infra.repo.Create(context.Background(), tx)
	require.NoError(t, err)

	return created
}

// transitionOf builds the entity a transition hands the repository: the terminal
// status, its description, and the empty body that nulls the persisted one.
func transitionOf(row *Transaction, statusCode string) *Transaction {
	description := statusCode

	return &Transaction{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		LedgerID:       row.LedgerID,
		Status:         Status{Code: statusCode, Description: &description},
	}
}

// readStatusRow reads the columns the CAS writes straight off the table, so the
// body-NULL contract is asserted against storage rather than through the entity
// mapper.
func readStatusRow(t *testing.T, infra *integrationTestInfra, id string) (status string, statusDescription *string, bodyIsNull bool) {
	t.Helper()

	err := infra.pgContainer.DB.QueryRowContext(context.Background(),
		`SELECT status, status_description, body IS NULL FROM transaction WHERE id = $1`, id).
		Scan(&status, &statusDescription, &bodyIsNull)
	require.NoError(t, err)

	return status, statusDescription, bodyIsNull
}

// TestIntegration_UpdateStatusFromPending_FlipsPendingRowOnce covers cases (a)
// and (b): the first transition lands on a PENDING row and nulls its body, and
// every repetition — same status or the opposite one — reports false without an
// error and leaves the row exactly as the first one left it.
func TestIntegration_UpdateStatusFromPending_FlipsPendingRowOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	row := seedTransactionWithStatus(t, infra, constant.PENDING, nil)

	_, _, bodyIsNull := readStatusRow(t, infra, row.ID)
	require.False(t, bodyIsNull, "a seeded PENDING row must carry the persisted body")

	updated, transitioned, err := infra.repo.UpdateStatusFromPending(ctx,
		infra.orgID, infra.ledgerID, parseID(t, row.ID), transitionOf(row, constant.APPROVED))

	require.NoError(t, err)
	require.True(t, transitioned, "a PENDING row must be transitioned")
	require.NotNil(t, updated)
	assert.Equal(t, constant.APPROVED, updated.Status.Code)

	status, statusDescription, bodyIsNull := readStatusRow(t, infra, row.ID)
	assert.Equal(t, constant.APPROVED, status)
	require.NotNil(t, statusDescription)
	assert.Equal(t, constant.APPROVED, *statusDescription)
	assert.True(t, bodyIsNull, "a terminal transition nulls the persisted body")

	settled, _, settledBodyIsNull := readStatusRow(t, infra, row.ID)

	// (b) The repetition, in both flavours: a resend of the same transition and
	// the opposite one. Neither may land, and neither is an error.
	for _, statusCode := range []string{constant.APPROVED, constant.CANCELED} {
		again, transitionedAgain, againErr := infra.repo.UpdateStatusFromPending(ctx,
			infra.orgID, infra.ledgerID, parseID(t, row.ID), transitionOf(row, statusCode))

		require.NoError(t, againErr, "a lost race is not an error: %s", statusCode)
		assert.False(t, transitionedAgain, "%s must not flip an already-settled row", statusCode)
		assert.Nil(t, again, "a compare-and-set that matched nothing returns no row")

		nowStatus, _, nowBodyIsNull := readStatusRow(t, infra, row.ID)
		assert.Equal(t, settled, nowStatus, "the settled row must not move")
		assert.Equal(t, settledBodyIsNull, nowBodyIsNull)
	}
}

// TestIntegration_UpdateStatusFromPending_TerminalRowIsNeverFlipped covers case
// (c): a row seeded directly in a terminal status was never PENDING, so no
// transition may claim it.
func TestIntegration_UpdateStatusFromPending_TerminalRowIsNeverFlipped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	for _, seededStatus := range []string{constant.APPROVED, constant.CANCELED} {
		t.Run(seededStatus, func(t *testing.T) {
			row := seedTransactionWithStatus(t, infra, seededStatus, nil)

			for _, target := range []string{constant.APPROVED, constant.CANCELED} {
				updated, transitioned, err := infra.repo.UpdateStatusFromPending(ctx,
					infra.orgID, infra.ledgerID, parseID(t, row.ID), transitionOf(row, target))

				require.NoError(t, err)
				assert.False(t, transitioned, "%s row must not accept a %s transition", seededStatus, target)
				assert.Nil(t, updated)
			}

			status, _, _ := readStatusRow(t, infra, row.ID)
			assert.Equal(t, seededStatus, status, "the row must keep the status it was seeded with")
		})
	}
}

// TestIntegration_UpdateStatusFromPending_GenericUpdateKeepsPatchSemantics covers
// case (d): the status predicate belongs to the CAS alone. The generic Update
// still serves the description/metadata PATCH on a terminal row, which is the
// surface a status predicate there would have broken.
func TestIntegration_UpdateStatusFromPending_GenericUpdateKeepsPatchSemantics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	row := seedTransactionWithStatus(t, infra, constant.APPROVED, nil)

	patched, err := infra.repo.Update(ctx, infra.orgID, infra.ledgerID, parseID(t, row.ID),
		&Transaction{Description: "patched after settlement"})

	require.NoError(t, err, "the generic Update must still patch a terminal row")
	assert.Equal(t, "patched after settlement", patched.Description)

	status, _, _ := readStatusRow(t, infra, row.ID)
	assert.Equal(t, constant.APPROVED, status, "a description PATCH must not touch the status")

	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, parseID(t, row.ID))
	require.NoError(t, err)
	assert.Equal(t, "patched after settlement", found.Description)
}

// TestIntegration_UpdateStatusFromPending_ConcurrentTransitionsElectOneWinner
// covers case (e), and is the case the whole method exists for: commits and
// cancels racing on the SAME pending row. Exactly one may win — anything else is
// the double transition that created money in the incident.
func TestIntegration_UpdateStatusFromPending_ConcurrentTransitionsElectOneWinner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	const racers = 8

	row := seedTransactionWithStatus(t, infra, constant.PENDING, nil)

	var (
		start     = make(chan struct{})
		wg        sync.WaitGroup
		mu        sync.Mutex
		winners   []string
		losers    int
		failures  []error
		winnerRow *Transaction
	)

	wg.Add(racers)

	for i := 0; i < racers; i++ {
		// Half commit, half cancel — the exact shape of the incident.
		target := constant.APPROVED
		if i%2 == 1 {
			target = constant.CANCELED
		}

		go func(target string) {
			defer wg.Done()

			<-start

			updated, transitioned, err := infra.repo.UpdateStatusFromPending(ctx,
				infra.orgID, infra.ledgerID, parseID(t, row.ID), transitionOf(row, target))

			mu.Lock()
			defer mu.Unlock()

			switch {
			case err != nil:
				failures = append(failures, err)
			case transitioned:
				winners = append(winners, target)
				winnerRow = updated
			default:
				losers++
			}
		}(target)
	}

	close(start)
	wg.Wait()

	require.Empty(t, failures, "a lost race must never surface as an error")
	require.Len(t, winners, 1, "exactly one transition may flip a pending row")
	assert.Equal(t, racers-1, losers)
	require.NotNil(t, winnerRow)

	status, _, bodyIsNull := readStatusRow(t, infra, row.ID)
	assert.Equal(t, winners[0], status, "the persisted status must be the winner's")
	assert.Equal(t, winners[0], winnerRow.Status.Code)
	assert.True(t, bodyIsNull, "the winning transition nulls the body exactly once")
}

// TestIntegration_UpdateStatusFromPending_SoftDeletedRowIsNeverFlipped covers
// case (f): the WHERE carries `deleted_at IS NULL`, so a soft-deleted pending is
// out of reach of a transition even though its status still reads PENDING.
func TestIntegration_UpdateStatusFromPending_SoftDeletedRowIsNeverFlipped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	deletedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	row := seedTransactionWithStatus(t, infra, constant.PENDING, &deletedAt)

	updated, transitioned, err := infra.repo.UpdateStatusFromPending(ctx,
		infra.orgID, infra.ledgerID, parseID(t, row.ID), transitionOf(row, constant.APPROVED))

	require.NoError(t, err)
	assert.False(t, transitioned, "a soft-deleted row is out of reach of a transition")
	assert.Nil(t, updated)

	status, _, bodyIsNull := readStatusRow(t, infra, row.ID)
	assert.Equal(t, constant.PENDING, status)
	assert.False(t, bodyIsNull, "a row the CAS did not match must keep its body")
}
