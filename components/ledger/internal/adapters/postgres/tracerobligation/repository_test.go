// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracerobligation

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestBeginExecutionNeverRetriesUnknownCommit(t *testing.T) {
	for _, scenario := range []string{"success", "commit unknown", "write failed", "already dispatched"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			ctx := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), dbresolver.New(dbresolver.WithPrimaryDBs(db)), constant.ModuleTransaction)
			intent, cfg := obligationFixture(t)
			repo, err := NewRepository(nil, cfg, true, 10)
			require.NoError(t, err)
			fault := errors.New("outcome unknown")
			mock.ExpectBegin()
			write := mock.ExpectExec("UPDATE tracer_reservation_obligation SET state='EXECUTING'").WithArgs(intent.Key.OrganizationID, intent.Key.LedgerID, intent.Key.TransactionID, "tenant-a", intent.CreatedAt.Add(time.Millisecond))
			switch scenario {
			case "write failed":
				write.WillReturnError(fault)
				mock.ExpectRollback()
			case "already dispatched":
				write.WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectRollback()
			default:
				write.WillReturnResult(sqlmock.NewResult(0, 1))
				commit := mock.ExpectCommit()
				if scenario == "commit unknown" {
					commit.WillReturnError(fault)
				}
			}
			err = repo.BeginExecution(ctx, intent.Key, intent.CreatedAt.Add(time.Millisecond))
			switch scenario {
			case "success":
				require.NoError(t, err)
			case "already dispatched":
				require.ErrorIs(t, err, constant.ErrReserveOperationConflict)
			default:
				require.ErrorIs(t, err, fault)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
