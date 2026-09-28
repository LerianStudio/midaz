//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package integration

import (
	"testing"

	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// routeLedgerNullableVersion is the transaction migration that lets accounting
// routes exist at organization level, without a ledger.
const routeLedgerNullableVersion = 43

// TestIntegration_TransactionMigrations_RouteLedgerNullable proves that routes
// without a ledger are accepted once the migration is applied, and that rolling
// it back refuses while such routes exist instead of silently rewriting them.
func TestIntegration_TransactionMigrations_RouteLedgerNullable(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	m := newMigrator(t, container.DB, "transaction")

	require.NoError(t, m.Migrate(routeLedgerNullableVersion), "migrate transaction module to the nullable route ledger version")

	orgID := uuid.New()
	operationRouteID := uuid.New()
	transactionRouteID := uuid.New()

	_, err := container.DB.Exec(`
		INSERT INTO operation_route (id, organization_id, ledger_id, title, operation_type, created_at, updated_at)
		VALUES ($1, $2, NULL, 'Organization source', 'source', now(), now())
	`, operationRouteID, orgID)
	require.NoError(t, err, "an operation route without a ledger must be accepted")

	_, err = container.DB.Exec(`
		INSERT INTO transaction_route (id, organization_id, ledger_id, title, created_at, updated_at)
		VALUES ($1, $2, NULL, 'Organization route', now(), now())
	`, transactionRouteID, orgID)
	require.NoError(t, err, "a transaction route without a ledger must be accepted")

	err = m.Migrate(routeLedgerNullableVersion - 1)
	require.Error(t, err, "rolling back must refuse while routes without a ledger exist")

	// A refused down leaves golang-migrate dirty at the version it was leaving;
	// clear that state before proving the rollback succeeds once no NULL rows remain.
	require.NoError(t, m.Force(routeLedgerNullableVersion))

	_, err = container.DB.Exec(`DELETE FROM transaction_route WHERE id = $1`, transactionRouteID)
	require.NoError(t, err)

	_, err = container.DB.Exec(`DELETE FROM operation_route WHERE id = $1`, operationRouteID)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(routeLedgerNullableVersion-1), "rolling back without routes lacking a ledger must succeed")
}
