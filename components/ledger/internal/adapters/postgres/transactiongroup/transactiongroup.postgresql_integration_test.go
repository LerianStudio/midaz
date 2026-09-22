//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transactiongroup

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func setupTransactionGroupRepo(t *testing.T) *TransactionGroupPostgreSQLRepository {
	t.Helper()

	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	dsn := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	client := pgtestutil.ConnectPostgresClient(t.Context(), t, dsn, dsn)

	return NewTransactionGroupPostgreSQLRepository(client)
}

func TestIntegration_TransactionGroup_LifecycleAndScope(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repo := setupTransactionGroupRepo(t)
	ctx := context.Background()
	group := &TransactionGroup{
		ID:             uuid.MustParse("0199a100-0000-7000-8000-000000000001"),
		OrganizationID: uuid.MustParse("0199a100-0000-7000-8000-000000000002"),
		LedgerID:       uuid.MustParse("0199a100-0000-7000-8000-000000000003"),
		Status:         "PENDING",
		AssetCode:      "BRL",
		Intent:         []byte(`{"formatVersion":1,"asset":"BRL","parts":[]}`),
		CreatedAt:      time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC),
	}

	require.NoError(t, repo.Create(ctx, group))

	scoped, err := repo.Find(ctx, group.OrganizationID, group.LedgerID, group.ID)
	require.NoError(t, err)
	assert.Equal(t, group, scoped)

	_, err = repo.Find(ctx, uuid.New(), group.LedgerID, group.ID)
	require.Error(t, err)

	unscoped, err := repo.FindByID(ctx, group.ID)
	require.NoError(t, err)
	assert.Equal(t, group, unscoped)

	updated, err := repo.UpdateStatus(ctx, group.ID, "PENDING", "APPROVED")
	require.NoError(t, err)
	assert.True(t, updated)

	updated, err = repo.UpdateStatus(ctx, group.ID, "PENDING", "CANCELED")
	require.NoError(t, err)
	assert.False(t, updated, "compare-and-swap must reject a stale source status")

	require.NoError(t, repo.Delete(ctx, group.ID))
	_, err = repo.FindByID(ctx, group.ID)
	require.Error(t, err)
}
