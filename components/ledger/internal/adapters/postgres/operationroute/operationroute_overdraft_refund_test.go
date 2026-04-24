// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// T-008: Accounting Entries Extension — JSONB persistence for Overdraft
// and Refund fields in the PostgreSQL adapter.
//
// These tests prove that:
//  1. ToEntity() restores the new fields from JSONB bytes stored in the
//     accounting_entries column.
//  2. FromEntity() serialises the new fields into JSONB bytes.
//  3. Round-trip (entity → model → entity) preserves all new-field values.
//  4. Legacy JSONB rows (missing overdraft/refund) unmarshal cleanly —
//     both new fields become nil pointers (Go zero value for pointer
//     types) without returning an error.
//
// These tests MUST FAIL until T-008 GREEN introduces the Overdraft and
// Refund fields on mmodel.AccountingEntries; they cannot even compile
// against the current model.
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

// TestToEntity_OverdraftAndRefund_JSONB verifies that JSONB bytes
// containing the new overdraft/refund keys are restored onto the entity.
func TestToEntity_OverdraftAndRefund_JSONB(t *testing.T) {
	entries := &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
			Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
		},
		Overdraft: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
			Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
		},
		Refund: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "1007", Description: "Refund Debit"},
			Credit: &mmodel.AccountingRubric{Code: "2007", Description: "Refund Credit"},
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
	require.NotNil(t, entity.AccountingEntries.Refund,
		"Refund must be unmarshalled from JSONB bytes")

	assert.Equal(t, "1006", entity.AccountingEntries.Overdraft.Debit.Code)
	assert.Equal(t, "Overdraft Debit", entity.AccountingEntries.Overdraft.Debit.Description)
	assert.Equal(t, "2006", entity.AccountingEntries.Overdraft.Credit.Code)
	assert.Equal(t, "Overdraft Credit", entity.AccountingEntries.Overdraft.Credit.Description)

	assert.Equal(t, "1007", entity.AccountingEntries.Refund.Debit.Code)
	assert.Equal(t, "Refund Debit", entity.AccountingEntries.Refund.Debit.Description)
	assert.Equal(t, "2007", entity.AccountingEntries.Refund.Credit.Code)
	assert.Equal(t, "Refund Credit", entity.AccountingEntries.Refund.Credit.Description)
}

// TestToEntity_LegacyJSONB_NoOverdraftRefund verifies that JSONB bytes
// stored BEFORE T-008 (no overdraft/refund keys) unmarshal cleanly and
// leave both new pointer fields at nil.
func TestToEntity_LegacyJSONB_NoOverdraftRefund(t *testing.T) {
	// Simulate a legacy JSONB row stored before T-008.
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

	// JSONB backward compatibility: new pointer fields default to nil.
	assert.Nil(t, entity.AccountingEntries.Overdraft,
		"Overdraft must be nil on legacy rows lacking the key")
	assert.Nil(t, entity.AccountingEntries.Refund,
		"Refund must be nil on legacy rows lacking the key")
}

// TestFromEntity_OverdraftAndRefund_JSONB verifies that FromEntity
// serialises Overdraft and Refund into JSONB bytes.
func TestFromEntity_OverdraftAndRefund_JSONB(t *testing.T) {
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
			Refund: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "1007", Description: "Refund Debit"},
				Credit: &mmodel.AccountingRubric{Code: "2007", Description: "Refund Credit"},
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
	require.NotNil(t, result.Refund)

	assert.Equal(t, "1006", result.Overdraft.Debit.Code)
	assert.Equal(t, "Overdraft Debit", result.Overdraft.Debit.Description)
	assert.Equal(t, "2006", result.Overdraft.Credit.Code)
	assert.Equal(t, "2007", result.Refund.Credit.Code)
	assert.Equal(t, "Refund Credit", result.Refund.Credit.Description)
}

// TestOverdraftRefund_RoundTrip_Entity_Model_Entity verifies that a full
// entity → model → entity cycle through JSONB preserves Overdraft and
// Refund fields without loss.
func TestOverdraftRefund_RoundTrip_Entity_Model_Entity(t *testing.T) {
	original := &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Round Trip Route",
		Description:    "Overdraft + Refund round-trip",
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
			Refund: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "1007", Description: "Refund Debit"},
				Credit: &mmodel.AccountingRubric{Code: "2007", Description: "Refund Credit"},
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
	require.NotNil(t, roundTripped.AccountingEntries.Refund)

	assert.Equal(t, original.AccountingEntries.Overdraft.Debit.Code,
		roundTripped.AccountingEntries.Overdraft.Debit.Code)
	assert.Equal(t, original.AccountingEntries.Overdraft.Credit.Description,
		roundTripped.AccountingEntries.Overdraft.Credit.Description)
	assert.Equal(t, original.AccountingEntries.Refund.Debit.Code,
		roundTripped.AccountingEntries.Refund.Debit.Code)
	assert.Equal(t, original.AccountingEntries.Refund.Credit.Description,
		roundTripped.AccountingEntries.Refund.Credit.Description)
}
