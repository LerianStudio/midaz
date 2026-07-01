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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aliasFixedTime is the deterministic timestamp used by the alias event tests
// so RFC3339 round-trips can be asserted by exact match.
var aliasFixedTime = time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)

// aliasFixedID is a deterministic UUID reused across alias tests so the
// Subject/ID assertions are exact-match.
var aliasFixedID = uuid.MustParse("0190d9e1-7c2a-7000-8000-0000000000a1")

// aliasHolderID is the deterministic holder scope for the alias tests.
var aliasHolderID = uuid.MustParse("0190d9e1-7c2a-7000-8000-0000000000b2")

// relatedPartyOneID / relatedPartyTwoID are deterministic related-party UUIDs.
var (
	relatedPartyOneID = uuid.MustParse("0190d9e1-7c2a-7000-8000-0000000000c3")
	relatedPartyTwoID = uuid.MustParse("0190d9e1-7c2a-7000-8000-0000000000d4")
)

// aliasTestOrgID is the deterministic organization scope. Alias carries no
// organization scope on the domain model, so it is supplied to the constructor
// explicitly.
const aliasTestOrgID = "01J7K7XB9C2D3E4F5G6H7J8K9L"

// minimalAlias returns the smallest mmodel.Alias suitable for the
// alias.created / alias.updated contract: identity, holder/ledger/account
// references, classification type, and timestamps. RelatedParties is left nil
// so tests can verify the empty-collection path. PII/banking fields (document,
// bankingDetails, regulatoryFields) are populated to PROVE they never reach the
// wire payload.
func minimalAlias() *mmodel.Alias {
	id := aliasFixedID
	holderID := aliasHolderID
	aliasType := "LEGAL_PERSON"
	ledgerID := "01J7K7XB9C2D3E4F5G6H7LEDGR"
	accountID := "01J7K7XB9C2D3E4F5G6H7ACCNT"
	document := "91315026015"
	iban := "US12345678901234567890"
	branch := "0001"
	account := "123450"
	participantDoc := "12345678912345"

	return &mmodel.Alias{
		ID:        &id,
		HolderID:  &holderID,
		Type:      &aliasType,
		LedgerID:  &ledgerID,
		AccountID: &accountID,
		Document:  &document,
		BankingDetails: &mmodel.BankingDetails{
			Branch:  &branch,
			Account: &account,
			IBAN:    &iban,
		},
		RegulatoryFields: &mmodel.RegulatoryFields{
			ParticipantDocument: &participantDoc,
		},
		CreatedAt: aliasFixedTime,
		UpdatedAt: aliasFixedTime,
	}
}

// aliasWithRelatedParties returns a minimalAlias with two related parties, each
// seeded with PII (document/name) and relationship dates to PROVE only the ID
// and non-PII role cross the wire.
func aliasWithRelatedParties() *mmodel.Alias {
	a := minimalAlias()

	rpOne := relatedPartyOneID
	rpTwo := relatedPartyTwoID

	a.RelatedParties = []*mmodel.RelatedParty{
		{
			ID:        &rpOne,
			Document:  "11122233344",
			Name:      "Jane Roe",
			Role:      "PRIMARY_HOLDER",
			StartDate: mmodel.Date{Time: aliasFixedTime},
		},
		{
			ID:        &rpTwo,
			Document:  "55566677788",
			Name:      "John Smith",
			Role:      "LEGAL_REPRESENTATIVE",
			StartDate: mmodel.Date{Time: aliasFixedTime},
		},
	}

	return a
}

// TestAliasCreatedDefinition_Key locks the canonical event key. Changing this
// assertion is a wire-contract change and requires a coordinated update of
// every downstream consumer.
func TestAliasCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "alias.created", events.AliasCreatedDefinition.Key())
	assert.Equal(t, "alias", events.AliasCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.AliasCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.AliasCreatedDefinition.SchemaVersion)
}

// TestNewAliasCreated_MapsMinimalAlias verifies the happy-path mapping for the
// simplest alias: identity, references, type, timestamps, and an empty
// relatedParties collection.
func TestNewAliasCreated_MapsMinimalAlias(t *testing.T) {
	a := minimalAlias()

	payload := events.NewAliasCreated(a, aliasTestOrgID)

	assert.Equal(t, aliasFixedID.String(), payload.ID)
	assert.Equal(t, aliasHolderID.String(), payload.HolderID)
	assert.Equal(t, aliasTestOrgID, payload.OrganizationID)
	assert.Equal(t, "01J7K7XB9C2D3E4F5G6H7LEDGR", payload.LedgerID)
	assert.Equal(t, "01J7K7XB9C2D3E4F5G6H7ACCNT", payload.AccountID)
	assert.Equal(t, "LEGAL_PERSON", payload.Type)

	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)

	// No related parties on the minimal alias -> nil slice.
	assert.Nil(t, payload.RelatedParties)
}

