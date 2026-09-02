// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestAccountExceptionDefinitionsKeys(t *testing.T) {
	assert.Equal(t, "account_exception.created", AccountExceptionCreatedDefinition.Key())
	assert.Equal(t, "account_exception.updated", AccountExceptionUpdatedDefinition.Key())
	assert.Equal(t, "account_exception.deleted", AccountExceptionDeletedDefinition.Key())
}

func TestNewAccountExceptionCreatedAndEmit(t *testing.T) {
	effectiveAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	balanceKey := "asset-freeze"
	e := &mmodel.AccountException{
		ID:                   "01965ed9-7fa4-75b2-8872-fc9e8509ab0a",
		OrganizationID:       "org",
		LedgerID:             "led",
		AccountID:            "acc",
		OperationalTypeCodes: []string{"PIX_IN"},
		BalanceKey:           &balanceKey,
		Context:              "ctx",
		EffectiveAt:          &effectiveAt,
		CreatedAt:            effectiveAt,
		UpdatedAt:            effectiveAt,
	}

	payload := NewAccountExceptionCreated(e)
	assert.Equal(t, e.ID, payload.ID)
	require.NotNil(t, payload.EffectiveAt)
	assert.Nil(t, payload.ExpiresAt, "unbounded expiry stays off the wire")

	req, err := payload.ToEmitRequest("tenant-1", e.CreatedAt)
	require.NoError(t, err)
	assert.Equal(t, "account_exception.created", req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, e.ID, req.Subject)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(req.Payload, &decoded))
	assert.Equal(t, "asset-freeze", decoded["balanceKey"])
	assert.Equal(t, "ctx", decoded["context"])
}

func TestNewAccountExceptionUpdatedAndEmit(t *testing.T) {
	ts := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	e := &mmodel.AccountException{
		ID:                   "id",
		OrganizationID:       "org",
		LedgerID:             "led",
		AccountID:            "acc",
		OperationalTypeCodes: []string{"TED_OUT"},
		Context:              "ctx",
		CreatedAt:            ts,
		UpdatedAt:            ts,
	}

	payload := NewAccountExceptionUpdated(e)
	assert.Nil(t, payload.BalanceKey)

	req, err := payload.ToEmitRequest("tenant-2", e.UpdatedAt)
	require.NoError(t, err)
	assert.Equal(t, "account_exception.updated", req.DefinitionKey)
	assert.Equal(t, "id", req.Subject)
}

func TestNewAccountExceptionDeletedAndEmit(t *testing.T) {
	deletedAt := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	payload := NewAccountExceptionDeleted("id", "org", "led", "acc", deletedAt)
	assert.Equal(t, "acc", payload.AccountID)
	assert.Equal(t, deletedAt.Format(time.RFC3339), payload.DeletedAt)

	req, err := payload.ToEmitRequest("tenant-3", deletedAt)
	require.NoError(t, err)
	assert.Equal(t, "account_exception.deleted", req.DefinitionKey)
	assert.Equal(t, "id", req.Subject)
}

func TestFormatOptionalTime(t *testing.T) {
	assert.Nil(t, formatOptionalTime(nil))

	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := formatOptionalTime(&ts)
	require.NotNil(t, got)
	assert.Equal(t, ts.Format(time.RFC3339), *got)
}
