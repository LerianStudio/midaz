// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"database/sql"
	"testing"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// fixedClosedAt is the closing instant the mapping tests read and write. A fixed
// time keeps the round trip comparable without reading the clock.
var fixedClosedAt = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// A NULL closed_at is an open account: the entity must carry no instant at all,
// not a zero time.
func TestAccountModel_ToEntity_NullClosedAtIsOpen(t *testing.T) {
	model := &AccountPostgreSQLModel{ID: "acc-1", Status: "ACTIVE"}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Nil(t, entity.ClosedAt, "an account with NULL closed_at must be open")
}

// A closed_at value reaches the entity as the same instant.
func TestAccountModel_ToEntity_ClosedAtIsProjected(t *testing.T) {
	model := &AccountPostgreSQLModel{
		ID:       "acc-1",
		Status:   "ACTIVE",
		ClosedAt: sql.NullTime{Time: fixedClosedAt, Valid: true},
	}

	entity := model.ToEntity()

	require.NotNil(t, entity.ClosedAt)
	assert.True(t, fixedClosedAt.Equal(*entity.ClosedAt), "the closing instant must survive the projection")
}

// FromEntity carries the instant for round-trip fidelity; no statement this
// package builds writes the column.
func TestAccountModel_FromEntity_ClosedAtRoundTrips(t *testing.T) {
	tests := []struct {
		name     string
		closedAt *time.Time
		wantTime bool
	}{
		{name: "open account carries no instant", closedAt: nil, wantTime: false},
		{name: "closed account carries its instant", closedAt: &fixedClosedAt, wantTime: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			model := &AccountPostgreSQLModel{}
			model.FromEntity(&mmodel.Account{ID: "acc-1", ClosedAt: tc.closedAt})

			assert.Equal(t, tc.wantTime, model.ClosedAt.Valid)

			if tc.wantTime {
				assert.True(t, fixedClosedAt.Equal(model.ClosedAt.Time))
			}
		})
	}
}

// closedAt is version-independent: both the /v1 and the /v2 projections name the
// column, so a closed account reports the same instant on either contract. Only
// the holder columns differ between the two.
func TestAccountColumns_BothProjectionsNameClosedAt(t *testing.T) {
	for _, policy := range []mmodel.HolderPolicy{mmodel.HolderOffV1, mmodel.HolderOnV2} {
		query, _, err := squirrel.Select(accountColumns(policy.ProjectsHolder())...).
			From("account").
			PlaceholderFormat(squirrel.Dollar).
			ToSql()
		require.NoError(t, err)

		assert.Contains(t, query, "closed_at", "every account projection must read closed_at")
	}
}

// buildUpdateSQL runs the real SET assembly of Update and returns the statement,
// so the assertion reads what the repository would send rather than a copy of it.
func buildUpdateSQL(t *testing.T, acc *mmodel.Account) string {
	t.Helper()

	record := &AccountPostgreSQLModel{}
	record.FromEntity(acc)

	query, _, err := applyAccountUpdateFields(squirrel.Update("account"), acc, record).
		Set("updated_at", fixedClosedAt).
		Where(squirrel.Eq{"id": acc.ID}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	require.NoError(t, err)

	return query
}

// The generic update never writes closed_at, whatever the entity carries. An
// account that already holds a closing instant round-trips through FromEntity
// with it set, so a SET list derived from the record would silently rewrite the
// column on every PATCH.
func TestUpdate_NeverWritesClosedAt(t *testing.T) {
	tests := []struct {
		name string
		acc  *mmodel.Account
	}{
		{
			name: "entity carries a closing instant",
			acc: &mmodel.Account{
				ID:       "acc-1",
				Name:     "Renamed",
				ClosedAt: &fixedClosedAt,
			},
		},
		{
			name: "null fields name the closing instant in both spellings",
			acc: &mmodel.Account{
				ID:         "acc-1",
				Name:       "Renamed",
				ClosedAt:   &fixedClosedAt,
				NullFields: []string{"closedAt", "closed_at"},
			},
		},
		{
			name: "null fields mix the closing instant with an allowed one",
			acc: &mmodel.Account{
				ID:         "acc-1",
				Name:       "Renamed",
				ClosedAt:   &fixedClosedAt,
				NullFields: []string{"closedAt", "segmentId"},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			query := buildUpdateSQL(t, tc.acc)

			// Guard against a vacuous pass: the statement must still be a real update.
			require.Contains(t, query, "name = ", "the update must still write the allowed fields")

			assert.NotContains(t, query, "closed_at", "a generic update must never name closed_at")
		})
	}
}

// applyNullableFields owns the merge-patch null semantics, and its field set is
// closed: a closedAt entry must clear nothing.
func TestApplyNullableFields_IgnoresClosedAt(t *testing.T) {
	acc := &mmodel.Account{ID: "acc-1", NullFields: []string{"closedAt", "closed_at", "segmentId"}}

	record := &AccountPostgreSQLModel{}
	record.FromEntity(acc)

	query, _, err := applyNullableFields(squirrel.Update("account"), acc, record).
		Where(squirrel.Eq{"id": acc.ID}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	require.NoError(t, err)

	require.Contains(t, query, "segment_id = ", "the allowed null field must still be cleared")
	assert.NotContains(t, query, "closed_at")
}
