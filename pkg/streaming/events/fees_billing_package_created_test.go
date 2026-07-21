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

// Shared primitive fixtures for the billing-package event tests. Timestamps are
// deterministic so string round-trips assert by exact match.
const (
	billingPkgID           = "01J7K8FN5W8R0R2S7Q1V4H6J0M"
	billingPkgOrgID        = "01J7K7XB9C2D3E4F5G6H7J8K9L"
	billingPkgLedgerID     = "01J7K9A1B2C3D4E5F6G7H8J9K0"
	billingPkgType         = "volume"
	billingPkgFixedRFC3339 = "2026-05-13T12:34:56Z"
)

// TestFeesBillingPackageCreatedDefinition_Key locks the canonical event key.
func TestFeesBillingPackageCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "fees-billing-package.created", events.FeesBillingPackageCreatedDefinition.Key())
	assert.Equal(t, "fees-billing-package", events.FeesBillingPackageCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.FeesBillingPackageCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.FeesBillingPackageCreatedDefinition.SchemaVersion)
}

// TestNewFeesBillingPackageCreated_MapsMinimal verifies the happy-path mapping
// for the simplest package: nil PricingModel/CountMode, enable false.
func TestNewFeesBillingPackageCreated_MapsMinimal(t *testing.T) {
	payload := events.NewFeesBillingPackageCreated(
		billingPkgID, billingPkgOrgID, billingPkgLedgerID, billingPkgType,
		nil, nil, false, billingPkgFixedRFC3339, billingPkgFixedRFC3339,
	)

	assert.Equal(t, billingPkgID, payload.ID)
	assert.Equal(t, billingPkgOrgID, payload.OrganizationID)
	assert.Equal(t, billingPkgLedgerID, payload.LedgerID)
	assert.Equal(t, "volume", payload.Type)

	assert.Nil(t, payload.PricingModel)
	assert.Nil(t, payload.CountMode)
	assert.False(t, payload.Enable)

	assert.Equal(t, billingPkgFixedRFC3339, payload.CreatedAt)
	assert.Equal(t, billingPkgFixedRFC3339, payload.UpdatedAt)
}

// TestNewFeesBillingPackageCreated_MapsAllOptional covers the path where
// PricingModel and CountMode are set and Enable is true.
func TestNewFeesBillingPackageCreated_MapsAllOptional(t *testing.T) {
	pricingModel := "tiered"
	countMode := "perRoute"

	payload := events.NewFeesBillingPackageCreated(
		billingPkgID, billingPkgOrgID, billingPkgLedgerID, billingPkgType,
		&pricingModel, &countMode, true, billingPkgFixedRFC3339, billingPkgFixedRFC3339,
	)

	require.NotNil(t, payload.PricingModel)
	assert.Equal(t, "tiered", *payload.PricingModel)

	require.NotNil(t, payload.CountMode)
	assert.Equal(t, "perRoute", *payload.CountMode)

	assert.True(t, payload.Enable)
}

// TestFeesBillingPackageCreatedPayload_ToEmitRequest verifies the ToEmitRequest
// helper composes a fully-populated EmitRequest and round-trips the payload.
func TestFeesBillingPackageCreatedPayload_ToEmitRequest(t *testing.T) {
	payload := events.NewFeesBillingPackageCreated(
		billingPkgID, billingPkgOrgID, billingPkgLedgerID, billingPkgType,
		nil, nil, false, billingPkgFixedRFC3339, billingPkgFixedRFC3339,
	)
	ts := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)

	req, err := payload.ToEmitRequest("tenant-1", ts)
	require.NoError(t, err)

	assert.Equal(t, events.FeesBillingPackageCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, ts, req.Timestamp)

	var roundTrip events.FeesBillingPackageCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestFeesBillingPackageCreatedPayload_JSONShape locks the wire JSON layout and
// asserts that every fee-detail / account / monetary field is ABSENT.
func TestFeesBillingPackageCreatedPayload_JSONShape(t *testing.T) {
	payload := events.NewFeesBillingPackageCreated(
		billingPkgID, billingPkgOrgID, billingPkgLedgerID, billingPkgType,
		nil, nil, false, billingPkgFixedRFC3339, billingPkgFixedRFC3339,
	)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	expectedKeys := map[string]struct{}{
		"id":             {},
		"organizationId": {},
		"ledgerId":       {},
		"type":           {},
		"pricingModel":   {},
		"countMode":      {},
		"enable":         {},
		"createdAt":      {},
		"updatedAt":      {},
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
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include excluded key %q", forbidden)
	}

	assert.Lenf(t, generic, 9, "expected 9 top-level fields, got %d (drift?)", len(generic))
}