// TestNewAliasCreated_MapsRelatedParties covers the multi-entry path and PROVES
// only relatedPartyId + role cross the wire (PII document/name/dates dropped).
func TestNewAliasCreated_MapsRelatedParties(t *testing.T) {
	a := aliasWithRelatedParties()

	payload := events.NewAliasCreated(a, aliasTestOrgID)

	require.Len(t, payload.RelatedParties, 2)

	assert.Equal(t, relatedPartyOneID.String(), payload.RelatedParties[0].RelatedPartyID)
	assert.Equal(t, "PRIMARY_HOLDER", payload.RelatedParties[0].Role)

	assert.Equal(t, relatedPartyTwoID.String(), payload.RelatedParties[1].RelatedPartyID)
	assert.Equal(t, "LEGAL_REPRESENTATIVE", payload.RelatedParties[1].Role)
}

// TestNewAliasCreated_NilRelatedPartyIDIsSafe verifies a related party with a
// nil ID maps to an empty string rather than panicking, and a nil entry in the
// slice is skipped.
func TestNewAliasCreated_NilRelatedPartyIDIsSafe(t *testing.T) {
	a := minimalAlias()
	a.RelatedParties = []*mmodel.RelatedParty{
		nil,
		{ID: nil, Role: "RESPONSIBLE_PARTY"},
	}

	payload := events.NewAliasCreated(a, aliasTestOrgID)

	require.Len(t, payload.RelatedParties, 1)
	assert.Equal(t, "", payload.RelatedParties[0].RelatedPartyID)
	assert.Equal(t, "RESPONSIBLE_PARTY", payload.RelatedParties[0].Role)
}

// TestAliasCreatedPayload_ToEmitRequest_AssemblesStreamingEvent verifies the
// ToEmitRequest helper composes a fully-populated EmitRequest.
func TestAliasCreatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewAliasCreated(aliasWithRelatedParties(), aliasTestOrgID)

	req, err := payload.ToEmitRequest("tenant-1", aliasFixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.AliasCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, aliasFixedTime, req.Timestamp)

	var roundTrip events.AliasCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestAliasCreatedPayload_JSONShape_NilRelatedPartiesIsNull locks the intended
// wire contract for an alias with no related parties: relatedParties MUST encode
// as JSON null, never as an empty array. mapAliasRelatedParties returns a nil
// slice for empty input and the field carries no omitempty; a future switch to a
// non-nil empty slice would silently flip the wire to [] and break consumers
// that distinguish absent from empty.
func TestAliasCreatedPayload_JSONShape_NilRelatedPartiesIsNull(t *testing.T) {
	payload := events.NewAliasCreated(minimalAlias(), aliasTestOrgID)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &generic))

	raw, present := generic["relatedParties"]
	require.Truef(t, present, "wire payload must include %q", "relatedParties")

	assert.JSONEq(t, "null", string(raw), "empty relatedParties must be JSON null, not []")
	assert.NotEqual(t, "[]", string(raw), "empty relatedParties must NOT be an empty array")

	// Round-trip the minimal (nil-slice) payload to prove marshal->unmarshal
	// preserves equality, mirroring the populated-slice round-trip.
	var roundTrip events.AliasCreatedPayload
	require.NoError(t, json.Unmarshal(data, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestAliasCreatedPayload_JSONShape locks the wire JSON layout against
// accidental field-name drift AND asserts that no PII/banking key ever appears
// on the wire, at both the top level and inside each relatedParties element.
func TestAliasCreatedPayload_JSONShape(t *testing.T) {
	payload := events.NewAliasCreated(aliasWithRelatedParties(), aliasTestOrgID)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	// Required top-level keys present.
	for _, key := range []string{
		"id", "holderId", "organizationId", "ledgerId", "accountId",
		"type", "createdAt", "updatedAt", "relatedParties",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	// PII / banking / non-contract keys must NEVER reach the wire.
	for _, forbidden := range []string{
		"externalId", "document", "cpf", "cnpj", "name",
		"bankingDetails", "iban", "branch", "account",
		"regulatoryFields", "participantDocument", "metadata", "deletedAt",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include key %q", forbidden)
	}

	// Pin the top-level field count so additive drift is caught here.
	assert.Lenf(t, generic, 9, "expected 9 top-level fields, got %d (drift?)", len(generic))

	// Descend into the relatedParties element and lock its shape.
	rawParties, ok := generic["relatedParties"].([]any)
	require.Truef(t, ok, "relatedParties must be a JSON array")
	require.NotEmpty(t, rawParties)

	elem, ok := rawParties[0].(map[string]any)
	require.Truef(t, ok, "relatedParties element must be a JSON object")

	for _, key := range []string{"relatedPartyId", "role"} {
		_, present := elem[key]
		assert.Truef(t, present, "relatedParties element must include %q", key)
	}

	for _, forbidden := range []string{
		"id", "document", "name", "startDate", "endDate",
	} {
		_, present := elem[forbidden]
		assert.Falsef(t, present, "relatedParties element must NOT include key %q", forbidden)
	}

	assert.Lenf(t, elem, 2, "expected 2 relatedParties element fields, got %d (drift?)", len(elem))
}
