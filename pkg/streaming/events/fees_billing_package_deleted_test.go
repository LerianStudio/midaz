// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// billingPkgDeletedID is the deterministic aggregate ID used by the deleted
// event tests.
const billingPkgDeletedID = "01J7K8FN5W8R0R2S7Q1V4H6J0M"

// billingPkgDeletedOrgID / billingPkgDeletedLedgerID are the deterministic
// scope identifiers used by the deleted event tests.
const (
	billingPkgDeletedOrgID    = "01J7K7XB9C2D3E4F5G6H7J8K9L"
	billingPkgDeletedLedgerID = "01J7K9A1B2C3D4E5F6G7H8J9K0"
)

// billingPkgDeletedTime is the deterministic deletion timestamp.
var billingPkgDeletedTime = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

// TestFeesBillingPackageDeletedDefinition_Key locks the canonical event key.
func TestFeesBillingPackageDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "fees-billing-package.deleted", events.FeesBillingPackageDeletedDefinition.Key())
	assert.Equal(t, "fees-billing-package", events.FeesBillingPackageDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.FeesBillingPackageDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.FeesBillingPackageDeletedDefinition.SchemaVersion)
}

// TestNewFeesBillingPackageDeleted_Maps verifies the primitive mapping into the
// deleted payload including RFC3339 formatting of the deletion timestamp.
func TestNewFeesBillingPackageDeleted_Maps(t *testing.T) {
	payload := events.NewFeesBillingPackageDeleted(
		billingPkgDeletedID, billingPkgDeletedOrgID, billingPkgDeletedLedgerID, billingPkgDeletedTime)

	assert.Equal(t, billingPkgDeletedID, payload.ID)
	assert.Equal(t, billingPkgDeletedOrgID, payload.OrganizationID)
	assert.Equal(t, billingPkgDeletedLedgerID, payload.LedgerID)
	assert.Equal(t, "2026-06-01T00:00:00Z", payload.DeletedAt)
}

// TestFeesBillingPackageDeletedPayload_ToEmitRequest verifies the ToEmitRequest
// helper composes a fully-populated EmitRequest and round-trips the payload.
func TestFeesBillingPackageDeletedPayload_ToEmitRequest(t *testing.T) {
	payload := events.NewFeesBillingPackageDeleted(
		billingPkgDeletedID, billingPkgDeletedOrgID, billingPkgDeletedLedgerID, billingPkgDeletedTime)

	req, err := payload.ToEmitRequest("tenant-1", billingPkgDeletedTime)
	require.NoError(t, err)

	assert.Equal(t, events.FeesBillingPackageDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, billingPkgDeletedTime, req.Timestamp)

	var roundTrip events.FeesBillingPackageDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestFeesBillingPackageDeletedPayload_JSONShape locks the wire JSON layout and
// asserts every fee-detail / account / monetary field is ABSENT.
func TestFeesBillingPackageDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewFeesBillingPackageDeleted(
		billingPkgDeletedID, billingPkgDeletedOrgID, billingPkgDeletedLedgerID, billingPkgDeletedTime)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	expectedKeys := map[string]struct{}{
		"id":             {},
		"organizationId": {},
		"ledgerId":       {},
		"deletedAt":      {},
	}

	for key := range generic {
		_, ok := expectedKeys[key]
		assert.Truef(t, ok, "wire payload has unexpected top-level key %q (drift?)", key)
	}

	for key := range expectedKeys {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, forbidden := range []string{
		"label", "description", "assetCode", "feeAmount", "tiers",
		"discountTiers", "freeQuota", "eventFilter", "accountTarget",
		"debitAccountAlias", "creditAccountAlias", "maintenanceCreditAccount",
		"type", "pricingModel", "countMode", "enable", "createdAt", "updatedAt",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include excluded key %q", forbidden)
	}

	assert.Lenf(t, generic, 4, "expected 4 top-level fields, got %d (drift?)", len(generic))
}
