// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// The validation trail is append-only, so these tests own a time window no
// other test writes into and read only inside it.
var schemeTestWindowStart = time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)

// schemeTestValidation builds an ALLOW validation on the given scheme, created
// minuteOffset minutes into the scheme test window.
func schemeTestValidation(seed int64, scheme model.TransactionType, minuteOffset int) *model.TransactionValidation {
	createdAt := schemeTestWindowStart.Add(time.Duration(minuteOffset) * time.Minute)

	return &model.TransactionValidation{
		ID:                   testutil.MustDeterministicUUID(seed),
		RequestID:            testutil.MustDeterministicUUID(seed + 1),
		TransactionType:      scheme,
		Scheme:               scheme,
		Amount:               decimal.RequireFromString("10.00"),
		Asset:                "BRL",
		TransactionTimestamp: createdAt,
		Account: model.AccountContext{
			ID:     testutil.MustDeterministicUUID(seed + 2),
			Type:   "checking",
			Status: "active",
		},
		EvaluationResult: model.EvaluationResult{
			Decision:         model.DecisionAllow,
			Reason:           "no rule matched",
			MatchedRuleIDs:   []uuid.UUID{},
			EvaluatedRuleIDs: []uuid.UUID{},
		},
		LimitUsageDetails: []model.LimitUsageDetail{},
		ProcessingTimeMs:  1,
		CreatedAt:         createdAt,
	}
}

// storedSchemeColumns reads the two raw scheme columns of a validation row.
func storedSchemeColumns(t *testing.T, db *sql.DB, id uuid.UUID) (sql.NullString, sql.NullString) {
	t.Helper()

	var scheme, transactionType sql.NullString

	require.NoError(t, db.QueryRow(
		`SELECT scheme, transaction_type::text FROM transaction_validations WHERE id = $1`, id,
	).
		Scan(&scheme, &transactionType))

	return scheme, transactionType
}

func TestIntegration_TransactionValidationRepo_Scheme(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := NewTransactionValidationRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	ctx := context.Background()

	t.Run("a free-form scheme persists with the enum column NULL", func(t *testing.T) {
		tv := schemeTestValidation(9_028_100, "BOLETO", 1)
		require.NoError(t, repo.Insert(ctx, tv))

		scheme, transactionType := storedSchemeColumns(t, db, tv.ID)
		assert.Equal(t, sql.NullString{String: "BOLETO", Valid: true}, scheme)
		assert.False(t, transactionType.Valid, "BOLETO is outside the enum, so the enum column stays NULL")

		got, err := repo.GetByID(ctx, tv.ID)
		require.NoError(t, err)
		assert.Equal(t, model.TransactionType("BOLETO"), got.TransactionType)
		assert.Equal(t, model.TransactionType("BOLETO"), got.Scheme)

		byRequest, err := repo.FindByRequestID(ctx, tv.RequestID)
		require.NoError(t, err)
		assert.Equal(t, model.TransactionType("BOLETO"), byRequest.TransactionType)
		assert.Equal(t, model.TransactionType("BOLETO"), byRequest.Scheme)
	})

	t.Run("an enum scheme writes both columns", func(t *testing.T) {
		tv := schemeTestValidation(9_028_200, model.TransactionTypePix, 2)
		require.NoError(t, repo.InsertWithTx(ctx, db, tv))

		scheme, transactionType := storedSchemeColumns(t, db, tv.ID)
		assert.Equal(t, sql.NullString{String: "PIX", Valid: true}, scheme)
		assert.Equal(t, sql.NullString{String: "PIX", Valid: true}, transactionType)

		got, err := repo.GetByID(ctx, tv.ID)
		require.NoError(t, err)
		assert.Equal(t, model.TransactionTypePix, got.TransactionType)
		assert.Equal(t, model.TransactionTypePix, got.Scheme)
	})

	t.Run("a row with only the enum column reads back its scheme", func(t *testing.T) {
		id := testutil.MustDeterministicUUID(9_028_300)
		createdAt := schemeTestWindowStart.Add(3 * time.Minute)

		_, err := db.Exec(`
			INSERT INTO transaction_validations
				(id, request_id, transaction_type, amount, asset, transaction_timestamp,
				 account, decision, reason, matched_rule_ids, evaluated_rule_ids,
				 processing_time_ms, created_at)
			VALUES ($1,$2,'WIRE','10.00','BRL',$3,$4,'ALLOW','no rule matched','{}','{}',1,$3)`,
			id, testutil.MustDeterministicUUID(9_028_301), createdAt,
			`{"accountId":"11111111-1111-1111-1111-111111111111","type":"checking"}`)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, model.TransactionTypeWire, got.TransactionType)
		assert.Equal(t, model.TransactionTypeWire, got.Scheme)
	})

	t.Run("list filters by the effective scheme", func(t *testing.T) {
		filterWindow := func(scheme model.TransactionType) *model.TransactionValidationFilters {
			return &model.TransactionValidationFilters{
				StartDate:       schemeTestWindowStart,
				EndDate:         schemeTestWindowStart.Add(time.Hour),
				TransactionType: &scheme,
			}
		}

		boleto, err := repo.List(ctx, filterWindow("BOLETO"))
		require.NoError(t, err)
		require.Len(t, boleto.TransactionValidations, 1)
		assert.Equal(t, testutil.MustDeterministicUUID(9_028_100), boleto.TransactionValidations[0].ID)
		assert.Equal(t, model.TransactionType("BOLETO"), boleto.TransactionValidations[0].Scheme)

		wire, err := repo.List(ctx, filterWindow(model.TransactionTypeWire))
		require.NoError(t, err)
		require.Len(t, wire.TransactionValidations, 1, "a row written with only the enum column still matches")
		assert.Equal(t, testutil.MustDeterministicUUID(9_028_300), wire.TransactionValidations[0].ID)

		count, err := repo.Count(ctx, filterWindow("BOLETO"))
		require.NoError(t, err)
		assert.EqualValues(t, 1, count)
	})
}
