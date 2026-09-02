// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountexception

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// fixedTestTime is a deterministic UTC instant used by the mapping tests so the
// timestamps stay reproducible instead of relying on time.Now().
var fixedTestTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// ptr returns a pointer to v. Used to build the optional entity fields
// (balanceKey, effectiveAt, expiresAt, deletedAt) inline in table cases.
func ptr[T any](v T) *T {
	return &v
}

func TestAccountExceptionPostgreSQLModel_ToEntity(t *testing.T) {
	t.Parallel()

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()
	id := uuid.New().String()

	effectiveAt := fixedTestTime
	expiresAt := fixedTestTime.Add(24 * time.Hour)
	deletedAt := fixedTestTime.Add(48 * time.Hour)

	tests := []struct {
		name   string
		model  *AccountExceptionPostgreSQLModel
		assert func(t *testing.T, entity *mmodel.AccountException)
	}{
		{
			name: "every_field_populated",
			model: &AccountExceptionPostgreSQLModel{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []byte(`["PIX_IN","TED_OUT"]`),
				BalanceKey:           sql.NullString{String: "asset-freeze", Valid: true},
				Context:              "Judicial order 12345/2026",
				EffectiveAt:          sql.NullTime{Time: effectiveAt, Valid: true},
				ExpiresAt:            sql.NullTime{Time: expiresAt, Valid: true},
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
				DeletedAt:            sql.NullTime{Time: deletedAt, Valid: true},
			},
			assert: func(t *testing.T, entity *mmodel.AccountException) {
				require.NotNil(t, entity)
				assert.Equal(t, id, entity.ID)
				assert.Equal(t, orgID, entity.OrganizationID)
				assert.Equal(t, ledgerID, entity.LedgerID)
				assert.Equal(t, accountID, entity.AccountID)
				assert.Equal(t, []string{"PIX_IN", "TED_OUT"}, entity.OperationalTypeCodes)
				require.NotNil(t, entity.BalanceKey)
				assert.Equal(t, "asset-freeze", *entity.BalanceKey)
				assert.Equal(t, "Judicial order 12345/2026", entity.Context)
				require.NotNil(t, entity.EffectiveAt)
				assert.Equal(t, effectiveAt, *entity.EffectiveAt)
				require.NotNil(t, entity.ExpiresAt)
				assert.Equal(t, expiresAt, *entity.ExpiresAt)
				assert.Equal(t, fixedTestTime, entity.CreatedAt)
				assert.Equal(t, fixedTestTime, entity.UpdatedAt)
				require.NotNil(t, entity.DeletedAt)
				assert.Equal(t, deletedAt, *entity.DeletedAt)
			},
		},
		{
			name: "every_nullable_column_null",
			model: &AccountExceptionPostgreSQLModel{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []byte(`["PIX_IN"]`),
				BalanceKey:           sql.NullString{},
				Context:              "no window, every balance",
				EffectiveAt:          sql.NullTime{},
				ExpiresAt:            sql.NullTime{},
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
				DeletedAt:            sql.NullTime{},
			},
			assert: func(t *testing.T, entity *mmodel.AccountException) {
				require.NotNil(t, entity)
				assert.Equal(t, []string{"PIX_IN"}, entity.OperationalTypeCodes)
				assert.Nil(t, entity.BalanceKey, "a NULL balance_key means every balance, so the entity pointer stays nil")
				assert.Nil(t, entity.EffectiveAt, "a NULL effective_at means effective immediately")
				assert.Nil(t, entity.ExpiresAt, "a NULL expires_at means indeterminate validity")
				assert.Nil(t, entity.DeletedAt, "a NULL deleted_at means the row is live")
			},
		},
		{
			name: "non_zero_time_with_invalid_null_flag_stays_nil",
			model: &AccountExceptionPostgreSQLModel{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []byte(`["PIX_IN"]`),
				Context:              "invalid null flags",
				EffectiveAt:          sql.NullTime{Time: fixedTestTime, Valid: false},
				ExpiresAt:            sql.NullTime{Time: fixedTestTime, Valid: false},
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
				DeletedAt:            sql.NullTime{Time: fixedTestTime, Valid: false},
			},
			assert: func(t *testing.T, entity *mmodel.AccountException) {
				require.NotNil(t, entity)
				assert.Nil(t, entity.EffectiveAt, "Valid=false must win over a non-zero Time")
				assert.Nil(t, entity.ExpiresAt, "Valid=false must win over a non-zero Time")
				assert.Nil(t, entity.DeletedAt, "Valid=false must win over a non-zero Time")
			},
		},
		{
			name: "empty_balance_key_string_is_preserved_when_valid",
			model: &AccountExceptionPostgreSQLModel{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []byte(`["PIX_IN"]`),
				BalanceKey:           sql.NullString{String: "", Valid: true},
				Context:              "empty but non-null balance key",
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
			},
			assert: func(t *testing.T, entity *mmodel.AccountException) {
				require.NotNil(t, entity)
				require.NotNil(t, entity.BalanceKey, "a non-NULL empty string is distinct from NULL and must survive the mapping")
				assert.Equal(t, "", *entity.BalanceKey)
			},
		},
		{
			name: "empty_jsonb_array_yields_empty_slice",
			model: &AccountExceptionPostgreSQLModel{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []byte(`[]`),
				Context:              "empty codes",
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
			},
			assert: func(t *testing.T, entity *mmodel.AccountException) {
				require.NotNil(t, entity)
				assert.Empty(t, entity.OperationalTypeCodes)
			},
		},
		{
			name: "nil_jsonb_yields_nil_slice_without_panicking",
			model: &AccountExceptionPostgreSQLModel{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: nil,
				Context:              "nil codes",
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
			},
			assert: func(t *testing.T, entity *mmodel.AccountException) {
				require.NotNil(t, entity)
				assert.Nil(t, entity.OperationalTypeCodes)
			},
		},
		{
			name: "malformed_jsonb_yields_nil_slice_without_panicking",
			model: &AccountExceptionPostgreSQLModel{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []byte(`{"not":"an array"`),
				Context:              "malformed codes",
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
			},
			assert: func(t *testing.T, entity *mmodel.AccountException) {
				require.NotNil(t, entity)
				assert.Nil(t, entity.OperationalTypeCodes, "unparseable JSONB must degrade to nil, never panic")
			},
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.assert(t, tc.model.ToEntity())
		})
	}
}

