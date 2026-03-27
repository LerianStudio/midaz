// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operationroute

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToEntity_AccountingEntries(t *testing.T) {
	baseModel := func() *OperationRoutePostgreSQLModel {
		return &OperationRoutePostgreSQLModel{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			Title:          "Test Route",
			OperationType:  "source",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}

	t.Run("nil_accounting_entries_returns_nil_on_entity", func(t *testing.T) {
		model := baseModel()
		model.AccountingEntries = nil

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.AccountingEntries, "AccountingEntries should be nil when DB bytes are nil")
	})

	t.Run("empty_accounting_entries_returns_nil_on_entity", func(t *testing.T) {
		model := baseModel()
		model.AccountingEntries = []byte{}

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.AccountingEntries, "AccountingEntries should be nil when DB bytes are empty")
	})

	t.Run("valid_jsonb_unmarshals_correctly", func(t *testing.T) {
		entries := &mmodel.AccountingEntries{
			Direct: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
				Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
			},
			Hold: &mmodel.AccountingEntry{
				Debit: &mmodel.AccountingRubric{Code: "1002", Description: "Held Funds"},
			},
		}

		jsonBytes, err := json.Marshal(entries)
		require.NoError(t, err)

		model := baseModel()
		model.AccountingEntries = jsonBytes

		entity := model.ToEntity()

		require.NotNil(t, entity)
		require.NotNil(t, entity.AccountingEntries)
		require.NotNil(t, entity.AccountingEntries.Direct)
		require.NotNil(t, entity.AccountingEntries.Direct.Debit)
		assert.Equal(t, "1001", entity.AccountingEntries.Direct.Debit.Code)
		assert.Equal(t, "Cash", entity.AccountingEntries.Direct.Debit.Description)
		require.NotNil(t, entity.AccountingEntries.Direct.Credit)
		assert.Equal(t, "2001", entity.AccountingEntries.Direct.Credit.Code)
		assert.Equal(t, "Revenue", entity.AccountingEntries.Direct.Credit.Description)
		require.NotNil(t, entity.AccountingEntries.Hold)
		require.NotNil(t, entity.AccountingEntries.Hold.Debit)
		assert.Equal(t, "1002", entity.AccountingEntries.Hold.Debit.Code)
		assert.Nil(t, entity.AccountingEntries.Commit)
		assert.Nil(t, entity.AccountingEntries.Cancel)
		assert.Nil(t, entity.AccountingEntries.Revert)
	})

	t.Run("malformed_json_returns_nil_accounting_entries", func(t *testing.T) {
		model := baseModel()
		model.AccountingEntries = []byte(`{invalid json`)

		entity := model.ToEntity()

		require.NotNil(t, entity)
		assert.Nil(t, entity.AccountingEntries, "AccountingEntries should be nil when JSON is malformed")
	})
}

func TestFromEntity_AccountingEntries(t *testing.T) {
	baseEntity := func() *mmodel.OperationRoute {
		return &mmodel.OperationRoute{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			Title:          "Test Route",
			OperationType:  "source",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}

	t.Run("nil_accounting_entries_writes_nil_bytes", func(t *testing.T) {
		entity := baseEntity()
		entity.AccountingEntries = nil

		var model OperationRoutePostgreSQLModel
		model.FromEntity(entity)

		assert.Nil(t, model.AccountingEntries, "AccountingEntries bytes should be nil when entity field is nil")
	})

	t.Run("populated_accounting_entries_marshals_to_json", func(t *testing.T) {
		entity := baseEntity()
		entity.AccountingEntries = &mmodel.AccountingEntries{
			Direct: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
				Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
			},
			Cancel: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
				Credit: &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
			},
		}

		var model OperationRoutePostgreSQLModel
		model.FromEntity(entity)

		require.NotNil(t, model.AccountingEntries, "AccountingEntries bytes should not be nil")

		var result mmodel.AccountingEntries
		err := json.Unmarshal(model.AccountingEntries, &result)
		require.NoError(t, err, "AccountingEntries bytes should be valid JSON")
		require.NotNil(t, result.Direct)
		assert.Equal(t, "1001", result.Direct.Debit.Code)
		require.NotNil(t, result.Cancel)
		assert.Equal(t, "2001", result.Cancel.Debit.Code)
		assert.Nil(t, result.Hold)
		assert.Nil(t, result.Commit)
		assert.Nil(t, result.Revert)
	})
}

