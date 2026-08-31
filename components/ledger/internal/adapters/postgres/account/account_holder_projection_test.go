// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"testing"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// holderIDPosition and holderCheckSkippedPosition are the indices the positional
// Scan calls in every read assume for the two holder columns. Both projections
// must agree on them, or a read decodes the wrong column into the wrong field.
const (
	holderIDPosition           = 4
	holderCheckSkippedPosition = 18
	accountColumnCount         = 19
)

func TestAccountColumns_HolderProjectionKeepsShape(t *testing.T) {
	withHolder := accountColumns(true)
	withoutHolder := accountColumns(false)

	require.Len(t, withHolder, accountColumnCount)
	require.Len(t, withoutHolder, accountColumnCount,
		"withholding the holder columns must substitute them, not drop them")

	assert.Equal(t, "holder_id", withHolder[holderIDPosition])
	assert.Equal(t, "holder_check_skipped", withHolder[holderCheckSkippedPosition])

	assert.Equal(t, "NULL::uuid AS holder_id", withoutHolder[holderIDPosition])
	assert.Equal(t, "FALSE AS holder_check_skipped", withoutHolder[holderCheckSkippedPosition])

	for i := range withHolder {
		if i == holderIDPosition || i == holderCheckSkippedPosition {
			continue
		}

		assert.Equal(t, withHolder[i], withoutHolder[i],
			"only the holder columns may differ between projections (index %d)", i)
	}
}

// A /v1 projection must not name the columns migrations 000017 and 000019 add,
// so the statement parses against a database that predates them.
func TestAccountColumns_V1ProjectionNamesNoHolderColumn(t *testing.T) {
	query, _, err := squirrel.Select(accountColumns(mmodel.HolderOffV1.ProjectsHolder())...).
		From("account").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	require.NoError(t, err)

	assert.NotContains(t, query, ", holder_id")
	assert.NotContains(t, query, ", holder_check_skipped")
	assert.Contains(t, query, "NULL::uuid AS holder_id")
	assert.Contains(t, query, "FALSE AS holder_check_skipped")
}

func TestAccountColumns_V2ProjectionNamesHolderColumns(t *testing.T) {
	query, _, err := squirrel.Select(accountColumns(mmodel.HolderOnV2.ProjectsHolder())...).
		From("account").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	require.NoError(t, err)

	assert.Contains(t, query, "holder_id")
	assert.Contains(t, query, "holder_check_skipped")
	assert.NotContains(t, query, "NULL::uuid")
}

func TestHolderPolicy_ProjectsHolder(t *testing.T) {
	assert.False(t, mmodel.HolderOffV1.ProjectsHolder())
	assert.True(t, mmodel.HolderOnV2.ProjectsHolder())

	var zero mmodel.HolderPolicy
	assert.False(t, zero.ProjectsHolder(), "the zero value must be the /v1 side")
}

// buildCreateSQL mirrors the column/value assembly of Create so the INSERT shape
// can be asserted without a database.
func buildCreateSQL(t *testing.T, acc *mmodel.Account) string {
	t.Helper()

	record := &AccountPostgreSQLModel{}
	record.FromEntity(acc)

	columns := []string{
		"id", "name", "parent_account_id", "entity_id", "asset_code",
		"organization_id", "ledger_id", "portfolio_id", "segment_id", "status",
		"status_description", "alias", "type", "created_at", "updated_at",
		"deleted_at", "blocked",
	}
	values := []any{
		record.ID, record.Name, record.ParentAccountID, record.EntityID, record.AssetCode,
		record.OrganizationID, record.LedgerID, record.PortfolioID, record.SegmentID, record.Status,
		record.StatusDescription, record.Alias, record.Type, record.CreatedAt, record.UpdatedAt,
		record.DeletedAt, record.Blocked,
	}

	if record.HolderID != nil || record.HolderCheckSkipped {
		columns = append(columns, holderIDColumn, holderCheckSkippedColumn)
		values = append(values, record.HolderID, record.HolderCheckSkipped)
	}

	query, _, err := squirrel.Insert("account").
		Columns(columns...).
		Values(values...).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	require.NoError(t, err)

	return query
}

func newAccountFixture() *mmodel.Account {
	created := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	return &mmodel.Account{
		ID:             "0198f0a0-0000-7000-8000-000000000001",
		Name:           "Test Account",
		AssetCode:      "USD",
		OrganizationID: "0198f0a0-0000-7000-8000-000000000002",
		LedgerID:       "0198f0a0-0000-7000-8000-000000000003",
		Type:           "deposit",
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      created,
		UpdatedAt:      created,
	}
}

// An account carrying no holder writes what the columns default to, so omitting
// them persists the identical row and keeps the statement parseable pre-000017.
func TestCreate_OmitsHolderColumnsWhenAccountCarriesNoHolder(t *testing.T) {
	query := buildCreateSQL(t, newAccountFixture())

	assert.NotContains(t, query, "holder_id")
	assert.NotContains(t, query, "holder_check_skipped")
	assert.Equal(t, 17, countPlaceholders(query))
}

func TestCreate_NamesHolderColumnsWhenHolderIsLinked(t *testing.T) {
	acc := newAccountFixture()
	holderID := "0198f0a0-0000-7000-8000-000000000004"
	acc.HolderID = &holderID

	query := buildCreateSQL(t, acc)

	assert.Contains(t, query, "holder_id")
	assert.Contains(t, query, "holder_check_skipped")
	assert.Equal(t, 19, countPlaceholders(query))
}

// An honored skip is a durable audit fact with no holder id, so it alone has to
// carry the columns into the statement.
func TestCreate_NamesHolderColumnsWhenOnlySkipIsRecorded(t *testing.T) {
	acc := newAccountFixture()
	acc.HolderCheckSkipped = true

	query := buildCreateSQL(t, acc)

	assert.Contains(t, query, "holder_check_skipped")
	assert.Equal(t, 19, countPlaceholders(query))
}

func countPlaceholders(query string) int {
	n := 0

	for i := 1; ; i++ {
		if !containsPlaceholder(query, i) {
			return n
		}

		n = i
	}
}

func containsPlaceholder(query string, n int) bool {
	needle := "$" + itoa(n)

	for i := 0; i+len(needle) <= len(query); i++ {
		if query[i:i+len(needle)] != needle {
			continue
		}

		// Reject a prefix match such as $1 inside $12.
		if i+len(needle) < len(query) && query[i+len(needle)] >= '0' && query[i+len(needle)] <= '9' {
			continue
		}

		return true
	}

	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var buf []byte

	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}

	return string(buf)
}
