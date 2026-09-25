//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transactiongroup

import (
	"context"
	"encoding/json"
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
	assertTransactionGroupEqual(t, group, scoped)

	_, err = repo.Find(ctx, uuid.New(), group.LedgerID, group.ID)
	require.Error(t, err)

	unscoped, err := repo.FindByID(ctx, group.ID)
	require.NoError(t, err)
	assertTransactionGroupEqual(t, group, unscoped)

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

func TestIntegration_TransactionGroup_ListByStatusOlderThanWalksByKeyset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repo := setupTransactionGroupRepo(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	organizationID := uuid.MustParse("0199a110-0000-7000-8000-0000000000a1")
	ledgerID := uuid.MustParse("0199a110-0000-7000-8000-0000000000a2")

	newGroup := func(id string, status string, createdAt time.Time) *TransactionGroup {
		return &TransactionGroup{
			ID: uuid.MustParse(id), OrganizationID: organizationID, LedgerID: ledgerID,
			Status: status, AssetCode: "BRL", Intent: []byte(`{"formatVersion":1,"asset":"BRL","parts":[]}`),
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}
	}

	oldest := newGroup("0199a110-0000-7000-8000-000000000001", "PENDING", cutoff.Add(-3*time.Hour))
	middle := newGroup("0199a110-0000-7000-8000-000000000002", "PENDING", cutoff.Add(-2*time.Hour))
	approved := newGroup("0199a110-0000-7000-8000-000000000003", "APPROVED", cutoff.Add(-2*time.Hour))
	newest := newGroup("0199a110-0000-7000-8000-000000000004", "PENDING", cutoff.Add(-time.Hour))
	young := newGroup("0199a110-0000-7000-8000-000000000005", "PENDING", cutoff.Add(time.Minute))

	for _, group := range []*TransactionGroup{young, newest, approved, middle, oldest} {
		require.NoError(t, repo.Create(ctx, group))
	}

	first, err := repo.ListByStatusOlderThan(ctx, "PENDING", cutoff, uuid.Nil, 2)
	require.NoError(t, err)
	require.Len(t, first, 2)
	assert.Equal(t, oldest.ID, first[0].ID, "pages are ordered by id")
	assertTransactionGroupEqual(t, middle, first[1])

	second, err := repo.ListByStatusOlderThan(ctx, "PENDING", cutoff, first[1].ID, 2)
	require.NoError(t, err)
	require.Len(t, second, 1, "other statuses and groups younger than the cutoff are excluded")
	assert.Equal(t, newest.ID, second[0].ID)

	third, err := repo.ListByStatusOlderThan(ctx, "PENDING", cutoff, second[0].ID, 2)
	require.NoError(t, err)
	assert.Empty(t, third)
}

func TestIntegration_TransactionGroup_DeleteIfMemberlessKeepsGroupsWithMembers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	dsn := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	repo := NewTransactionGroupPostgreSQLRepository(pgtestutil.ConnectPostgresClient(t.Context(), t, dsn, dsn))
	ctx := context.Background()
	createdAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	organizationID := uuid.MustParse("0199a120-0000-7000-8000-0000000000a1")
	ledgerID := uuid.MustParse("0199a120-0000-7000-8000-0000000000a2")

	newGroup := func(id, status string) *TransactionGroup {
		return &TransactionGroup{
			ID: uuid.MustParse(id), OrganizationID: organizationID, LedgerID: ledgerID,
			Status: status, AssetCode: "BRL", Intent: []byte(`{"formatVersion":1,"asset":"BRL","parts":[]}`),
			CreatedAt: createdAt, UpdatedAt: createdAt,
		}
	}

	orphan := newGroup("0199a120-0000-7000-8000-000000000001", "PENDING")
	withMember := newGroup("0199a120-0000-7000-8000-000000000002", "PENDING")
	settled := newGroup("0199a120-0000-7000-8000-000000000003", "APPROVED")

	for _, group := range []*TransactionGroup{orphan, withMember, settled} {
		require.NoError(t, repo.Create(ctx, group))
	}

	memberID := pgtestutil.CreateTestTransaction(t, container.DB, organizationID, ledgerID, pgtestutil.DefaultTransactionParams())
	_, err := container.DB.Exec(`UPDATE transaction SET group_id = $1 WHERE id = $2`, withMember.ID, memberID)
	require.NoError(t, err)

	deleted, err := repo.DeleteIfMemberless(ctx, orphan.ID)
	require.NoError(t, err)
	assert.True(t, deleted)

	deleted, err = repo.DeleteIfMemberless(ctx, withMember.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "a group that has a member row is never deleted")

	deleted, err = repo.DeleteIfMemberless(ctx, settled.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "only a PENDING intent is ever deleted")

	_, err = repo.FindByID(ctx, orphan.ID)
	require.Error(t, err)
	_, err = repo.FindByID(ctx, withMember.ID)
	require.NoError(t, err)
	_, err = repo.FindByID(ctx, settled.ID)
	require.NoError(t, err)
}

func assertTransactionGroupEqual(t *testing.T, want, got *TransactionGroup) {
	t.Helper()
	require.NotNil(t, got)
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.OrganizationID, got.OrganizationID)
	assert.Equal(t, want.LedgerID, got.LedgerID)
	assert.Equal(t, want.Status, got.Status)
	assert.Equal(t, want.AssetCode, got.AssetCode)
	assert.JSONEq(t, string(want.Intent), string(got.Intent))
	assert.True(t, want.CreatedAt.Equal(got.CreatedAt))
	assert.True(t, want.UpdatedAt.Equal(got.UpdatedAt))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(got.Intent, &decoded))
}
