// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	closedAccountID      = "01J7K8FN5W8R0R2S7Q1V4H6J0M"
	closedOrganizationID = "01J7K7XB9C2D3E4F5G6H7J8K9L"
	closedLedgerID       = "01J7K9A1B2C3D4E5F6G7H8J9K0"
)

// TestAccountClosedDefinition_Key locks the canonical event key. Changing this
// assertion is a wire-contract change and requires a coordinated update of
// every downstream consumer.
func TestAccountClosedDefinition_Key(t *testing.T) {
	assert.Equal(t, "account.closed", events.AccountClosedDefinition.Key())
	assert.Equal(t, "account", events.AccountClosedDefinition.ResourceType)
	assert.Equal(t, "closed", events.AccountClosedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AccountClosedDefinition.SchemaVersion)
}

// TestNewAccountClosed_MapsIdentityAndPersistedInstant verifies the mapping:
// the scope and the account come through unchanged, and the timestamp the
// database returned is formatted as RFC3339.
func TestNewAccountClosed_MapsIdentityAndPersistedInstant(t *testing.T) {
	payload := events.NewAccountClosed(closedAccountID, closedOrganizationID, closedLedgerID, fixedTime)

	assert.Equal(t, closedAccountID, payload.ID)
	assert.Equal(t, closedOrganizationID, payload.OrganizationID)
	assert.Equal(t, closedLedgerID, payload.LedgerID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.ClosedAt)
}

// TestNewAccountClosed_UsesTheGivenInstantNotAWallClock proves the payload
// carries the instant handed to it. The closing timestamp comes from the
// database RETURNING clause, so a producer-side clock must never replace it:
// an instant far in the past survives verbatim.
func TestNewAccountClosed_UsesTheGivenInstantNotAWallClock(t *testing.T) {
	persisted := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

	payload := events.NewAccountClosed(closedAccountID, closedOrganizationID, closedLedgerID, persisted)

	assert.Equal(t, "2020-01-02T03:04:05Z", payload.ClosedAt)
}

// TestAccountClosedPayload_ToEmitRequest_AssemblesStreamingEvent verifies the
// ToEmitRequest helper composes a fully-populated EmitRequest. Catalog fields
// (Source/ResourceType/EventType/SchemaVersion) are not on the request and
// intentionally not asserted here.
func TestAccountClosedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAccountClosed(closedAccountID, closedOrganizationID, closedLedgerID, fixedTime)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	// Catalog routing key.
	assert.Equal(t, events.AccountClosedDefinition.Key(), req.DefinitionKey)

	// Per-emit fields. The subject is the aggregate — the account being closed.
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, closedAccountID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	// Payload round-trips back to the same struct.
	var roundTrip events.AccountClosedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestAccountClosedPayload_JSONShape locks the wire JSON layout against
// accidental field-name drift. Breaking this test is a wire-contract change;
// downstream consumers and the e2e mirror struct must be updated in the same
// PR.
func TestAccountClosedPayload_JSONShape(t *testing.T) {
	payload := events.NewAccountClosed(closedAccountID, closedOrganizationID, closedLedgerID, fixedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "closedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	assert.Lenf(t, generic, 4, "expected 4 top-level fields, got %d (drift?)", len(generic))
}

// TestAccountClosedPayload_CarriesNoClosingTransactionReference pins AC-14: a
// closing writes no transaction and no operation, so nothing on the wire may
// point at one. A field added later to carry a CLOSING entry would fail here as
// well as in the shape lock above.
func TestAccountClosedPayload_CarriesNoClosingTransactionReference(t *testing.T) {
	payload := events.NewAccountClosed(closedAccountID, closedOrganizationID, closedLedgerID, fixedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for key := range generic {
		lower := strings.ToLower(key)
		assert.NotContainsf(t, lower, "transaction", "wire payload must not reference a transaction, found %q", key)
		assert.NotContainsf(t, lower, "operation", "wire payload must not reference an operation, found %q", key)
		assert.NotContainsf(t, lower, "closing", "wire payload must not reference a CLOSING entry, found %q", key)
	}

	assert.NotContains(t, strings.ToLower(string(data)), "closing",
		"no serialized value may name a CLOSING entry")
}
