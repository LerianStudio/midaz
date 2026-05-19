// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	accountTypeID  = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")
	accountTypeOrg = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0b")
	accountTypeLed = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0c")
)

func minimalAccountType() *mmodel.AccountType {
	return &mmodel.AccountType{
		ID:             accountTypeID,
		OrganizationID: accountTypeOrg,
		LedgerID:       accountTypeLed,
		Name:           "Current Assets",
		KeyValue:       "current_assets",
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
}

func TestAccountTypeCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "account-type.created", events.AccountTypeCreatedDefinition.Key())
	assert.Equal(t, "account-type", events.AccountTypeCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.AccountTypeCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AccountTypeCreatedDefinition.SchemaVersion)
}

func TestNewAccountTypeCreated_MapsMinimalAccountType(t *testing.T) {
	a := minimalAccountType()

	payload := events.NewAccountTypeCreated(a)

	assert.Equal(t, accountTypeID.String(), payload.ID)
	assert.Equal(t, accountTypeOrg.String(), payload.OrganizationID)
	assert.Equal(t, accountTypeLed.String(), payload.LedgerID)
	assert.Equal(t, "Current Assets", payload.Name)
	assert.Empty(t, payload.Description)
	assert.Equal(t, "current_assets", payload.KeyValue)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewAccountTypeCreated_MapsDescription(t *testing.T) {
	a := minimalAccountType()
	a.Description = "Assets convertible to cash within one year"

	payload := events.NewAccountTypeCreated(a)

	assert.Equal(t, "Assets convertible to cash within one year", payload.Description)
}

func TestAccountTypeCreatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAccountTypeCreated(minimalAccountType())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.AccountTypeCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.AccountTypeCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestAccountTypeCreatedPayload_JSONShape_MinimalOmitsDescription(t *testing.T) {
	payload := events.NewAccountTypeCreated(minimalAccountType())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "name", "keyValue", "createdAt", "updatedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	_, hasDescription := generic["description"]
	assert.False(t, hasDescription, "description must omitempty when empty")

	assert.Lenf(t, generic, 7, "expected 7 top-level fields when description omitted, got %d (drift?)", len(generic))
}

func TestAccountTypeCreatedPayload_JSONShape_WithDescriptionIncludesIt(t *testing.T) {
	a := minimalAccountType()
	a.Description = "Long-form description"

	payload := events.NewAccountTypeCreated(a)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	desc, ok := generic["description"]
	require.True(t, ok, "description must be present when non-empty")
	assert.Equal(t, "Long-form description", desc)

	assert.Lenf(t, generic, 8, "expected 8 top-level fields when description present, got %d (drift?)", len(generic))
}
