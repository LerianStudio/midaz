// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// T-008 + T-014: Accounting Entries Extension — JSONB persistence for
// Overdraft field in the PostgreSQL adapter.
//
// After T-014, the Refund field was collapsed into Overdraft. These tests
// prove that:
//  1. ToEntity() restores the Overdraft field from JSONB bytes.
//  2. FromEntity() serialises the Overdraft field into JSONB bytes.
//  3. Round-trip (entity → model → entity) preserves all values.
//  4. Legacy JSONB rows (missing overdraft) unmarshal cleanly.
//  5. Legacy JSONB rows containing "refund" keys are silently ignored.
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

// TestToEntity_Overdraft_JSONB verifies that JSONB bytes containing
// the overdraft key are restored onto the entity.
func TestToEntity_Overdraft_JSONB(t *testing.T) {
	entries := &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
			Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
		},
		Overdraft: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
			Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
		},
	}

	raw, err := json.Marshal(entries)
	require.NoError(t, err)

	model := &OperationRoutePostgreSQLModel{
		ID:                uuid.New(),
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Title:             "Overdraft Route",
		OperationType:     "bidirectional",
		AccountingEntries: raw,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	entity := model.ToEntity()
	require.NotNil(t, entity)
	require.NotNil(t, entity.AccountingEntries)

	require.NotNil(t, entity.AccountingEntries.Overdraft,
		"Overdraft must be unmarshalled from JSONB bytes")

	assert.Equal(t, "1006", entity.AccountingEntries.Overdraft.Debit.Code)
	assert.Equal(t, "Overdraft Debit", entity.AccountingEntries.Overdraft.Debit.Description)
	assert.Equal(t, "2006", entity.AccountingEntries.Overdraft.Credit.Code)
	assert.Equal(t, "Overdraft Credit", entity.AccountingEntries.Overdraft.Credit.Description)
}

// TestToEntity_LegacyJSONB_NoOverdraft verifies that JSONB bytes stored
// BEFORE overdraft was added unmarshal cleanly and leave the new pointer
// field at nil.
func TestToEntity_LegacyJSONB_NoOverdraft(t *testing.T) {
	legacyBytes := []byte(`{
		"direct":{
			"debit":{"code":"1001","description":"Cash"},
			"credit":{"code":"2001","description":"Revenue"}
		},
		"hold":{
			"debit":{"code":"1002","description":"Held"},
			"credit":{"code":"2002","description":"Held Revenue"}
		}
	}`)

	model := &OperationRoutePostgreSQLModel{
		ID:                uuid.New(),
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Title:             "Legacy Route",
		OperationType:     "source",
		AccountingEntries: legacyBytes,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	entity := model.ToEntity()
	require.NotNil(t, entity)
	require.NotNil(t, entity.AccountingEntries, "legacy JSONB must unmarshal successfully")
	require.NotNil(t, entity.AccountingEntries.Direct)
	require.NotNil(t, entity.AccountingEntries.Hold)

	assert.Nil(t, entity.AccountingEntries.Overdraft,
		"Overdraft must be nil on legacy rows lacking the key")
}

// TestToEntity_LegacyJSONB_WithRefundKey verifies that JSONB bytes
// containing the removed "refund" key (from pre-T-014 dev rows)
// unmarshal cleanly — the unknown key is silently dropped.
func TestToEntity_LegacyJSONB_WithRefundKey(t *testing.T) {
	legacyBytes := []byte(`{
		"direct":{
			"debit":{"code":"1001","description":"Cash"},
			"credit":{"code":"2001","description":"Revenue"}
		},
		"overdraft":{
			"debit":{"code":"1006","description":"Overdraft Debit"},
			"credit":{"code":"2006","description":"Overdraft Credit"}
		},
		"refund":{
			"debit":{"code":"1007","description":"Legacy Refund Debit"},
			"credit":{"code":"2007","description":"Legacy Refund Credit"}
		}
	}`)

	model := &OperationRoutePostgreSQLModel{
		ID:                uuid.New(),
		OrganizationID:    uuid.New(),
		LedgerID:          uuid.New(),
		Title:             "Legacy Route with Refund",
		OperationType:     "bidirectional",
		AccountingEntries: legacyBytes,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	entity := model.ToEntity()
	require.NotNil(t, entity)
	require.NotNil(t, entity.AccountingEntries)
	require.NotNil(t, entity.AccountingEntries.Overdraft,
		"Overdraft must still be decoded even when refund key is present")
	assert.Equal(t, "1006", entity.AccountingEntries.Overdraft.Debit.Code)
}

// TestFromEntity_Overdraft_JSONB verifies that FromEntity serialises
// the Overdraft field into JSONB bytes.
func TestFromEntity_Overdraft_JSONB(t *testing.T) {
	entity := &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Overdraft Route",
		OperationType:  "bidirectional",
		AccountingEntries: &mmodel.AccountingEntries{
			Direct: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
				Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
			},
			Overdraft: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
				Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	var model OperationRoutePostgreSQLModel
	model.FromEntity(entity)

	require.NotNil(t, model.AccountingEntries, "JSONB bytes must be populated")

	var result mmodel.AccountingEntries
	require.NoError(t, json.Unmarshal(model.AccountingEntries, &result))

	require.NotNil(t, result.Overdraft)
	assert.Equal(t, "1006", result.Overdraft.Debit.Code)
	assert.Equal(t, "Overdraft Debit", result.Overdraft.Debit.Description)
	assert.Equal(t, "2006", result.Overdraft.Credit.Code)
	assert.Equal(t, "Overdraft Credit", result.Overdraft.Credit.Description)
}

// TestOverdraft_RoundTrip_Entity_Model_Entity verifies that a full
// entity → model → entity cycle through JSONB preserves the Overdraft
// field without loss.
func TestOverdraft_RoundTrip_Entity_Model_Entity(t *testing.T) {
	original := &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Round Trip Route",
		Description:    "Overdraft round-trip",
		OperationType:  "bidirectional",
		AccountingEntries: &mmodel.AccountingEntries{
			Direct: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
				Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
			},
			Overdraft: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
				Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	var model OperationRoutePostgreSQLModel
	model.FromEntity(original)

	roundTripped := model.ToEntity()
	require.NotNil(t, roundTripped)
	require.NotNil(t, roundTripped.AccountingEntries)
	require.NotNil(t, roundTripped.AccountingEntries.Overdraft)

	assert.Equal(t, original.AccountingEntries.Overdraft.Debit.Code,
		roundTripped.AccountingEntries.Overdraft.Debit.Code)
	assert.Equal(t, original.AccountingEntries.Overdraft.Credit.Description,
		roundTripped.AccountingEntries.Overdraft.Credit.Description)
}
