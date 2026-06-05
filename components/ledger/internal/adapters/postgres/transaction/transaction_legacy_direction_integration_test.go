//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"database/sql"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var legacyTransactionDirectionTestTime = time.Date(2026, time.May, 21, 12, 0, 0, 0, time.UTC)

func legacyTransactionDirectionTestContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	return ctx
}

func insertLegacyDirectionNullOperationForTransaction(
	t *testing.T,
	infra *integrationTestInfra,
	transactionID uuid.UUID,
	opType string,
) uuid.UUID {
	t.Helper()

	id := uuid.Must(libCommons.GenerateUUIDv7())
	amount := decimal.NewFromInt(100)
	available := decimal.NewFromInt(1000)
	zero := decimal.Zero

	_, err := infra.pgContainer.DB.Exec(`
		INSERT INTO operation (
			id, transaction_id, description, type, account_id, account_alias,
			balance_id, balance_key, asset_code, chart_of_accounts, amount,
			available_balance, on_hold_balance, available_balance_after,
			on_hold_balance_after, balance_version_before, balance_version_after,
			status, balance_affected, organization_id, ledger_id, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
	`,
		id, transactionID, "legacy v3.5.3 fixture", opType,
		infra.accountID, "@legacy-account", infra.balanceID, "default",
		"USD", "default", amount, available, zero, available.Sub(amount), zero,
		int64(1), int64(2), "APPROVED", true,
		infra.orgID, infra.ledgerID, legacyTransactionDirectionTestTime, legacyTransactionDirectionTestTime,
	)
	require.NoError(t, err)

	var direction sql.NullString
	require.NoError(t, infra.pgContainer.DB.QueryRow(`SELECT direction FROM operation WHERE id = $1`, id).Scan(&direction))
	require.False(t, direction.Valid, "fixture must persist with direction = NULL")

	return id
}

func TestIntegration_TransactionRepository_LegacyDirectionFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	ctx := legacyTransactionDirectionTestContext(t)
	tx := infra.createTestTransaction(t, "legacy direction transaction read")
	txID := parseID(t, tx.ID)
	opID := insertLegacyDirectionNullOperationForTransaction(t, infra, txID, constant.ONHOLD)

	found, err := infra.repo.FindWithOperations(ctx, infra.orgID, infra.ledgerID, txID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assertTransactionOperationDirection(t, found, opID.String(), constant.DirectionDebit)

	transactions, _, err := infra.repo.FindOrListAllWithOperations(
		ctx,
		infra.orgID,
		infra.ledgerID,
		[]uuid.UUID{txID},
		http.Pagination{Limit: 10},
	)
	require.NoError(t, err)
	require.Len(t, transactions, 1)
	assertTransactionOperationDirection(t, transactions[0], opID.String(), constant.DirectionDebit)
}

func assertTransactionOperationDirection(t *testing.T, tx *Transaction, operationID, wantDirection string) {
	t.Helper()

	for _, op := range tx.Operations {
		if op.ID == operationID {
			assert.Equal(t, wantDirection, op.Direction)
			return
		}
	}

	t.Fatalf("operation %s not returned", operationID)
}