func TestAccountExceptionPostgreSQLModel_ToEntity_NilReceiver(t *testing.T) {
	t.Parallel()

	var model *AccountExceptionPostgreSQLModel

	assert.Nil(t, model.ToEntity(), "a nil receiver must map to a nil entity instead of panicking")
}

func TestAccountExceptionPostgreSQLModel_FromEntity(t *testing.T) {
	t.Parallel()

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()
	id := uuid.New().String()

	effectiveAt := fixedTestTime
	expiresAt := fixedTestTime.Add(24 * time.Hour)
	deletedAt := fixedTestTime.Add(48 * time.Hour)

	tests := []struct {
		name   string
		entity *mmodel.AccountException
		assert func(t *testing.T, model *AccountExceptionPostgreSQLModel)
	}{
		{
			name: "every_field_populated",
			entity: &mmodel.AccountException{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []string{"PIX_IN", "TED_OUT"},
				BalanceKey:           ptr("asset-freeze"),
				Context:              "Judicial order 12345/2026",
				EffectiveAt:          &effectiveAt,
				ExpiresAt:            &expiresAt,
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
				DeletedAt:            &deletedAt,
			},
			assert: func(t *testing.T, model *AccountExceptionPostgreSQLModel) {
				assert.Equal(t, id, model.ID)
				assert.Equal(t, orgID, model.OrganizationID)
				assert.Equal(t, ledgerID, model.LedgerID)
				assert.Equal(t, accountID, model.AccountID)
				assert.JSONEq(t, `["PIX_IN","TED_OUT"]`, string(model.OperationalTypeCodes))
				assert.Equal(t, sql.NullString{String: "asset-freeze", Valid: true}, model.BalanceKey)
				assert.Equal(t, "Judicial order 12345/2026", model.Context)
				assert.Equal(t, sql.NullTime{Time: effectiveAt, Valid: true}, model.EffectiveAt)
				assert.Equal(t, sql.NullTime{Time: expiresAt, Valid: true}, model.ExpiresAt)
				assert.Equal(t, fixedTestTime, model.CreatedAt)
				assert.Equal(t, fixedTestTime, model.UpdatedAt)
				assert.Equal(t, sql.NullTime{Time: deletedAt, Valid: true}, model.DeletedAt)
			},
		},
		{
			name: "every_optional_field_nil",
			entity: &mmodel.AccountException{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []string{"PIX_IN"},
				BalanceKey:           nil,
				Context:              "no window, every balance",
				EffectiveAt:          nil,
				ExpiresAt:            nil,
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
				DeletedAt:            nil,
			},
			assert: func(t *testing.T, model *AccountExceptionPostgreSQLModel) {
				assert.JSONEq(t, `["PIX_IN"]`, string(model.OperationalTypeCodes))
				assert.False(t, model.BalanceKey.Valid, "a nil balanceKey must persist as SQL NULL")
				assert.False(t, model.EffectiveAt.Valid, "a nil effectiveAt must persist as SQL NULL")
				assert.False(t, model.ExpiresAt.Valid, "a nil expiresAt must persist as SQL NULL")
				assert.False(t, model.DeletedAt.Valid, "a nil deletedAt must persist as SQL NULL")
			},
		},
		{
			name: "empty_balance_key_pointer_persists_as_non_null",
			entity: &mmodel.AccountException{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []string{"PIX_IN"},
				BalanceKey:           ptr(""),
				Context:              "cleared balance key",
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
			},
			assert: func(t *testing.T, model *AccountExceptionPostgreSQLModel) {
				assert.Equal(t, sql.NullString{String: "", Valid: true}, model.BalanceKey,
					"a non-nil pointer to the empty string is a value, not a NULL")
			},
		},
		{
			name: "nil_codes_persist_as_an_empty_json_array",
			entity: &mmodel.AccountException{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: nil,
				Context:              "nil codes",
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
			},
			assert: func(t *testing.T, model *AccountExceptionPostgreSQLModel) {
				assert.JSONEq(t, `[]`, string(model.OperationalTypeCodes),
					"operational_type_codes is JSONB NOT NULL, so a nil slice must become [] and never a JSON null")
			},
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			model := &AccountExceptionPostgreSQLModel{}
			model.FromEntity(tc.entity)

			tc.assert(t, model)
		})
	}
}

