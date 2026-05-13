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

// minimalDeletedAccount returns the smallest mmodel.Account suitable as
// the pre-delete record consumed by NewAccountDeleted. PortfolioID is
// left nil; the test that exercises portfolio routing sets it
// explicitly.
func minimalDeletedAccount() *mmodel.Account {
	return &mmodel.Account{
		ID:             "01J7K8FN5W8R0R2S7Q1V4H6J0M",
		OrganizationID: "01J7K7XB9C2D3E4F5G6H7J8K9L",
		LedgerID:       "01J7K9A1B2C3D4E5F6G7H8J9K0",
		AssetCode:      "BRL",
		Type:           "deposit",
		Status:         mmodel.Status{Code: "ACTIVE"},
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
}

// TestAccountDeletedDefinition_Key locks the canonical event key.
// Changing this assertion is a wire-contract change and requires a
// coordinated update of every downstream consumer.
func TestAccountDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "account.deleted", events.AccountDeletedDefinition.Key())
	assert.Equal(t, "account", events.AccountDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.AccountDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AccountDeletedDefinition.SchemaVersion)
}

// TestNewAccountDeleted_MapsAccountWithoutPortfolio verifies the
// happy-path mapping for the simplest pre-delete record: identity only,
// no portfolio scope.
func TestNewAccountDeleted_MapsAccountWithoutPortfolio(t *testing.T) {
	acc := minimalDeletedAccount()

	payload := events.NewAccountDeleted(acc, fixedTime)

	// Identity.
	assert.Equal(t, acc.ID, payload.ID)
	assert.Equal(t, acc.OrganizationID, payload.OrganizationID)
	assert.Equal(t, acc.LedgerID, payload.LedgerID)

	// Nullable ref round-trips as nil pointer.
	assert.Nil(t, payload.PortfolioID)

	// RFC3339 formatting locks producer-side timestamp discipline.
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

// TestNewAccountDeleted_MapsAccountWithPortfolio verifies that a
// portfolio-scoped account propagates its PortfolioID into the payload
// as a non-nil pointer.
func TestNewAccountDeleted_MapsAccountWithPortfolio(t *testing.T) {
	portfolioID := "01J7K8FN5W8R0R2S7Q1V4H6J01"

	acc := minimalDeletedAccount()
	acc.PortfolioID = &portfolioID

	payload := events.NewAccountDeleted(acc, fixedTime)

	require.NotNil(t, payload.PortfolioID)
	assert.Equal(t, portfolioID, *payload.PortfolioID)
}

// TestAccountDeletedPayload_ToEvent_AssemblesStreamingEvent verifies the
// ToEvent helper composes a fully-populated streaming.Event matching the
// definition constants and the supplied tenantID/source/timestamp.
func TestAccountDeletedPayload_ToEvent_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAccountDeleted(minimalDeletedAccount(), fixedTime)

	evt, err := payload.ToEvent("tenant-1", "lerian.midaz.ledger", fixedTime)
	require.NoError(t, err)

	// Routing — sourced from the package-level Definition.
	assert.Equal(t, events.AccountDeletedDefinition.ResourceType, evt.ResourceType)
	assert.Equal(t, events.AccountDeletedDefinition.EventType, evt.EventType)
	assert.Equal(t, events.AccountDeletedDefinition.SchemaVersion, evt.SchemaVersion)

	// Per-emit fields.
	assert.Equal(t, "tenant-1", evt.TenantID)
	assert.Equal(t, "lerian.midaz.ledger", evt.Source)
	assert.Equal(t, payload.ID, evt.Subject)
	assert.Equal(t, fixedTime, evt.Timestamp)

	// Payload round-trips back to the same struct.
	var roundTrip events.AccountDeletedPayload
	require.NoError(t, json.Unmarshal(evt.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestAccountDeletedPayload_JSONShape locks the wire JSON layout against
// accidental field-name drift. Breaking this test is a wire-contract
// change; downstream consumers and the e2e mirror struct must be
// updated in the same PR.
func TestAccountDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewAccountDeleted(minimalDeletedAccount(), fixedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	// Required keys present.
	for _, key := range []string{
		"id", "organizationId", "ledgerId",
		"portfolioId", "deletedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	// Sanity: no field count surprises. Pin the count so additive drift
	// is caught here as well as in the strict e2e unmarshal.
	assert.Lenf(t, generic, 5, "expected 5 top-level fields, got %d (drift?)", len(generic))
}
