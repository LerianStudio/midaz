//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountexception

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// fixedIntegrationTime is a deterministic UTC instant used by the persistence
// tests. It carries microsecond precision because Postgres round-trips
// timestamps at microsecond precision; a fixed value keeps the round-trip
// assertions reproducible without relying on time.Now().
var fixedIntegrationTime = time.Date(2026, 1, 2, 3, 4, 5, 654321*1000, time.UTC)

// scope holds the organization/ledger/account triple every repository call is
// scoped by, plus the live repository and the raw DB used to seed rows.
type scope struct {
	repo      *AccountExceptionPostgreSQLRepository
	container *pgtestutil.ContainerResult
	orgID     uuid.UUID
	ledgerID  uuid.UUID
	accountID uuid.UUID
}

// newScope provisions a migrated onboarding database with one organization,
// ledger and account, and returns a repository bound to it.
func newScope(t *testing.T) *scope {
	t.Helper()

	container := pgtestutil.SetupMigratedContainer(t, "onboarding")

	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	conn := pgtestutil.ConnectPostgresClient(t.Context(), t, connStr, connStr)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	pgtestutil.CreateTestAsset(t, container.DB, orgID, ledgerID, "USD")
	accountID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Blocked Account", "@blocked", "USD", nil)

	return &scope{
		repo:      NewAccountExceptionPostgreSQLRepository(conn),
		container: container,
		orgID:     orgID,
		ledgerID:  ledgerID,
		accountID: accountID,
	}
}

// mustID parses an identifier the repository just returned. It is a require, not
// a uuid.MustParse, so a malformed identifier is reported as a test failure with
// the offending value rather than as a panic in the middle of the run.
func mustID(t *testing.T, id string) uuid.UUID {
	t.Helper()

	parsed, err := uuid.Parse(id)
	require.NoError(t, err, "repository returned a non-UUID identifier: %q", id)

	return parsed
}

// newException builds a valid entity in the scope, with createdAt controlled by
// the caller so ordering assertions are deterministic.
func (s *scope) newException(createdAt time.Time, codes ...string) *mmodel.AccountException {
	return &mmodel.AccountException{
		ID:                   uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID:       s.orgID.String(),
		LedgerID:             s.ledgerID.String(),
		AccountID:            s.accountID.String(),
		OperationalTypeCodes: codes,
		Context:              "Judicial order 12345/2026",
		CreatedAt:            createdAt,
		UpdatedAt:            createdAt,
	}
}

func TestIntegration_AccountExceptionRepository_Create_PersistsEveryField(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	effectiveAt := fixedIntegrationTime
	expiresAt := fixedIntegrationTime.Add(24 * time.Hour)
	balanceKey := "asset-freeze"

	input := s.newException(fixedIntegrationTime, "PIX_IN", "TED_OUT")
	input.BalanceKey = &balanceKey
	input.EffectiveAt = &effectiveAt
	input.ExpiresAt = &expiresAt

	created, err := s.repo.Create(ctx, s.orgID, s.ledgerID, input)

	require.NoError(t, err, "Create must persist a valid exception")
	require.NotNil(t, created)

	assert.Equal(t, input.ID, created.ID)
	assert.Equal(t, s.orgID.String(), created.OrganizationID)
	assert.Equal(t, s.ledgerID.String(), created.LedgerID)
	assert.Equal(t, s.accountID.String(), created.AccountID)
	assert.Equal(t, []string{"PIX_IN", "TED_OUT"}, created.OperationalTypeCodes,
		"the JSONB code list must survive the database round trip in order")
	require.NotNil(t, created.BalanceKey)
	assert.Equal(t, balanceKey, *created.BalanceKey)
	assert.Equal(t, "Judicial order 12345/2026", created.Context)
	require.NotNil(t, created.EffectiveAt)
	assert.True(t, effectiveAt.Equal(*created.EffectiveAt), "effective_at must round-trip")
	require.NotNil(t, created.ExpiresAt)
	assert.True(t, expiresAt.Equal(*created.ExpiresAt), "expires_at must round-trip")
	assert.Nil(t, created.DeletedAt, "a freshly created exception must be live")
}

