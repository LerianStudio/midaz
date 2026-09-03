//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// ============================================================================
// ListBlockedAccountIDs Tests
// ============================================================================
// ListBlockedAccountIDs is the source-of-truth read that hydrates the
// blocked-accounts Redis SET. Everything the enforcement index believes comes
// from here, so these tests pin the scope of the read against a real schema.

// seedBlockedAccount inserts one account with the given block state and alias.
func seedBlockedAccount(t *testing.T, container *pgtestutil.ContainerResult, orgID, ledgerID uuid.UUID, alias string, blocked bool, deletedAt *time.Time) uuid.UUID {
	t.Helper()

	params := pgtestutil.DefaultAccountParams()
	params.Name = "Account " + alias
	params.Alias = alias
	params.Blocked = blocked
	params.DeletedAt = deletedAt

	return pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, params)
}

func TestIntegration_AccountRepository_ListBlockedAccountIDs_ReturnsOnlyLiveBlockedAccounts(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	deletedAt := time.Now().UTC()

	firstBlocked := seedBlockedAccount(t, container, orgID, ledgerID, "@blocked-1", true, nil)
	secondBlocked := seedBlockedAccount(t, container, orgID, ledgerID, "@blocked-2", true, nil)
	seedBlockedAccount(t, container, orgID, ledgerID, "@active-1", false, nil)
	seedBlockedAccount(t, container, orgID, ledgerID, "@blocked-deleted", true, &deletedAt)

	// Act
	got, err := repo.ListBlockedAccountIDs(context.Background(), orgID, ledgerID)

	// Assert
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{firstBlocked, secondBlocked}, got,
		"a soft-deleted account must not be hydrated into the enforcement index")
}

func TestIntegration_AccountRepository_ListBlockedAccountIDs_ScopesToOrganizationAndLedger(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	otherLedgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	otherOrgID := pgtestutil.CreateTestOrganization(t, container.DB)
	otherOrgLedgerID := pgtestutil.CreateTestLedger(t, container.DB, otherOrgID)

	inScope := seedBlockedAccount(t, container, orgID, ledgerID, "@in-scope", true, nil)
	seedBlockedAccount(t, container, orgID, otherLedgerID, "@other-ledger", true, nil)
	seedBlockedAccount(t, container, otherOrgID, otherOrgLedgerID, "@other-org", true, nil)

	// Act
	got, err := repo.ListBlockedAccountIDs(context.Background(), orgID, ledgerID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{inScope}, got,
		"the index is per-ledger: hydration must not pull a block from another ledger or organization")
}

// TestIntegration_AccountRepository_ListBlockedAccountIDs_EmptyLedgerIsNotAnError
// matters because a ledger with nothing blocked is a legitimate, fully-known
// state that still has to be hydratable — an error here would leave that ledger
// permanently unhydrated and every transaction on it retrying forever.
func TestIntegration_AccountRepository_ListBlockedAccountIDs_EmptyLedgerIsNotAnError(t *testing.T) {
	// Arrange
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	seedBlockedAccount(t, container, orgID, ledgerID, "@active-only", false, nil)

	// Act
	got, err := repo.ListBlockedAccountIDs(context.Background(), orgID, ledgerID)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, got)
}
