//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func accountRefsLeg(t *testing.T, infra *integrationTestInfra, transactionID, accountID uuid.UUID, side string) uuid.UUID {
	t.Helper()

	return pgtestutil.CreateTestOperation(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, pgtestutil.OperationParams{
		TransactionID: transactionID,
		Description:   "Account refs fixture",
		Type:          side,
		AccountID:     accountID,
		AccountAlias:  "@refs-" + side,
		BalanceID:     uuid.Must(libCommons.GenerateUUIDv7()),
		AssetCode:     "USD",
		Amount:        decimal.NewFromInt(10),
	})
}

func requireTransactionNotFound(t *testing.T, err error) {
	t.Helper()

	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, constant.ErrEntityNotFound.Error(), notFound.Code)
}

func TestIntegration_ListAccountRefsByTransaction_AnswersDistinctAccountsOfEveryLeg(t *testing.T) {
	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	a := uuid.Must(libCommons.GenerateUUIDv7())
	b := uuid.Must(libCommons.GenerateUUIDv7())
	c := uuid.Must(libCommons.GenerateUUIDv7())

	twoLegs := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.APPROVED, decimal.NewFromInt(10), "USD")
	accountRefsLeg(t, infra, twoLegs, a, constant.DEBIT)
	accountRefsLeg(t, infra, twoLegs, b, constant.CREDIT)

	threeLegs := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.APPROVED, decimal.NewFromInt(10), "USD")
	accountRefsLeg(t, infra, threeLegs, a, constant.DEBIT)
	accountRefsLeg(t, infra, threeLegs, b, constant.CREDIT)
	accountRefsLeg(t, infra, threeLegs, c, constant.CREDIT)
	accountRefsLeg(t, infra, threeLegs, c, constant.CREDIT)

	refs, err := infra.repo.ListAccountRefsByTransaction(ctx, infra.orgID, infra.ledgerID, twoLegs)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{a, b}, refs.AccountIDs)

	refs, err = infra.repo.ListAccountRefsByTransaction(ctx, infra.orgID, infra.ledgerID, threeLegs)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{a, b, c}, refs.AccountIDs, "a repeated account is answered once")
}

func TestIntegration_ListAccountRefsByTransaction_ReturnsTheBodyLegs(t *testing.T) {
	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	source := uuid.Must(libCommons.GenerateUUIDv7())

	pending := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.PENDING, decimal.NewFromInt(10), "USD")
	accountRefsLeg(t, infra, pending, source, constant.ONHOLD)

	_, err := infra.pgContainer.DB.Exec(`UPDATE "transaction" SET body = $1 WHERE id = $2`,
		`{"send":{"asset":"USD","value":"10","source":{"from":[{"accountAlias":"@source"}]},"distribute":{"to":[{"accountAlias":"@destination#default"}]}}}`,
		pending)
	require.NoError(t, err)

	refs, err := infra.repo.ListAccountRefsByTransaction(ctx, infra.orgID, infra.ledgerID, pending)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{source}, refs.AccountIDs, "a hold writes only the source rows")
	require.Len(t, refs.Body.Send.Distribute.To, 1)
	assert.Equal(t, "@destination#default", refs.Body.Send.Distribute.To[0].AccountAlias)
	require.Len(t, refs.Body.Send.Source.From, 1)
	assert.Equal(t, "@source", refs.Body.Send.Source.From[0].AccountAlias)
}

func TestIntegration_ListAccountRefsByTransaction_FoundWithoutRowsIsNotNotFound(t *testing.T) {
	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	unprojected := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.PENDING, decimal.NewFromInt(10), "USD")

	refs, err := infra.repo.ListAccountRefsByTransaction(ctx, infra.orgID, infra.ledgerID, unprojected)
	require.NoError(t, err)
	require.NotNil(t, refs)
	assert.Empty(t, refs.AccountIDs)
}

func TestIntegration_ListAccountRefsByTransaction_IsScopedAndHonorsSoftDelete(t *testing.T) {
	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	kept := uuid.Must(libCommons.GenerateUUIDv7())
	dropped := uuid.Must(libCommons.GenerateUUIDv7())

	tx := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		constant.APPROVED, decimal.NewFromInt(10), "USD")
	accountRefsLeg(t, infra, tx, kept, constant.DEBIT)
	droppedOp := accountRefsLeg(t, infra, tx, dropped, constant.CREDIT)

	_, err := infra.pgContainer.DB.Exec(`UPDATE operation SET deleted_at = now() WHERE id = $1`, droppedOp)
	require.NoError(t, err)

	refs, err := infra.repo.ListAccountRefsByTransaction(ctx, infra.orgID, infra.ledgerID, tx)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{kept}, refs.AccountIDs, "a deleted operation names no account")

	_, err = infra.repo.ListAccountRefsByTransaction(ctx, infra.orgID, uuid.Must(libCommons.GenerateUUIDv7()), tx)
	requireTransactionNotFound(t, err)

	_, err = infra.repo.ListAccountRefsByTransaction(ctx, uuid.Must(libCommons.GenerateUUIDv7()), infra.ledgerID, tx)
	requireTransactionNotFound(t, err)

	_, err = infra.repo.ListAccountRefsByTransaction(ctx, infra.orgID, infra.ledgerID, uuid.Must(libCommons.GenerateUUIDv7()))
	requireTransactionNotFound(t, err)

	_, err = infra.pgContainer.DB.Exec(`UPDATE "transaction" SET deleted_at = now() WHERE id = $1`, tx)
	require.NoError(t, err)

	_, err = infra.repo.ListAccountRefsByTransaction(ctx, infra.orgID, infra.ledgerID, tx)
	requireTransactionNotFound(t, err)
}
