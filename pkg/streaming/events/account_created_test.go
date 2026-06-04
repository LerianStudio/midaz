// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedTime is the deterministic timestamp used by every test in this
// file so RFC3339 round-trips can be asserted by exact match.
var fixedTime = time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)

// minimalAccount returns the smallest mmodel.Account that satisfies the
// account.created contract: required identity, name, assetCode, type,
// status, timestamps. Every optional pointer is left nil so tests can
// verify nullable-field handling without further setup.
func minimalAccount() *mmodel.Account {
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

// TestAccountCreatedDefinition_Key locks the canonical event key.
// Changing this assertion is a wire-contract change and requires a
// coordinated update of every downstream consumer.
func TestAccountCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "account.created", events.AccountCreatedDefinition.Key())
	assert.Equal(t, "account", events.AccountCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.AccountCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AccountCreatedDefinition.SchemaVersion)
}

// TestNewAccountCreated_MapsMinimalAccount verifies the happy-path
// mapping from mmodel.Account into the wire payload for the simplest
// possible account: no optional references, ACTIVE status, no blocked
// flag, no portfolio/segment/parent.
func TestNewAccountCreated_MapsMinimalAccount(t *testing.T) {
	acc := minimalAccount()

	payload := events.NewAccountCreated(acc)

	// Identity.
	assert.Equal(t, acc.ID, payload.ID)
	assert.Equal(t, acc.OrganizationID, payload.OrganizationID)
	assert.Equal(t, acc.LedgerID, payload.LedgerID)

	// Required scalars.
	assert.Equal(t, "Treasury", payload.Name)
	assert.Equal(t, "BRL", payload.AssetCode)
	assert.Equal(t, "deposit", payload.Type)

	// Nullable refs round-trip as nil pointers.
	assert.Nil(t, payload.PortfolioID)
	assert.Nil(t, payload.SegmentID)
	assert.Nil(t, payload.ParentAccountID)
	assert.Nil(t, payload.EntityID)
	assert.Nil(t, payload.Alias)
	assert.Nil(t, payload.Blocked)

	// Status nesting.
	assert.Equal(t, "ACTIVE", payload.Status.Code)
	assert.Nil(t, payload.Status.Description)

	// RFC3339 formatting locks producer-side timestamp discipline.
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

// TestNewAccountCreated_MapsAllOptionalFields covers the path where every
// nullable field is set. Verifies that *string values are propagated
// (not stripped or empty-stringed) and that Blocked round-trips as a
// non-nil bool pointer.
func TestNewAccountCreated_MapsAllOptionalFields(t *testing.T) {
	alias := "@treasury"
	portfolioID := "01J7K8FN5W8R0R2S7Q1V4H6J01"
	segmentID := "01J7K8FN5W8R0R2S7Q1V4H6J02"
	parentID := "01J7K8FN5W8R0R2S7Q1V4H6J03"
	entityID := "EXT-ACC-12345"
	statusDesc := "Active treasury account"
	blocked := false

	acc := minimalAccount()
	acc.Alias = &alias
	acc.PortfolioID = &portfolioID
	acc.SegmentID = &segmentID
	acc.ParentAccountID = &parentID
	acc.EntityID = &entityID
	acc.Status.Description = &statusDesc
	acc.Blocked = &blocked

	payload := events.NewAccountCreated(acc)

	require.NotNil(t, payload.Alias)
	assert.Equal(t, alias, *payload.Alias)

	require.NotNil(t, payload.PortfolioID)
	assert.Equal(t, portfolioID, *payload.PortfolioID)

	require.NotNil(t, payload.SegmentID)
	assert.Equal(t, segmentID, *payload.SegmentID)

	require.NotNil(t, payload.ParentAccountID)
	assert.Equal(t, parentID, *payload.ParentAccountID)

	require.NotNil(t, payload.EntityID)
	assert.Equal(t, entityID, *payload.EntityID)

	require.NotNil(t, payload.Status.Description)
	assert.Equal(t, statusDesc, *payload.Status.Description)

	require.NotNil(t, payload.Blocked)
	assert.False(t, *payload.Blocked)
}

// TestAccountCreatedPayload_ToEmitRequest_AssemblesStreamingEvent verifies
// the ToEmitRequest helper composes a fully-populated EmitRequest with the
// correct DefinitionKey, tenant ID, subject, timestamp, and payload.
//
// Source/ResourceType/EventType/SchemaVersion are not asserted here —
// they resolve from the Catalog at emit time via DefinitionKey, not from
// EmitRequest.
func TestAccountCreatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAccountCreated(minimalAccount())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	// Catalog routing key.
	assert.Equal(t, events.AccountCreatedDefinition.Key(), req.DefinitionKey)

	// Per-emit fields.
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	// Payload round-trips back to the same struct.
	var roundTrip events.AccountCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestAccountCreatedPayload_JSONShape locks the wire JSON layout against
// accidental field-name drift. Breaking this test is a wire-contract
// change; downstream consumers and the e2e mirror struct must be
// updated in the same PR.
func TestAccountCreatedPayload_JSONShape(t *testing.T) {
	payload := events.NewAccountCreated(minimalAccount())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Re-unmarshal as a generic map so we can assert key presence
	// without coupling to struct tag ordering.
	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	// Required keys present.
	for _, key := range []string{
		"id", "organizationId", "ledgerId",
		"name", "assetCode", "type",
		"portfolioId", "segmentId", "parentAccountId",
		"entityId", "holderId", "alias",
		"status", "blocked",
		"createdAt", "updatedAt",
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
	assert.Lenf(t, generic, 16, "expected 16 top-level fields, got %d (drift?)", len(generic))
}
