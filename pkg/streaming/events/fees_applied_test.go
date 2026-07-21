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

// feesAppliedTransactionID is the deterministic aggregate ID reused across
// fee-charge.applied tests so Subject assertions are exact-match.
const feesAppliedTransactionID = "0190d9e1-7c2a-7000-8000-0000000000a1"

// feesAppliedOrgID is the deterministic organization scope for fee-charge.applied
// event tests.
const feesAppliedOrgID = "0190d9e1-7c2a-7000-8000-0000000000a3"

// feesAppliedLedgerID is the deterministic ledger scope for fee-charge.applied event
// tests.
const feesAppliedLedgerID = "0190d9e1-7c2a-7000-8000-0000000000a2"

// feesAppliedPackageID is the deterministic fee package reference for
// fee-charge.applied event tests.
const feesAppliedPackageID = "0190d9e1-7c2a-7000-8000-0000000000a4"

func TestFeesAppliedDefinition_Key(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "fee-charge.applied", events.FeesAppliedDefinition.Key())
	assert.Equal(t, "fee-charge", events.FeesAppliedDefinition.ResourceType)
	assert.Equal(t, "applied", events.FeesAppliedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.FeesAppliedDefinition.SchemaVersion)
}

func TestNewFeesApplied_MapsMinimalPayload(t *testing.T) {
	t.Parallel()

	payload := events.NewFeesApplied(
		feesAppliedTransactionID, feesAppliedOrgID, feesAppliedLedgerID,
		feesAppliedPackageID, fixedTime,
	)

	assert.Equal(t, feesAppliedTransactionID, payload.TransactionID)
	assert.Equal(t, feesAppliedOrgID, payload.OrganizationID)
	assert.Equal(t, feesAppliedLedgerID, payload.LedgerID)
	assert.Equal(t, feesAppliedPackageID, payload.FeePackageID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.AppliedAt)
}

func TestFeesAppliedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	t.Parallel()

	payload := events.NewFeesApplied(
		feesAppliedTransactionID, feesAppliedOrgID, feesAppliedLedgerID,
		feesAppliedPackageID, fixedTime,
	)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.FeesAppliedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, feesAppliedTransactionID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.FeesAppliedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestFeesAppliedPayload_JSONShape locks the wire JSON layout against field
// drift AND proves no sensitive/monetary/alias surface ever crosses the wire.
func TestFeesAppliedPayload_JSONShape(t *testing.T) {
	t.Parallel()

	payload := events.NewFeesApplied(
		feesAppliedTransactionID, feesAppliedOrgID, feesAppliedLedgerID,
		feesAppliedPackageID, fixedTime,
	)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"transactionId", "organizationId", "ledgerId", "feePackageId", "appliedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, forbidden := range []string{
		"amount", "assetCode", "source", "destination", "metadata",
		"operations", "description", "fees", "waivedAccounts",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include excluded key %q", forbidden)
	}

	assert.Lenf(t, generic, 5, "expected 5 top-level fields, got %d (drift?)", len(generic))
}