func TestIntegration_AccountExceptionRepository_Create_PersistsSparseException(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	created, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime, "PIX_IN"))

	require.NoError(t, err)
	require.NotNil(t, created)

	assert.Nil(t, created.BalanceKey, "an omitted balanceKey must persist as NULL and read back as nil")
	assert.Nil(t, created.EffectiveAt, "an omitted effectiveAt must persist as NULL")
	assert.Nil(t, created.ExpiresAt, "an omitted expiresAt must persist as NULL")
	assert.Equal(t, []string{"PIX_IN"}, created.OperationalTypeCodes)
}

func TestIntegration_AccountExceptionRepository_FindByID_ReturnsException(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	created, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime, "PIX_IN"))
	require.NoError(t, err)

	found, err := s.repo.FindByID(ctx, s.orgID, s.ledgerID, s.accountID, mustID(t, created.ID))

	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, []string{"PIX_IN"}, found.OperationalTypeCodes)
}

func TestIntegration_AccountExceptionRepository_FindByID_ReturnsNotFoundForUnknownID(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	unknownID := uuid.Must(libCommons.GenerateUUIDv7())

	found, err := s.repo.FindByID(ctx, s.orgID, s.ledgerID, s.accountID, unknownID)

	require.Error(t, err)
	assert.Nil(t, found)

	// Assert on the structured code, not on Error(): the rendered string is the
	// human message, and pinning it here would make the test a copy of the
	// catalog wording rather than of the contract.
	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound, "a missing exception must surface an EntityNotFoundError")
	assert.Equal(t, "0503", notFound.Code, "a missing exception must carry error code 0503")
	assert.Equal(t, constant.EntityAccountException, notFound.EntityType)
}

func TestIntegration_AccountExceptionRepository_FindByID_IsScopedByOrgLedgerAndAccount(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	created, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime, "PIX_IN"))
	require.NoError(t, err)

	id := mustID(t, created.ID)

	otherOrgID := pgtestutil.CreateTestOrganization(t, s.container.DB)
	otherLedgerID := pgtestutil.CreateTestLedger(t, s.container.DB, s.orgID)
	otherAccountID := pgtestutil.CreateTestAccount(t, s.container.DB, s.orgID, s.ledgerID, nil, "Other", "@other", "USD", nil)

	tests := []struct {
		name      string
		orgID     uuid.UUID
		ledgerID  uuid.UUID
		accountID uuid.UUID
	}{
		{name: "wrong_organization", orgID: otherOrgID, ledgerID: s.ledgerID, accountID: s.accountID},
		{name: "wrong_ledger", orgID: s.orgID, ledgerID: otherLedgerID, accountID: s.accountID},
		{name: "wrong_account", orgID: s.orgID, ledgerID: s.ledgerID, accountID: otherAccountID},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			found, err := s.repo.FindByID(ctx, tc.orgID, tc.ledgerID, tc.accountID, id)

			require.Error(t, err, "a row must be invisible outside its own org/ledger/account scope")
			assert.Nil(t, found)
		})
	}
}

func TestIntegration_AccountExceptionRepository_FindAllByAccountID_OrdersNewestFirstAndPaginates(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	// Three rows one hour apart, inserted oldest first so the ordering assertion
	// cannot pass by insertion order alone.
	oldest, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime, "OLDEST"))
	require.NoError(t, err)

	middle, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime.Add(time.Hour), "MIDDLE"))
	require.NoError(t, err)

	newest, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime.Add(2*time.Hour), "NEWEST"))
	require.NoError(t, err)

	page1, err := s.repo.FindAllByAccountID(ctx, s.orgID, s.ledgerID, s.accountID, http.Pagination{Limit: 2, Page: 1})
	require.NoError(t, err)
	require.Len(t, page1, 2, "page 1 with limit 2 must return two rows")
	assert.Equal(t, newest.ID, page1[0].ID, "the listing must be newest first")
	assert.Equal(t, middle.ID, page1[1].ID)

	page2, err := s.repo.FindAllByAccountID(ctx, s.orgID, s.ledgerID, s.accountID, http.Pagination{Limit: 2, Page: 2})
	require.NoError(t, err)
	require.Len(t, page2, 1, "page 2 must carry the remaining row via OFFSET")
	assert.Equal(t, oldest.ID, page2[0].ID)

	page3, err := s.repo.FindAllByAccountID(ctx, s.orgID, s.ledgerID, s.accountID, http.Pagination{Limit: 2, Page: 3})
	require.NoError(t, err)
	assert.Empty(t, page3, "a page past the end must be empty, not an error")
}

