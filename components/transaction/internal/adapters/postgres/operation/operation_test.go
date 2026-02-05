// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"database/sql"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationPostgreSQLModel_ToEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		amount := decimal.NewFromFloat(1500.00)
		availableBalance := decimal.NewFromFloat(5000.00)
		onHoldBalance := decimal.NewFromFloat(500.00)
		availableBalanceAfter := decimal.NewFromFloat(3500.00)
		onHoldBalanceAfter := decimal.NewFromFloat(500.00)
		versionBalance := int64(1)
		versionBalanceAfter := int64(2)
		statusDesc := "Transaction completed"
		route := "route-123"
		deletedAt := time.Now().Add(-24 * time.Hour)

		model := &OperationPostgreSQLModel{
			ID:                    "op-123",
			TransactionID:         "tx-456",
			Description:           "Payment operation",
			Type:                  "DEBIT",
			AssetCode:             "BRL",
			Amount:                &amount,
			AvailableBalance:      &availableBalance,
			OnHoldBalance:         &onHoldBalance,
			VersionBalance:        &versionBalance,
			AvailableBalanceAfter: &availableBalanceAfter,
			OnHoldBalanceAfter:    &onHoldBalanceAfter,
			VersionBalanceAfter:   &versionBalanceAfter,
			Status:                "ACTIVE",
			StatusDescription:     &statusDesc,
			AccountID:             "acc-789",
			AccountAlias:          "@main",
			BalanceKey:            "default",
			BalanceID:             "bal-012",
			ChartOfAccounts:       "1000",
			OrganizationID:        "org-345",
			LedgerID:              "ledger-678",
			Route:                 &route,
			BalanceAffected:       true,
			CreatedAt:             time.Now().Add(-48 * time.Hour),
			UpdatedAt:             time.Now().Add(-1 * time.Hour),
			DeletedAt:             sql.NullTime{Time: deletedAt, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Equal(t, model.TransactionID, entity.TransactionID)
		assert.Equal(t, model.Description, entity.Description)
		assert.Equal(t, model.Type, entity.Type)
		assert.Equal(t, model.AssetCode, entity.AssetCode)
		assert.Equal(t, model.ChartOfAccounts, entity.ChartOfAccounts)
		// Amount
		require.NotNil(t, entity.Amount.Value)
		assert.True(t, amount.Equal(*entity.Amount.Value))
		// Balance (before)
		require.NotNil(t, entity.Balance.Available)
		assert.True(t, availableBalance.Equal(*entity.Balance.Available))
		require.NotNil(t, entity.Balance.OnHold)
		assert.True(t, onHoldBalance.Equal(*entity.Balance.OnHold))
		require.NotNil(t, entity.Balance.Version)
		assert.Equal(t, versionBalance, *entity.Balance.Version)
		// Balance (after)
		require.NotNil(t, entity.BalanceAfter.Available)
		assert.True(t, availableBalanceAfter.Equal(*entity.BalanceAfter.Available))
		require.NotNil(t, entity.BalanceAfter.OnHold)
		assert.True(t, onHoldBalanceAfter.Equal(*entity.BalanceAfter.OnHold))
		require.NotNil(t, entity.BalanceAfter.Version)
		assert.Equal(t, versionBalanceAfter, *entity.BalanceAfter.Version)
		// Status
		assert.Equal(t, model.Status, entity.Status.Code)
		assert.Equal(t, model.StatusDescription, entity.Status.Description)
		// Other fields
		assert.Equal(t, model.AccountID, entity.AccountID)
		assert.Equal(t, model.AccountAlias, entity.AccountAlias)
		assert.Equal(t, model.BalanceKey, entity.BalanceKey)
		assert.Equal(t, model.BalanceID, entity.BalanceID)
		assert.Equal(t, model.OrganizationID, entity.OrganizationID)
		assert.Equal(t, model.LedgerID, entity.LedgerID)
		assert.Equal(t, route, entity.Route)
		assert.True(t, entity.BalanceAffected)
		assert.Equal(t, model.CreatedAt, entity.CreatedAt)
		assert.Equal(t, model.UpdatedAt, entity.UpdatedAt)
		require.NotNil(t, entity.DeletedAt)
		assert.Equal(t, deletedAt, *entity.DeletedAt)
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		model := &OperationPostgreSQLModel{
			ID:              "op-456",
			TransactionID:   "tx-789",
			Type:            "CREDIT",
			AssetCode:       "USD",
			Status:          "PENDING",
			AccountID:       "acc-123",
			BalanceID:       "bal-456",
			OrganizationID:  "org-789",
			LedgerID:        "ledger-012",
			BalanceAffected: false,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Equal(t, model.ID, entity.ID)
		assert.Empty(t, entity.Description)
		assert.Nil(t, entity.Amount.Value)
		assert.Nil(t, entity.Balance.Available)
		assert.Nil(t, entity.Balance.OnHold)
		assert.Nil(t, entity.Balance.Version)
		assert.Nil(t, entity.BalanceAfter.Available)
		assert.Nil(t, entity.BalanceAfter.OnHold)
		assert.Nil(t, entity.BalanceAfter.Version)
		assert.Nil(t, entity.Status.Description)
		assert.Empty(t, entity.Route)
		assert.False(t, entity.BalanceAffected)
		assert.Nil(t, entity.DeletedAt)
	})

	t.Run("with_route_nil", func(t *testing.T) {
		model := &OperationPostgreSQLModel{
			ID:              "op-route-nil",
			TransactionID:   "tx-route",
			Type:            "DEBIT",
			AssetCode:       "BRL",
			Status:          "ACTIVE",
			AccountID:       "acc-route",
			BalanceID:       "bal-route",
			OrganizationID:  "org-route",
			LedgerID:        "ledger-route",
			Route:           nil,
			BalanceAffected: true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Empty(t, entity.Route, "Route should be empty when model.Route is nil")
	})

	t.Run("with_deleted_at_valid_but_zero_time", func(t *testing.T) {
		model := &OperationPostgreSQLModel{
			ID:              "op-edge",
			TransactionID:   "tx-edge",
			Type:            "DEBIT",
			AssetCode:       "EUR",
			Status:          "ACTIVE",
			AccountID:       "acc-edge",
			BalanceID:       "bal-edge",
			OrganizationID:  "org-edge",
			LedgerID:        "ledger-edge",
			BalanceAffected: true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
			DeletedAt:       sql.NullTime{Time: time.Time{}, Valid: true},
		}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.DeletedAt, "DeletedAt should be nil when Time is zero, regardless of Valid flag")
	})
}

