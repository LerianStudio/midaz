// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestReservationOwnerUsesPrimaryAndProducer(t *testing.T) {
	for _, scenario := range []string{"found", "missing", "corrupt owner"} {
		t.Run(scenario, func(t *testing.T) {
			primary, p, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = primary.Close() })
			replica, r, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = replica.Close() })
			resolver := dbresolver.New(dbresolver.WithPrimaryDBs(primary), dbresolver.WithReplicaDBs(replica))
			conn := mocks.NewMockConnection(gomock.NewController(t))
			conn.EXPECT().GetDB(gomock.Any()).Return(resolver, nil)
			repo, err := NewReserveDecisionRepository(conn, 10, 100)
			require.NoError(t, err)
			reservation := testutil.MustDeterministicUUID(88921)
			transaction := testutil.MustDeterministicUUID(88922)
			evaluation := testutil.MustDeterministicUUID(88923)
			rows := sqlmock.NewRows([]string{"integration_id", "transaction_id", "evaluation_id"})
			integration := "producer"
			if scenario == "corrupt owner" {
				integration = "other"
			}
			if scenario != "missing" {
				rows.AddRow(integration, transaction.String(), evaluation.String())
			}
			p.ExpectQuery(`SELECT .* FROM usage_reservations .* JOIN reserve_decisions .* WHERE .*`).WithArgs(reservation, "producer").WillReturnRows(rows)
			owner, err := repo.GetReservationOwner(t.Context(), "producer", reservation)
			switch scenario {
			case "found":
				require.NoError(t, err)
				require.Equal(t, transaction, owner.Operation.TransactionID)
				require.Equal(t, evaluation, owner.EvaluationID)
				require.Equal(t, reservation, owner.ReservationID)
			case "missing":
				require.NoError(t, err)
				require.Nil(t, owner)
			case "corrupt owner":
				require.ErrorIs(t, err, constant.ErrInternalServer)
				require.Nil(t, owner)
			}
			require.NoError(t, p.ExpectationsWereMet())
			require.NoError(t, r.ExpectationsWereMet())
		})
	}
}
