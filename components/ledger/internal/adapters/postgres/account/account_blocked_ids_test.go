// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockedAccountIDsQuery feeds the hydration of the blocked-accounts Redis SET,
// which is the enforcement index for account blocking. These assertions pin the
// three properties a hydration read cannot get wrong: it must be scoped to one
// ledger, it must exclude soft-deleted rows, and it must return the WHOLE
// blocked population — a paged read would hydrate a partial index that reports
// itself as complete.

func TestBlockedAccountIDsQuery_ProjectsOnlyTheID(t *testing.T) {
	t.Parallel()

	repo := &AccountPostgreSQLRepository{tableName: "account"}

	query, _, err := repo.blockedAccountIDsQuery(uuid.New(), uuid.New())
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(query, "SELECT id FROM account"),
		"hydration only needs the identifier, got: %s", query)
}

func TestBlockedAccountIDsQuery_ScopesTenantAndExcludesDeleted(t *testing.T) {
	t.Parallel()

	repo := &AccountPostgreSQLRepository{tableName: "account"}

	organizationID := uuid.New()
	ledgerID := uuid.New()

	query, args, err := repo.blockedAccountIDsQuery(organizationID, ledgerID)
	require.NoError(t, err)

	assert.Contains(t, query, "organization_id = $1")
	assert.Contains(t, query, "ledger_id = $2")
	assert.Contains(t, query, "blocked = $3")
	assert.Contains(t, query, "deleted_at IS NULL",
		"a soft-deleted account must never be hydrated into the enforcement index")

	// uuid.UUID satisfies driver.Valuer, so squirrel binds it as its canonical
	// string form — the same shape every other read in this repository sends.
	require.Len(t, args, 3)
	assert.Equal(t, organizationID.String(), args[0])
	assert.Equal(t, ledgerID.String(), args[1])
	assert.Equal(t, true, args[2])
}

// TestBlockedAccountIDsQuery_IsNotPaginated is a correctness assertion, not a
// style one: the SET is only marked hydrated after every member landed, so a
// query that silently truncated its result would produce an index that claims
// completeness while missing blocked accounts.
func TestBlockedAccountIDsQuery_IsNotPaginated(t *testing.T) {
	t.Parallel()

	repo := &AccountPostgreSQLRepository{tableName: "account"}

	query, _, err := repo.blockedAccountIDsQuery(uuid.New(), uuid.New())
	require.NoError(t, err)

	assert.NotContains(t, query, "LIMIT")
	assert.NotContains(t, query, "OFFSET")
}