func TestAccountExceptionPostgreSQLModel_FromEntity_NilEntity(t *testing.T) {
	t.Parallel()

	model := &AccountExceptionPostgreSQLModel{}

	assert.NotPanics(t, func() { model.FromEntity(nil) },
		"a nil entity must be a no-op instead of panicking")
	assert.Equal(t, &AccountExceptionPostgreSQLModel{}, model, "a nil entity must leave the model untouched")
}

func TestAccountExceptionPostgreSQLModel_RoundTrip(t *testing.T) {
	t.Parallel()

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()
	id := uuid.New().String()

	effectiveAt := fixedTestTime
	expiresAt := fixedTestTime.Add(24 * time.Hour)
	deletedAt := fixedTestTime.Add(48 * time.Hour)

	tests := []struct {
		name   string
		entity *mmodel.AccountException
	}{
		{
			name: "populated_entity_survives_the_round_trip",
			entity: &mmodel.AccountException{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []string{"PIX_IN", "TED_OUT", "BOLETO_IN"},
				BalanceKey:           ptr("asset-freeze"),
				Context:              "Judicial order 12345/2026",
				EffectiveAt:          &effectiveAt,
				ExpiresAt:            &expiresAt,
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
				DeletedAt:            &deletedAt,
			},
		},
		{
			name: "sparse_entity_survives_the_round_trip",
			entity: &mmodel.AccountException{
				ID:                   id,
				OrganizationID:       orgID,
				LedgerID:             ledgerID,
				AccountID:            accountID,
				OperationalTypeCodes: []string{"PIX_IN"},
				Context:              "no window, every balance",
				CreatedAt:            fixedTestTime,
				UpdatedAt:            fixedTestTime,
			},
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			model := &AccountExceptionPostgreSQLModel{}
			model.FromEntity(tc.entity)

			// The JSONB column must hold a valid JSON array at rest, not a Go-side blob.
			var codes []string
			require.NoError(t, json.Unmarshal(model.OperationalTypeCodes, &codes),
				"operational_type_codes must be valid JSON in the database column")

			assert.Equal(t, tc.entity, model.ToEntity(), "FromEntity followed by ToEntity must be the identity")
		})
	}
}

func TestGetDB_RequiresTenantContext_WhenConfigured(t *testing.T) {
	t.Parallel()

	repo := NewAccountExceptionPostgreSQLRepository(nil, true)

	_, err := repo.getDB(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant postgres connection missing from context")
}

func TestGetDB_ReturnsErrorWhenNoConnectionConfigured(t *testing.T) {
	t.Parallel()

	repo := NewAccountExceptionPostgreSQLRepository(nil)

	_, err := repo.getDB(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "postgres connection not configured")
}

func TestNewAccountExceptionPostgreSQLRepository_UsesAccountExceptionTable(t *testing.T) {
	t.Parallel()

	repo := NewAccountExceptionPostgreSQLRepository(nil)

	assert.Equal(t, "account_exception", repo.tableName,
		"the table name must match migration 000022")
}
