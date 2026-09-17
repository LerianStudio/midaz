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