func TestIntegration_AccountExceptionRepository_FindAllByAccountID_ExcludesOtherAccounts(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	_, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime, "PIX_IN"))
	require.NoError(t, err)

	otherAccountID := pgtestutil.CreateTestAccount(t, s.container.DB, s.orgID, s.ledgerID, nil, "Other", "@other", "USD", nil)

	found, err := s.repo.FindAllByAccountID(ctx, s.orgID, s.ledgerID, otherAccountID, http.Pagination{Limit: 10, Page: 1})

	require.NoError(t, err)
	assert.Empty(t, found, "exceptions of one account must not leak into another account's listing")
}

func TestIntegration_AccountExceptionRepository_ListByAccountID_OrdersOldestFirstAndIgnoresPagination(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	first, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime, "FIRST"))
	require.NoError(t, err)

	second, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime.Add(time.Hour), "SECOND"))
	require.NoError(t, err)

	third, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime.Add(2*time.Hour), "THIRD"))
	require.NoError(t, err)

	listed, err := s.repo.ListByAccountID(ctx, s.orgID, s.ledgerID, s.accountID)

	require.NoError(t, err)
	require.Len(t, listed, 3, "the enrichment loader must return every live rule, unpaginated")
	assert.Equal(t, []string{first.ID, second.ID, third.ID},
		[]string{listed[0].ID, listed[1].ID, listed[2].ID},
		"the loader order is the matching order: oldest first, so the first matching rule wins deterministically")
}

func TestIntegration_AccountExceptionRepository_ListByAccountID_SkipsSoftDeletedRows(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	kept, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime, "KEPT"))
	require.NoError(t, err)

	removed, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime.Add(time.Hour), "REMOVED"))
	require.NoError(t, err)

	require.NoError(t, s.repo.Delete(ctx, s.orgID, s.ledgerID, s.accountID, mustID(t, removed.ID)))

	listed, err := s.repo.ListByAccountID(ctx, s.orgID, s.ledgerID, s.accountID)

	require.NoError(t, err)
	require.Len(t, listed, 1, "a soft-deleted rule must not reach the enrichment loader")
	assert.Equal(t, kept.ID, listed[0].ID)
}

func TestIntegration_AccountExceptionRepository_Update_AppliesPartialChanges(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	balanceKey := "asset-freeze"
	effectiveAt := fixedIntegrationTime
	expiresAt := fixedIntegrationTime.Add(24 * time.Hour)

	input := s.newException(fixedIntegrationTime, "PIX_IN")
	input.BalanceKey = &balanceKey
	input.EffectiveAt = &effectiveAt
	input.ExpiresAt = &expiresAt

	created, err := s.repo.Create(ctx, s.orgID, s.ledgerID, input)
	require.NoError(t, err)

	id := mustID(t, created.ID)

	// Only the codes are supplied: every other stored field must survive.
	updated, err := s.repo.Update(ctx, s.orgID, s.ledgerID, s.accountID, id, &mmodel.AccountException{
		OperationalTypeCodes: []string{"TED_OUT", "BOLETO_IN"},
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, []string{"TED_OUT", "BOLETO_IN"}, updated.OperationalTypeCodes, "supplied codes must replace the stored list wholesale")
	require.NotNil(t, updated.BalanceKey, "an omitted balanceKey must leave the stored key unchanged")
	assert.Equal(t, balanceKey, *updated.BalanceKey)
	assert.Equal(t, "Judicial order 12345/2026", updated.Context, "an omitted context must leave the stored value unchanged")
	require.NotNil(t, updated.EffectiveAt, "an omitted effectiveAt must leave the stored bound unchanged")
	assert.True(t, updated.UpdatedAt.After(created.UpdatedAt), "every update must advance updated_at")
}

func TestIntegration_AccountExceptionRepository_Update_EmptyBalanceKeyClearsTheRestriction(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	balanceKey := "asset-freeze"
	input := s.newException(fixedIntegrationTime, "PIX_IN")
	input.BalanceKey = &balanceKey

	created, err := s.repo.Create(ctx, s.orgID, s.ledgerID, input)
	require.NoError(t, err)
	require.NotNil(t, created.BalanceKey)

	cleared := ""

	updated, err := s.repo.Update(ctx, s.orgID, s.ledgerID, s.accountID, mustID(t, created.ID), &mmodel.AccountException{
		BalanceKey: &cleared,
	})

	require.NoError(t, err)
	assert.Nil(t, updated.BalanceKey,
		"the empty-string clear sentinel must widen the rule back to every balance, which on the column is NULL")
}

func TestIntegration_AccountExceptionRepository_Update_ReturnsNotFoundForUnknownID(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	unknownID := uuid.Must(libCommons.GenerateUUIDv7())

	updated, err := s.repo.Update(ctx, s.orgID, s.ledgerID, s.accountID, unknownID, &mmodel.AccountException{
		Context: "does not matter",
	})

	require.Error(t, err)
	assert.Nil(t, updated)
	assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound)
}

