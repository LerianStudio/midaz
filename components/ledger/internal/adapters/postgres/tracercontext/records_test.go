// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontext

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

var (
	orgID     = uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	ledgerID  = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	accountID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
	assetID   = uuid.MustParse("550e8400-e29b-41d4-a716-446655440004")
)

func testBounds() tracercontract.Limits {
	return tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
}

func TestReadOfficialRecords(t *testing.T) {
	for _, scenario := range []string{"success", "missing account", "duplicate asset", "missing asset", "query failure", "commit failure"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			resolver := dbresolver.New(dbresolver.WithPrimaryDBs(db))
			ctx := tmcore.ContextWithPG(t.Context(), resolver)
			repo, err := NewRepository(nil, testBounds(), true)
			require.NoError(t, err)
			mock.ExpectBegin()
			accounts := sqlmock.NewRows([]string{"id", "asset_code", "type", "status", "blocked"})
			if scenario != "missing account" {
				accounts.AddRow(accountID, "BTC", "deposit", "ACTIVE", false)
			}
			query := mock.ExpectQuery("SELECT").WillReturnRows(accounts)
			if scenario == "query failure" {
				query.WillReturnError(errors.New("database unavailable"))
			}
			if scenario != "missing account" && scenario != "query failure" {
				assets := sqlmock.NewRows([]string{"id", "code"})
				if scenario != "missing asset" {
					assets.AddRow(assetID, "BTC")
				}
				if scenario == "duplicate asset" {
					assets.AddRow(orgID, "BTC")
				}
				mock.ExpectQuery("SELECT").WillReturnRows(assets)
			}
			if scenario == "success" {
				mock.ExpectCommit()
			} else if scenario == "commit failure" {
				mock.ExpectCommit().WillReturnError(errors.New("commit unknown"))
			} else {
				mock.ExpectRollback()
			}
			accountsResult, assetsResult, err := repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID}, []string{"BTC"})
			if scenario == "success" {
				require.NoError(t, err)
				require.Len(t, accountsResult, 1)
				require.Len(t, assetsResult, 1)
				require.Equal(t, assetID.String(), assetsResult[0].ID)
				require.NotNil(t, accountsResult[0].Blocked)
				require.False(t, *accountsResult[0].Blocked)
			} else {
				require.Error(t, err)
				require.Nil(t, accountsResult)
				require.Nil(t, assetsResult)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestReadOfficialRecordsGuards(t *testing.T) {
	repo, err := NewRepository(nil, testBounds(), true)
	require.NoError(t, err)
	for _, ids := range [][]uuid.UUID{nil, {uuid.Nil}, {accountID, accountID}} {
		_, _, err := repo.Read(t.Context(), orgID, ledgerID, ids, nil)
		require.Error(t, err)
	}
	_, _, err = repo.Read(t.Context(), orgID, ledgerID, []uuid.UUID{accountID}, nil)
	require.Error(t, err) // A multi-tenant reader never falls back to a static pool.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID}, nil)
	require.ErrorIs(t, err, context.Canceled)
}
