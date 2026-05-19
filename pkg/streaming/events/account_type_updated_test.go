// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountTypeUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "account-type.updated", events.AccountTypeUpdatedDefinition.Key())
	assert.Equal(t, "account-type", events.AccountTypeUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.AccountTypeUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AccountTypeUpdatedDefinition.SchemaVersion)
}

func TestNewAccountTypeUpdated_MapsMinimalAccountType(t *testing.T) {
	a := minimalAccountType()

	payload := events.NewAccountTypeUpdated(a)

	assert.Equal(t, accountTypeID.String(), payload.ID)
	assert.Equal(t, accountTypeOrg.String(), payload.OrganizationID)
	assert.Equal(t, accountTypeLed.String(), payload.LedgerID)
	assert.Equal(t, "Current Assets", payload.Name)
	assert.Empty(t, payload.Description)
	assert.Equal(t, "current_assets", payload.KeyValue)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewAccountTypeUpdated_MapsDescription(t *testing.T) {
	a := minimalAccountType()
	a.Description = "Updated description"

	payload := events.NewAccountTypeUpdated(a)

	assert.Equal(t, "Updated description", payload.Description)
}

func TestAccountTypeUpdatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAccountTypeUpdated(minimalAccountType())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.AccountTypeUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.AccountTypeUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestAccountTypeUpdatedPayload_JSONShape(t *testing.T) {
	payload := events.NewAccountTypeUpdated(minimalAccountType())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "name", "keyValue", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	_, hasCreatedAt := generic["createdAt"]
	assert.False(t, hasCreatedAt, "createdAt must NOT appear on account-type.updated")

	_, hasDescription := generic["description"]
	assert.False(t, hasDescription, "description must omitempty when empty")

	assert.Lenf(t, generic, 6, "expected 6 top-level fields when description omitted, got %d (drift?)", len(generic))
}