func TestOperationPostgreSQLModel_FromEntity(t *testing.T) {
	t.Run("with_all_fields_populated", func(t *testing.T) {
		amount := decimal.NewFromFloat(1500.00)
		availableBalance := decimal.NewFromFloat(5000.00)
		onHoldBalance := decimal.NewFromFloat(500.00)
		availableBalanceAfter := decimal.NewFromFloat(3500.00)
		onHoldBalanceAfter := decimal.NewFromFloat(500.00)
		versionBalance := int64(1)
		versionBalanceAfter := int64(2)
		statusDesc := "Transaction completed"
		deletedAt := time.Now().Add(-24 * time.Hour)

		entity := &Operation{
			ID:            "op-existing-id",
			TransactionID: "tx-456",
			Description:   "Payment operation",
			Type:          "DEBIT",
			AssetCode:     "BRL",
			Amount: Amount{
				Value: &amount,
			},
			Balance: Balance{
				Available: &availableBalance,
				OnHold:    &onHoldBalance,
				Version:   &versionBalance,
			},
			BalanceAfter: Balance{
				Available: &availableBalanceAfter,
				OnHold:    &onHoldBalanceAfter,
				Version:   &versionBalanceAfter,
			},
			Status: Status{
				Code:        "ACTIVE",
				Description: &statusDesc,
			},
			AccountID:       "acc-789",
			AccountAlias:    "@main",
			BalanceKey:      "default",
			BalanceID:       "bal-012",
			ChartOfAccounts: "1000",
			OrganizationID:  "org-345",
			LedgerID:        "ledger-678",
			Route:           "route-123",
			BalanceAffected: true,
			CreatedAt:       time.Now().Add(-48 * time.Hour),
			UpdatedAt:       time.Now().Add(-1 * time.Hour),
			DeletedAt:       &deletedAt,
		}

		var model OperationPostgreSQLModel
		model.FromEntity(entity)

		assert.Equal(t, entity.ID, model.ID, "ID should be preserved when provided")
		assert.Equal(t, entity.TransactionID, model.TransactionID)
		assert.Equal(t, entity.Description, model.Description)
		assert.Equal(t, entity.Type, model.Type)
		assert.Equal(t, entity.AssetCode, model.AssetCode)
		assert.Equal(t, entity.ChartOfAccounts, model.ChartOfAccounts)
		// Amount
		require.NotNil(t, model.Amount)
		assert.True(t, amount.Equal(*model.Amount))
		// Balance (before)
		require.NotNil(t, model.AvailableBalance)
		assert.True(t, availableBalance.Equal(*model.AvailableBalance))
		require.NotNil(t, model.OnHoldBalance)
		assert.True(t, onHoldBalance.Equal(*model.OnHoldBalance))
		require.NotNil(t, model.VersionBalance)
		assert.Equal(t, versionBalance, *model.VersionBalance)
		// Balance (after)
		require.NotNil(t, model.AvailableBalanceAfter)
		assert.True(t, availableBalanceAfter.Equal(*model.AvailableBalanceAfter))
		require.NotNil(t, model.OnHoldBalanceAfter)
		assert.True(t, onHoldBalanceAfter.Equal(*model.OnHoldBalanceAfter))
		require.NotNil(t, model.VersionBalanceAfter)
		assert.Equal(t, versionBalanceAfter, *model.VersionBalanceAfter)
		// Status
		assert.Equal(t, entity.Status.Code, model.Status)
		assert.Equal(t, entity.Status.Description, model.StatusDescription)
		// Other fields
		assert.Equal(t, entity.AccountID, model.AccountID)
		assert.Equal(t, entity.AccountAlias, model.AccountAlias)
		assert.Equal(t, entity.BalanceKey, model.BalanceKey)
		assert.Equal(t, entity.BalanceID, model.BalanceID)
		assert.Equal(t, entity.OrganizationID, model.OrganizationID)
		assert.Equal(t, entity.LedgerID, model.LedgerID)
		require.NotNil(t, model.Route)
		assert.Equal(t, entity.Route, *model.Route)
		assert.True(t, model.BalanceAffected)
		assert.Equal(t, entity.CreatedAt, model.CreatedAt)
		assert.Equal(t, entity.UpdatedAt, model.UpdatedAt)
		assert.True(t, model.DeletedAt.Valid)
		assert.Equal(t, deletedAt, model.DeletedAt.Time)
	})

	t.Run("with_optional_fields_nil", func(t *testing.T) {
		entity := &Operation{
			TransactionID:   "tx-789",
			Type:            "CREDIT",
			AssetCode:       "USD",
			Status:          Status{Code: "PENDING"},
			AccountID:       "acc-123",
			BalanceID:       "bal-456",
			OrganizationID:  "org-789",
			LedgerID:        "ledger-012",
			BalanceAffected: false,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		var model OperationPostgreSQLModel
		model.FromEntity(entity)

		assert.NotEmpty(t, model.ID, "ID should be generated when entity.ID is empty")
		assert.Empty(t, model.Description)
		assert.Nil(t, model.Amount)
		assert.Nil(t, model.AvailableBalance)
		assert.Nil(t, model.OnHoldBalance)
		assert.Nil(t, model.VersionBalance)
		assert.Nil(t, model.AvailableBalanceAfter)
		assert.Nil(t, model.OnHoldBalanceAfter)
		assert.Nil(t, model.VersionBalanceAfter)
		assert.Nil(t, model.StatusDescription)
		assert.Nil(t, model.Route)
		assert.False(t, model.BalanceAffected)
		assert.False(t, model.DeletedAt.Valid)
	})

	t.Run("generates_uuid_when_id_empty", func(t *testing.T) {
		entity := &Operation{
			ID:              "",
			TransactionID:   "tx-uuid",
			Type:            "DEBIT",
			AssetCode:       "EUR",
			Status:          Status{Code: "ACTIVE"},
			AccountID:       "acc-uuid",
			BalanceID:       "bal-uuid",
			OrganizationID:  "org-uuid",
			LedgerID:        "ledger-uuid",
			BalanceAffected: true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		var model1 OperationPostgreSQLModel
		var model2 OperationPostgreSQLModel
		model1.FromEntity(entity)
		model2.FromEntity(entity)

		assert.NotEmpty(t, model1.ID)
		assert.NotEmpty(t, model2.ID)
		assert.NotEqual(t, model1.ID, model2.ID, "Each call should generate a unique ID when entity.ID is empty")
		assert.Len(t, model1.ID, 36, "ID should be a valid UUID string")
	})

	t.Run("with_empty_route", func(t *testing.T) {
		entity := &Operation{
			ID:              "op-empty-route",
			TransactionID:   "tx-empty-route",
			Type:            "DEBIT",
			AssetCode:       "BRL",
			Status:          Status{Code: "ACTIVE"},
			AccountID:       "acc-route",
			BalanceID:       "bal-route",
			OrganizationID:  "org-route",
			LedgerID:        "ledger-route",
			Route:           "",
			BalanceAffected: true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		var model OperationPostgreSQLModel
		model.FromEntity(entity)

		assert.Nil(t, model.Route, "Route should be nil when entity.Route is empty")
	})
}

func TestStatus_IsEmpty(t *testing.T) {
	t.Run("returns_true_when_empty", func(t *testing.T) {
		status := Status{}

		assert.True(t, status.IsEmpty())
	})

	t.Run("returns_false_when_code_set", func(t *testing.T) {
		status := Status{Code: "ACTIVE"}

		assert.False(t, status.IsEmpty())
	})

	t.Run("returns_false_when_description_set", func(t *testing.T) {
		desc := "Active status"
		status := Status{Description: &desc}

		assert.False(t, status.IsEmpty())
	})

	t.Run("returns_false_when_both_set", func(t *testing.T) {
		desc := "Active status"
		status := Status{Code: "ACTIVE", Description: &desc}

		assert.False(t, status.IsEmpty())
	})
}

func TestAmount_IsEmpty(t *testing.T) {
	t.Run("returns_true_when_empty", func(t *testing.T) {
		amount := Amount{}

		assert.True(t, amount.IsEmpty())
	})

	t.Run("returns_false_when_value_set", func(t *testing.T) {
		value := decimal.NewFromFloat(100.00)
		amount := Amount{Value: &value}

		assert.False(t, amount.IsEmpty())
	})

	t.Run("returns_false_when_value_is_zero", func(t *testing.T) {
		value := decimal.NewFromFloat(0)
		amount := Amount{Value: &value}

		assert.False(t, amount.IsEmpty(), "Amount is not empty if Value pointer is set, even to zero")
	})
}

func TestBalance_IsEmpty(t *testing.T) {
	t.Run("returns_true_when_empty", func(t *testing.T) {
		balance := Balance{}

		assert.True(t, balance.IsEmpty())
	})

	t.Run("returns_false_when_available_set", func(t *testing.T) {
		available := decimal.NewFromFloat(1000.00)
		balance := Balance{Available: &available}

		assert.False(t, balance.IsEmpty())
	})

	t.Run("returns_false_when_onhold_set", func(t *testing.T) {
		onHold := decimal.NewFromFloat(500.00)
		balance := Balance{OnHold: &onHold}

		assert.False(t, balance.IsEmpty())
	})

	t.Run("returns_false_when_both_set", func(t *testing.T) {
		available := decimal.NewFromFloat(1000.00)
		onHold := decimal.NewFromFloat(500.00)
		balance := Balance{Available: &available, OnHold: &onHold}

		assert.False(t, balance.IsEmpty())
	})

	t.Run("returns_true_when_only_version_set", func(t *testing.T) {
		// IsEmpty only checks Available and OnHold, not Version
		version := int64(1)
		balance := Balance{Version: &version}

		assert.True(t, balance.IsEmpty(), "Balance.IsEmpty() only checks Available and OnHold, not Version")
	})
}

func TestOperation_ToLog(t *testing.T) {
	t.Run("converts_all_fields", func(t *testing.T) {
		amount := decimal.NewFromFloat(1500.00)
		availableBalance := decimal.NewFromFloat(5000.00)
		onHoldBalance := decimal.NewFromFloat(500.00)
		availableBalanceAfter := decimal.NewFromFloat(3500.00)
		onHoldBalanceAfter := decimal.NewFromFloat(500.00)
		versionBalance := int64(1)
		versionBalanceAfter := int64(2)
		statusDesc := "Completed"

		operation := &Operation{
			ID:              "op-log-123",
			TransactionID:   "tx-log-456",
			Description:     "This should not be in log",
			Type:            "DEBIT",
			AssetCode:       "BRL",
			ChartOfAccounts: "1000",
			Amount: Amount{
				Value: &amount,
			},
			Balance: Balance{
				Available: &availableBalance,
				OnHold:    &onHoldBalance,
				Version:   &versionBalance,
			},
			BalanceAfter: Balance{
				Available: &availableBalanceAfter,
				OnHold:    &onHoldBalanceAfter,
				Version:   &versionBalanceAfter,
			},
			Status: Status{
				Code:        "ACTIVE",
				Description: &statusDesc,
			},
			AccountID:       "acc-log-789",
			AccountAlias:    "@log-main",
			BalanceKey:      "default",
			BalanceID:       "bal-log-012",
			OrganizationID:  "org-log",
			LedgerID:        "ledger-log",
			Route:           "route-log",
			BalanceAffected: true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		log := operation.ToLog()

		require.NotNil(t, log)
		assert.Equal(t, operation.ID, log.ID)
		assert.Equal(t, operation.TransactionID, log.TransactionID)
		assert.Equal(t, operation.Type, log.Type)
		assert.Equal(t, operation.AssetCode, log.AssetCode)
		assert.Equal(t, operation.ChartOfAccounts, log.ChartOfAccounts)
		assert.Equal(t, operation.Amount, log.Amount)
		assert.Equal(t, operation.Balance, log.Balance)
		assert.Equal(t, operation.BalanceAfter, log.BalanceAfter)
		assert.Equal(t, operation.Status, log.Status)
		assert.Equal(t, operation.AccountID, log.AccountID)
		assert.Equal(t, operation.AccountAlias, log.AccountAlias)
		assert.Equal(t, operation.BalanceKey, log.BalanceKey)
		assert.Equal(t, operation.BalanceID, log.BalanceID)
		assert.Equal(t, operation.Route, log.Route)
		assert.Equal(t, operation.BalanceAffected, log.BalanceAffected)
		assert.Equal(t, operation.CreatedAt, log.CreatedAt)
	})

	t.Run("excludes_mutable_fields", func(t *testing.T) {
		// OperationLog intentionally excludes mutable fields for audit log immutability:
		// Description, OrganizationID, LedgerID, UpdatedAt, DeletedAt, Metadata
		// The OperationLog struct provides compile-time exclusion guarantee.
		// This test verifies that ToLog() correctly maps only immutable fields.
		deletedAt := time.Now().Add(-1 * time.Hour)
		operation := &Operation{
			ID:              "op-immutable",
			TransactionID:   "tx-immutable",
			Description:     "Mutable description that should not appear in log",
			Type:            "CREDIT",
			AssetCode:       "USD",
			ChartOfAccounts: "2000",
			Status:          Status{Code: "ACTIVE"},
			AccountID:       "acc-immutable",
			AccountAlias:    "@immutable",
			BalanceKey:      "default",
			BalanceID:       "bal-immutable",
			OrganizationID:  "org-should-not-appear",
			LedgerID:        "ledger-should-not-appear",
			Route:           "route-immutable",
			BalanceAffected: true,
			CreatedAt:       time.Now().Add(-48 * time.Hour),
			UpdatedAt:       time.Now(),
			DeletedAt:       &deletedAt,
			Metadata:        map[string]any{"key": "value", "nested": map[string]any{"inner": 123}},
		}

		// Verify source operation has non-empty excluded fields (proving they exist and have values)
		require.NotEmpty(t, operation.Description, "Source Description must be set for this test")
		require.NotEmpty(t, operation.OrganizationID, "Source OrganizationID must be set for this test")
		require.NotEmpty(t, operation.LedgerID, "Source LedgerID must be set for this test")
		require.False(t, operation.UpdatedAt.IsZero(), "Source UpdatedAt must be set for this test")
		require.NotNil(t, operation.DeletedAt, "Source DeletedAt must be set for this test")
		require.NotEmpty(t, operation.Metadata, "Source Metadata must be set for this test")

		log := operation.ToLog()

		require.NotNil(t, log)

		// Verify all immutable fields are correctly copied
		assert.Equal(t, operation.ID, log.ID)
		assert.Equal(t, operation.TransactionID, log.TransactionID)
		assert.Equal(t, operation.Type, log.Type)
		assert.Equal(t, operation.AssetCode, log.AssetCode)
		assert.Equal(t, operation.ChartOfAccounts, log.ChartOfAccounts)
		assert.Equal(t, operation.Amount, log.Amount)
		assert.Equal(t, operation.Balance, log.Balance)
		assert.Equal(t, operation.BalanceAfter, log.BalanceAfter)
		assert.Equal(t, operation.Status, log.Status)
		assert.Equal(t, operation.AccountID, log.AccountID)
		assert.Equal(t, operation.AccountAlias, log.AccountAlias)
		assert.Equal(t, operation.BalanceKey, log.BalanceKey)
		assert.Equal(t, operation.BalanceID, log.BalanceID)
		assert.Equal(t, operation.Route, log.Route)
		assert.Equal(t, operation.BalanceAffected, log.BalanceAffected)
		assert.Equal(t, operation.CreatedAt, log.CreatedAt)

		// Note: OperationLog struct does not have Description, OrganizationID, LedgerID,
		// UpdatedAt, DeletedAt, or Metadata fields. This is enforced at compile-time.
		// The assertions above confirm ToLog() maps exactly the fields OperationLog contains.
	})
}
