// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFeesBillingPackageUpdatedDefinition_Key locks the canonical event key.
func TestFeesBillingPackageUpdatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "fees-billing-package.updated", events.FeesBillingPackageUpdatedDefinition.Key())
	assert.Equal(t, "fees-billing-package", events.FeesBillingPackageUpdatedDefinition.ResourceType)
	assert.Equal(t, "updated", events.FeesBillingPackageUpdatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.FeesBillingPackageUpdatedDefinition.SchemaVersion)
}

// TestNewFeesBillingPackageUpdated_MapsMinimal verifies the happy-path mapping
// for the simplest package: nil PricingModel/CountMode, nil Enable -> false.
func TestNewFeesBillingPackageUpdated_MapsMinimal(t *testing.T) {
	bp := minimalBillingPackage()

	payload := events.NewFeesBillingPackageUpdated(bp)

	assert.Equal(t, bp.ID, payload.ID)
	assert.Equal(t, bp.OrganizationID, payload.OrganizationID)
	assert.Equal(t, bp.LedgerID, payload.LedgerID)
	assert.Equal(t, "volume", payload.Type)

	assert.Nil(t, payload.PricingModel)
	assert.Nil(t, payload.CountMode)
	assert.False(t, payload.Enable)

	assert.Equal(t, billingPkgFixedRFC3339, payload.CreatedAt)
	assert.Equal(t, billingPkgFixedRFC3339, payload.UpdatedAt)
}

// TestNewFeesBillingPackageUpdated_MapsAllOptional covers the path where
// PricingModel and CountMode are set and Enable is true.
func TestNewFeesBillingPackageUpdated_MapsAllOptional(t *testing.T) {
	pricingModel := model.PricingModelFixed
	countMode := model.CountModePerAccount
	enable := true

	bp := minimalBillingPackage()
	bp.PricingModel = &pricingModel
	bp.CountMode = &countMode
	bp.Enable = &enable

	payload := events.NewFeesBillingPackageUpdated(bp)

	require.NotNil(t, payload.PricingModel)
	assert.Equal(t, "fixed", *payload.PricingModel)

	require.NotNil(t, payload.CountMode)
	assert.Equal(t, "perAccount", *payload.CountMode)

	assert.True(t, payload.Enable)
}

// TestFeesBillingPackageUpdatedPayload_ToEmitRequest verifies the ToEmitRequest
// helper composes a fully-populated EmitRequest and round-trips the payload.
func TestFeesBillingPackageUpdatedPayload_ToEmitRequest(t *testing.T) {
	payload := events.NewFeesBillingPackageUpdated(minimalBillingPackage())
	ts := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)

	req, err := payload.ToEmitRequest("tenant-1", ts)
	require.NoError(t, err)

	assert.Equal(t, events.FeesBillingPackageUpdatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, ts, req.Timestamp)

	var roundTrip events.FeesBillingPackageUpdatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestFeesBillingPackageUpdatedPayload_JSONShape locks the wire JSON layout and
// asserts that every fee-detail / account / monetary field is ABSENT even
// though the fixture populates them.
func TestFeesBillingPackageUpdatedPayload_JSONShape(t *testing.T) {
	payload := events.NewFeesBillingPackageUpdated(minimalBillingPackage())

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
