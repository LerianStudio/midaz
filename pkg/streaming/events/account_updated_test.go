// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalUpdatedAccount returns the smallest mmodel.Account that satisfies
// the account.updated contract: required identity, name, status,
// updatedAt. Every optional pointer is left nil so tests can verify
// nullable-field handling without further setup.
func minimalUpdatedAccount() *mmodel.Account {
	return &mmodel.Account{
		ID:             "01J7K8FN5W8R0R2S7Q1V4H6J0M",
		OrganizationID: "01J7K7XB9C2D3E4F5G6H7J8K9L",
		LedgerID:       "01J7K9A1B2C3D4E5F6G7H8J9K0",
		Name:           "Treasury",
		AssetCode:      "BRL",
		Type:           "deposit",
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
}

// TestAccountUpdatedDefinition_Key locks the canonical event key.
// Changing this assertion is a wire-contract change and requires a
// coordinated update of every downstream consumer.
func TestAccountUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "account.updated", events.AccountUpdatedDefinition.Key())
	assert.Equal(t, "account", events.AccountUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.AccountUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AccountUpdatedDefinition.SchemaVersion)
}

// TestNewAccountUpdated_MapsMinimalAccount verifies the happy-path
// mapping for the simplest post-update record: ACTIVE status, no blocked
// flag, no portfolio/segment/entity references.
func TestNewAccountUpdated_MapsMinimalAccount(t *testing.T) {
	acc := minimalUpdatedAccount()

	payload := events.NewAccountUpdated(acc)

	// Identity.
	assert.Equal(t, acc.ID, payload.ID)
	assert.Equal(t, acc.OrganizationID, payload.OrganizationID)
	assert.Equal(t, acc.LedgerID, payload.LedgerID)

	// Required scalar.
	assert.Equal(t, "Treasury", payload.Name)

	// Nullable refs round-trip as nil pointers.
	assert.Nil(t, payload.PortfolioID)
	assert.Nil(t, payload.SegmentID)
	assert.Nil(t, payload.EntityID)
	assert.Nil(t, payload.Blocked)

	// Status nesting.
	assert.Equal(t, "ACTIVE", payload.Status.Code)
	assert.Nil(t, payload.Status.Description)

	// RFC3339 formatting locks producer-side timestamp discipline.
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

// TestNewAccountUpdated_MapsAllOptionalFields covers the path where every
// nullable field is set. Verifies that *string values are propagated
// (not stripped or empty-stringed) and that Blocked round-trips as a
// non-nil bool pointer.
func TestNewAccountUpdated_MapsAllOptionalFields(t *testing.T) {
	portfolioID := "01J7K8FN5W8R0R2S7Q1V4H6J01"
	segmentID := "01J7K8FN5W8R0R2S7Q1V4H6J02"
	entityID := "EXT-ACC-12345"
	statusDesc := "Active treasury account"
	blocked := true

	acc := minimalUpdatedAccount()
	acc.PortfolioID = &portfolioID
	acc.SegmentID = &segmentID
	acc.EntityID = &entityID
	acc.Status.Description = &statusDesc
	acc.Blocked = &blocked

	payload := events.NewAccountUpdated(acc)

	require.NotNil(t, payload.PortfolioID)
	assert.Equal(t, portfolioID, *payload.PortfolioID)

	require.NotNil(t, payload.SegmentID)
	assert.Equal(t, segmentID, *payload.SegmentID)

	require.NotNil(t, payload.EntityID)
	assert.Equal(t, entityID, *payload.EntityID)

	require.NotNil(t, payload.Status.Description)
	assert.Equal(t, statusDesc, *payload.Status.Description)

	require.NotNil(t, payload.Blocked)
	assert.True(t, *payload.Blocked)
}

// TestAccountUpdatedPayload_ToEvent_AssemblesStreamingEvent verifies the
// ToEvent helper composes a fully-populated streaming.Event matching the
// definition constants and the supplied tenantID/source/timestamp.
func TestAccountUpdatedPayload_ToEvent_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAccountUpdated(minimalUpdatedAccount())

	evt, err := payload.ToEvent("tenant-1", "lerian.midaz.ledger", fixedTime)
	require.NoError(t, err)

	// Routing — sourced from the package-level Definition.
	assert.Equal(t, events.AccountUpdatedDefinition.ResourceType, evt.ResourceType)
	assert.Equal(t, events.AccountUpdatedDefinition.EventType, evt.EventType)
	assert.Equal(t, events.AccountUpdatedDefinition.SchemaVersion, evt.SchemaVersion)

	// Per-emit fields.
	assert.Equal(t, "tenant-1", evt.TenantID)
	assert.Equal(t, "lerian.midaz.ledger", evt.Source)
	assert.Equal(t, payload.ID, evt.Subject)
	assert.Equal(t, fixedTime, evt.Timestamp)

	// Payload round-trips back to the same struct.
	var roundTrip events.AccountUpdatedPayload
	require.NoError(t, json.Unmarshal(evt.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestAccountUpdatedPayload_JSONShape locks the wire JSON layout against
// accidental field-name drift. Breaking this test is a wire-contract
// change; downstream consumers and the e2e mirror struct must be
// updated in the same PR.
func TestAccountUpdatedPayload_JSONShape(t *testing.T) {
	payload := events.NewAccountUpdated(minimalUpdatedAccount())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Re-unmarshal as a generic map so we can assert key presence
	// without coupling to struct tag ordering.
	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	// Required keys present.
	for _, key := range []string{
		"id", "organizationId", "ledgerId",
		"name",
		"portfolioId", "segmentId", "entityId",
		"status", "blocked",
		"updatedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	// Nested status object always has Code.
	status, ok := generic["status"].(map[string]any)
	require.True(t, ok, "status must serialize as an object")
	assert.Equal(t, "ACTIVE", status["code"])

	// Optional description is omitempty — absent on minimal account.
	_, hasDesc := status["description"]
	assert.False(t, hasDesc, "status.description must omitempty when nil")

	// Sanity: no field count surprises. Pin the count so additive drift
	// is caught here as well as in the strict e2e unmarshal.
	assert.Lenf(t, generic, 10, "expected 10 top-level fields, got %d (drift?)", len(generic))
}
