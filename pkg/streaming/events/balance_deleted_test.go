// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBalanceDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "balance.deleted", events.BalanceDeletedDefinition.Key())
	assert.Equal(t, "balance", events.BalanceDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.BalanceDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.BalanceDeletedDefinition.SchemaVersion)
}

func TestNewBalanceDeleted_MapsIdentityAndTimestamp(t *testing.T) {
	payload := events.NewBalanceDeleted(balanceID, balanceOrg, balanceLed, balanceAccount, fixedTime)

	assert.Equal(t, balanceID, payload.ID)
	assert.Equal(t, balanceOrg, payload.OrganizationID)
	assert.Equal(t, balanceLed, payload.LedgerID)
	assert.Equal(t, balanceAccount, payload.AccountID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

func TestBalanceDeletedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewBalanceDeleted(balanceID, balanceOrg, balanceLed, balanceAccount, fixedTime)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.BalanceDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.BalanceDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestBalanceDeletedPayload_JSONShape_IncludesAllRequiredFields(t *testing.T) {
	payload := events.NewBalanceDeleted(balanceID, balanceOrg, balanceLed, balanceAccount, fixedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "accountId", "deletedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	assert.Lenf(t, generic, 5, "expected 5 top-level fields, got %d (drift?)", len(generic))
}
