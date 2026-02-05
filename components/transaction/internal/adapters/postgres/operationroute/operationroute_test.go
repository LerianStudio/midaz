// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operationroute

import (
	"database/sql"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationRoutePostgreSQLModel_ToEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()
		deletedAt := time.Now().Add(-24 * time.Hour)

		model := &OperationRoutePostgreSQLModel{
			ID:                 id,
			OrganizationID:     orgID,
			LedgerID:           ledgerID,
			Title:              "Cashin Route",
			Description:        "Route for cash-in operations",
			Code:               sql.NullString{String: "CASHIN-001", Valid: true},
			OperationType:      "source",
			AccountRuleType:    "alias",
			AccountRuleValidIf: "@cash_account",
			CreatedAt:          time.Now().Add(-48 * time.Hour),
			UpdatedAt:          time.Now().Add(-1 * time.Hour),
			DeletedAt:          sql.NullTime{Time: deletedAt, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Equal(t, model.Title, entity.Title)
		assert.Equal(t, model.Description, entity.Description)
		assert.Equal(t, "CASHIN-001", entity.Code)
		assert.Equal(t, model.OperationType, entity.OperationType)
		require.NotNil(t, entity.Account)
		assert.Equal(t, "alias", entity.Account.RuleType)
		assert.Equal(t, "@cash_account", entity.Account.ValidIf)
		assert.Equal(t, model.CreatedAt, entity.CreatedAt)
		assert.Equal(t, model.UpdatedAt, entity.UpdatedAt)
		require.NotNil(t, entity.DeletedAt)
		assert.Equal(t, deletedAt, *entity.DeletedAt)
	})

	t.Run("with_account_type_rule", func(t *testing.T) {
		// Tests the account_type rule which converts comma-separated values to array
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		model := &OperationRoutePostgreSQLModel{
			ID:                 id,
			OrganizationID:     orgID,
			LedgerID:           ledgerID,
			Title:              "Account Type Route",
			Description:        "Route with account type rule",
			OperationType:      "destination",
			AccountRuleType:    "account_type",
			AccountRuleValidIf: "deposit, savings, checking",
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		require.NotNil(t, entity.Account)
		assert.Equal(t, "account_type", entity.Account.RuleType)
		validIf, ok := entity.Account.ValidIf.([]string)
		require.True(t, ok, "ValidIf should be []string for account_type rule")
		assert.Len(t, validIf, 3)
		assert.Equal(t, "deposit", validIf[0])
		assert.Equal(t, "savings", validIf[1])
		assert.Equal(t, "checking", validIf[2])
	})

	t.Run("with_code_null", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		model := &OperationRoutePostgreSQLModel{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "No Code Route",
			Description:    "Route without code",
			Code:           sql.NullString{Valid: false},
			OperationType:  "source",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Empty(t, entity.Code, "Code should be empty when sql.NullString is invalid")
	})

	t.Run("with_no_account_rule", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		model := &OperationRoutePostgreSQLModel{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Simple Route",
			Description:    "Route without account rules",
			OperationType:  "source",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.Account, "Account should be nil when AccountRuleType and AccountRuleValidIf are empty")
	})

	t.Run("with_nil_model", func(t *testing.T) {
		var model *OperationRoutePostgreSQLModel

		entity := model.ToEntity()

		assert.Nil(t, entity, "ToEntity should return nil for nil model")
	})

	t.Run("with_deleted_at_valid_but_zero_time", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		model := &OperationRoutePostgreSQLModel{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Edge Case Route",
			OperationType:  "source",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      sql.NullTime{Time: time.Time{}, Valid: false},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.DeletedAt, "DeletedAt should be nil when Valid is false")
	})
}