func TestAccountingEntries_RoundTrip(t *testing.T) {
	t.Run("entity_to_model_to_entity_preserves_all_fields", func(t *testing.T) {
		original := &mmodel.OperationRoute{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			Title:          "Round Trip Route",
			Description:    "Testing round trip",
			Code:           "RT-001",
			OperationType:  "source",
			AccountingEntries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit: &mmodel.AccountingRubric{Code: "1002", Description: "Held"},
				},
				Commit: &mmodel.AccountingEntry{
					Credit: &mmodel.AccountingRubric{Code: "3001", Description: "Committed"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "4001", Description: "Cancelled Debit"},
					Credit: &mmodel.AccountingRubric{Code: "4002", Description: "Cancelled Credit"},
				},
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "5001", Description: "Reverted Debit"},
					Credit: &mmodel.AccountingRubric{Code: "5002", Description: "Reverted Credit"},
				},
			},
			CreatedAt: time.Now().Truncate(time.Microsecond),
			UpdatedAt: time.Now().Truncate(time.Microsecond),
		}

		// Entity -> DB model
		var model OperationRoutePostgreSQLModel
		model.FromEntity(original)

		// DB model -> Entity
		restored := model.ToEntity()

		require.NotNil(t, restored)
		require.NotNil(t, restored.AccountingEntries)

		ae := restored.AccountingEntries

		// Direct
		require.NotNil(t, ae.Direct)
		require.NotNil(t, ae.Direct.Debit)
		assert.Equal(t, "1001", ae.Direct.Debit.Code)
		assert.Equal(t, "Cash", ae.Direct.Debit.Description)
		require.NotNil(t, ae.Direct.Credit)
		assert.Equal(t, "2001", ae.Direct.Credit.Code)
		assert.Equal(t, "Revenue", ae.Direct.Credit.Description)

		// Hold
		require.NotNil(t, ae.Hold)
		require.NotNil(t, ae.Hold.Debit)
		assert.Equal(t, "1002", ae.Hold.Debit.Code)
		assert.Equal(t, "Held", ae.Hold.Debit.Description)

		// Commit
		require.NotNil(t, ae.Commit)
		require.NotNil(t, ae.Commit.Credit)
		assert.Equal(t, "3001", ae.Commit.Credit.Code)

		// Cancel
		require.NotNil(t, ae.Cancel)
		require.NotNil(t, ae.Cancel.Debit)
		assert.Equal(t, "4001", ae.Cancel.Debit.Code)
		require.NotNil(t, ae.Cancel.Credit)
		assert.Equal(t, "4002", ae.Cancel.Credit.Code)

		// Revert
		require.NotNil(t, ae.Revert)
		require.NotNil(t, ae.Revert.Debit)
		assert.Equal(t, "5001", ae.Revert.Debit.Code)
		require.NotNil(t, ae.Revert.Credit)
		assert.Equal(t, "5002", ae.Revert.Credit.Code)
	})

	t.Run("nil_accounting_entries_round_trip", func(t *testing.T) {
		original := &mmodel.OperationRoute{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			Title:          "No Accounting",
			OperationType:  "source",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		var model OperationRoutePostgreSQLModel
		model.FromEntity(original)

		restored := model.ToEntity()

		require.NotNil(t, restored)
		assert.Nil(t, restored.AccountingEntries, "Nil AccountingEntries should survive round trip")
	})
}
