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

func TestAccountTypeDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "account-type.deleted", events.AccountTypeDeletedDefinition.Key())
	assert.Equal(t, "account-type", events.AccountTypeDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.AccountTypeDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AccountTypeDeletedDefinition.SchemaVersion)
}

func TestNewAccountTypeDeleted_MapsIdentity(t *testing.T) {
	payload := events.NewAccountTypeDeleted(
		accountTypeID.String(),
		accountTypeOrg.String(),
		accountTypeLed.String(),
		fixedTime,
	)

	assert.Equal(t, accountTypeID.String(), payload.ID)
	assert.Equal(t, accountTypeOrg.String(), payload.OrganizationID)
	assert.Equal(t, accountTypeLed.String(), payload.LedgerID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

func TestAccountTypeDeletedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAccountTypeDeleted("at-123", "org-456", "led-789", fixedTime)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.AccountTypeDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.AccountTypeDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestAccountTypeDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewAccountTypeDeleted("at-123", "org-456", "led-789", fixedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "deletedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	assert.Lenf(t, generic, 4, "expected 4 top-level fields, got %d (drift?)", len(generic))
}