func TestOperationRoutePostgreSQLModel_FromEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()
		deletedAt := time.Now().Add(-24 * time.Hour)

		entity := &mmodel.OperationRoute{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Cashin Route",
			Description:    "Route for cash-in operations",
			Code:           "CASHIN-001",
			OperationType:  "SOURCE",
			Account: &mmodel.AccountRule{
				RuleType: "alias",
				ValidIf:  "@cash_account",
			},
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
			DeletedAt: &deletedAt,
		}

		var model OperationRoutePostgreSQLModel
		model.FromEntity(entity)

		assert.Equal(t, entity.ID, model.ID)
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		assert.Equal(t, entity.Title, model.Title)
		assert.Equal(t, entity.Description, model.Description)
		assert.True(t, model.Code.Valid, "Code should be valid")
		assert.Equal(t, entity.Code, model.Code.String)
		assert.Equal(t, "source", model.OperationType, "OperationType should be converted to lowercase")
		assert.Equal(t, "alias", model.AccountRuleType)
		assert.Equal(t, "@cash_account", model.AccountRuleValidIf)
		assert.Equal(t, entity.CreatedAt, model.CreatedAt)
		assert.Equal(t, entity.UpdatedAt, model.UpdatedAt)
		assert.True(t, model.DeletedAt.Valid)
		assert.Equal(t, deletedAt, model.DeletedAt.Time)
	})

	t.Run("with_account_type_rule_string_slice", func(t *testing.T) {
		// Tests conversion of []string ValidIf for account_type rule
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		entity := &mmodel.OperationRoute{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Account Type Route",
			OperationType:  "destination",
			Account: &mmodel.AccountRule{
				RuleType: "account_type",
				ValidIf:  []string{"deposit", "savings", "checking"},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		var model OperationRoutePostgreSQLModel
		model.FromEntity(entity)

		assert.Equal(t, "account_type", model.AccountRuleType)
		assert.Equal(t, "deposit,savings,checking", model.AccountRuleValidIf)
	})

	t.Run("with_account_type_rule_any_slice", func(t *testing.T) {
		// Tests conversion of []any ValidIf for account_type rule
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		entity := &mmodel.OperationRoute{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Account Type Route",
			OperationType:  "destination",
			Account: &mmodel.AccountRule{
				RuleType: "account_type",
				ValidIf:  []any{"deposit", "savings"},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		var model OperationRoutePostgreSQLModel
		model.FromEntity(entity)

		assert.Equal(t, "account_type", model.AccountRuleType)
		assert.Equal(t, "deposit,savings", model.AccountRuleValidIf)
	})

	t.Run("with_empty_code", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		entity := &mmodel.OperationRoute{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "No Code Route",
			Code:           "   ",
			OperationType:  "source",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		var model OperationRoutePostgreSQLModel
		model.FromEntity(entity)

		assert.False(t, model.Code.Valid, "Code should be invalid when entity.Code is whitespace only")
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		entity := &mmodel.OperationRoute{
			ID:             id,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "Simple Route",
			OperationType:  "source",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		var model OperationRoutePostgreSQLModel
		model.FromEntity(entity)

		assert.Equal(t, entity.ID, model.ID)
		assert.Empty(t, model.Description)
		assert.False(t, model.Code.Valid, "Code should be invalid when empty")
		assert.Empty(t, model.AccountRuleType)
		assert.Empty(t, model.AccountRuleValidIf)
		assert.False(t, model.DeletedAt.Valid)
	})

	t.Run("with_nil_entity", func(t *testing.T) {
		var model OperationRoutePostgreSQLModel
		initialID := model.ID

		model.FromEntity(nil)

		assert.Equal(t, initialID, model.ID, "Model should remain unchanged when entity is nil")
	})

	t.Run("converts_operation_type_to_lowercase", func(t *testing.T) {
		cases := []struct {
			input    string
			expected string
		}{
			{"SOURCE", "source"},
			{"DESTINATION", "destination"},
			{"Source", "source"},
			{"MixedCase", "mixedcase"},
		}

		for _, tc := range cases {
			entity := &mmodel.OperationRoute{
				ID:             uuid.New(),
				OrganizationID: uuid.New(),
				LedgerID:       uuid.New(),
				Title:          "Type Test",
				OperationType:  tc.input,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			var model OperationRoutePostgreSQLModel
			model.FromEntity(entity)

			assert.Equal(t, tc.expected, model.OperationType, "OperationType '%s' should be converted to '%s'", tc.input, tc.expected)
		}
	})
}