func TestIntegration_AccountExceptionRepository_Update_IgnoresSoftDeletedRows(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	created, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime, "PIX_IN"))
	require.NoError(t, err)

	id := mustID(t, created.ID)
	require.NoError(t, s.repo.Delete(ctx, s.orgID, s.ledgerID, s.accountID, id))

	updated, err := s.repo.Update(ctx, s.orgID, s.ledgerID, s.accountID, id, &mmodel.AccountException{
		Context: "resurrect me",
	})

	require.Error(t, err, "a soft-deleted rule must not be updatable")
	assert.Nil(t, updated)
	assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound)
}

func TestIntegration_AccountExceptionRepository_Delete_SoftDeletesAndIsNotRepeatable(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	created, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime, "PIX_IN"))
	require.NoError(t, err)

	id := mustID(t, created.ID)

	require.NoError(t, s.repo.Delete(ctx, s.orgID, s.ledgerID, s.accountID, id))

	// The row is still on disk (soft delete), but invisible to every read.
	var deletedAt *time.Time
	require.NoError(t,
		s.container.DB.QueryRow(`SELECT deleted_at FROM account_exception WHERE id = $1`, id).Scan(&deletedAt),
		"the row must remain physically present after a soft delete")
	require.NotNil(t, deletedAt, "deleted_at must be stamped")

	found, err := s.repo.FindByID(ctx, s.orgID, s.ledgerID, s.accountID, id)
	require.Error(t, err, "a soft-deleted rule must be invisible to FindByID")
	assert.Nil(t, found)

	err = s.repo.Delete(ctx, s.orgID, s.ledgerID, s.accountID, id)
	require.Error(t, err, "deleting an already-deleted rule must not silently succeed")
	assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound)
}

func TestIntegration_AccountExceptionRepository_Delete_IsScopedByAccount(t *testing.T) {
	s := newScope(t)
	ctx := context.Background()

	created, err := s.repo.Create(ctx, s.orgID, s.ledgerID, s.newException(fixedIntegrationTime, "PIX_IN"))
	require.NoError(t, err)

	otherAccountID := pgtestutil.CreateTestAccount(t, s.container.DB, s.orgID, s.ledgerID, nil, "Other", "@other", "USD", nil)

	err = s.repo.Delete(ctx, s.orgID, s.ledgerID, otherAccountID, mustID(t, created.ID))

	require.Error(t, err, "a delete addressed to the wrong account must not touch the row")
	assert.ErrorIs(t, err, services.ErrDatabaseItemNotFound)

	found, findErr := s.repo.FindByID(ctx, s.orgID, s.ledgerID, s.accountID, mustID(t, created.ID))
	require.NoError(t, findErr, "the row must still be live after the misaddressed delete")
	assert.NotNil(t, found)
}
