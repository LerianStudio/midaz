// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontext

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

var (
	orgID     = uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	ledgerID  = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	accountID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
)

func testBounds() tracercontract.Limits {
	return tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
}

func TestReadOfficialRecords(t *testing.T) {
	for _, scenario := range []string{"success", "missing account", "duplicate account", "invalid text", "query failure", "iteration failure", "commit failure", "begin failure"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			resolver := dbresolver.New(dbresolver.WithPrimaryDBs(db))
			ctx := tmcore.ContextWithPG(t.Context(), resolver)
			repo, err := NewRepository(nil, testBounds(), true)
			require.NoError(t, err)
			if scenario == "begin failure" {
				mock.ExpectBegin().WillReturnError(errors.New("connection refused"))
				_, err := repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID})
				require.ErrorIs(t, err, traceradapter.ErrOfficialRecordsUnavailable)
				require.NoError(t, mock.ExpectationsWereMet())

				return
			}
			mock.ExpectBegin()
			accounts := sqlmock.NewRows([]string{"id", "asset_code", "type", "status", "blocked"})
			switch scenario {
			case "missing account":
			case "invalid text":
				accounts.AddRow(accountID, " BTC", "deposit", "ACTIVE", false)
			default:
				accounts.AddRow(accountID, "BTC", "deposit", "ACTIVE", false)
			}
			if scenario == "duplicate account" {
				accounts.AddRow(accountID, "BTC", "deposit", "ACTIVE", false)
			}
			if scenario == "iteration failure" {
				accounts.RowError(0, errors.New("connection reset"))
			}
			query := mock.ExpectQuery("SELECT").WillReturnRows(accounts)
			if scenario == "query failure" {
				query.WillReturnError(errors.New("database unavailable"))
			}
			if scenario == "success" {
				mock.ExpectCommit()
			} else if scenario == "commit failure" {
				mock.ExpectCommit().WillReturnError(errors.New("commit unknown"))
			} else {
				mock.ExpectRollback()
			}
			accountsResult, err := repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID})
			if scenario == "success" {
				require.NoError(t, err)
				require.Len(t, accountsResult, 1)
				require.Equal(t, "BTC", accountsResult[0].AssetCode)
				require.NotNil(t, accountsResult[0].Blocked)
				require.False(t, *accountsResult[0].Blocked)
			} else {
				require.Error(t, err)
				require.Nil(t, accountsResult)
			}
			// Only a store failure is an availability failure; records that were
			// read and found missing, duplicated or invalid stay deterministic.
			switch scenario {
			case "query failure", "iteration failure", "commit failure":
				require.ErrorIs(t, err, traceradapter.ErrOfficialRecordsUnavailable)
			case "missing account", "duplicate account", "invalid text":
				require.ErrorIs(t, err, constant.ErrTracerFactsUnavailable)
				require.NotErrorIs(t, err, traceradapter.ErrOfficialRecordsUnavailable)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestReadOfficialRecordsGuards(t *testing.T) {
	repo, err := NewRepository(nil, testBounds(), true)
	require.NoError(t, err)
	for _, ids := range [][]uuid.UUID{nil, {uuid.Nil}, {accountID, accountID}} {
		_, err := repo.Read(t.Context(), orgID, ledgerID, ids)
		require.Error(t, err)
	}
	_, err = repo.Read(t.Context(), orgID, ledgerID, []uuid.UUID{accountID})
	require.Error(t, err) // A multi-tenant reader never falls back to a static pool.
	require.NotErrorIs(t, err, traceradapter.ErrOfficialRecordsUnavailable)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID})
	require.ErrorIs(t, err, context.Canceled)
}

func TestReadOfficialRecordsBoundsAssetCodeByCharacters(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// code is what the bounded query returns; nil is the NULL it yields for
		// a code longer than utils.MaxAssetCodeLength characters.
		code    any
		wantErr func(t *testing.T, err error)
	}{
		{name: "legacy lowercase stored code", code: "usdt"},
		{name: "hundred multi-byte characters", code: strings.Repeat("É", utils.MaxAssetCodeLength)},
		{name: "over hundred characters withheld as NULL", code: nil, wantErr: func(t *testing.T, err error) {
			require.ErrorContains(t, err, "scan official account")
			require.ErrorContains(t, err, "converting NULL to string is unsupported")
		}},
		{name: "surrounding space", code: " USD", wantErr: func(t *testing.T, err error) {
			require.ErrorIs(t, err, constant.ErrTracerFactsUnavailable)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			ctx := tmcore.ContextWithPG(t.Context(), dbresolver.New(dbresolver.WithPrimaryDBs(db)))
			// The generic text bound is smaller than a 100-character multi-byte code.
			bounds := testBounds()
			bounds.MaxTextBytes = 64
			repo, err := NewRepository(nil, bounds, true)
			require.NoError(t, err)
			mock.ExpectBegin()
			mock.ExpectQuery("char_length\\(asset_code\\)").
				WithArgs(orgID, ledgerID, sqlmock.AnyArg(), bounds.MaxTextBytes, 1, utils.MaxAssetCodeLength).
				WillReturnRows(sqlmock.NewRows([]string{"id", "asset_code", "type", "status", "blocked"}).
					AddRow(accountID, tc.code, "deposit", "ACTIVE", false))
			if tc.wantErr == nil {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}

			accounts, err := repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID})
			if tc.wantErr == nil {
				require.NoError(t, err)
				require.Equal(t, tc.code, accounts[0].AssetCode)
			} else {
				tc.wantErr(t, err)
				require.NotErrorIs(t, err, traceradapter.ErrOfficialRecordsUnavailable, "a record read and found invalid is not a store failure")
				require.Nil(t, accounts)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
