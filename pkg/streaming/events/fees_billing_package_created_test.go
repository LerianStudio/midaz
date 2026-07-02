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
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// billingPkgFixedRFC3339 is the deterministic RFC3339 timestamp used across
// the billing-package event tests so string round-trips assert by exact match.
const billingPkgFixedRFC3339 = "2026-05-13T12:34:56Z"

// minimalBillingPackage returns the smallest model.BillingPackage that
// satisfies the fees-billing-package.created/updated contract: identity, scope,
// type, timestamps. Every optional pointer (Enable, PricingModel, CountMode) is
// left nil so tests can verify nullable-field and nil->false handling.
//
// Every fee-detail / account / monetary field is populated ON PURPOSE to prove
// the JSONShape test catches any leak onto the wire.
func minimalBillingPackage() *model.BillingPackage {
	description := "Charges per completed transaction route"
	assetCode := "BRL"
	debit := "account_fees_debit"
	credit := "account_fees_credit"
	maintCredit := "account_maintenance_credit"
	freeQuota := 100
	feeAmount := decimal.NewFromInt(50)

	return &model.BillingPackage{
		ID:             "01J7K8FN5W8R0R2S7Q1V4H6J0M",
		OrganizationID: "01J7K7XB9C2D3E4F5G6H7J8K9L",
		LedgerID:       "01J7K9A1B2C3D4E5F6G7H8J9K0",
		Type:           model.BillingPackageTypeVolume,
		CreatedAt:      billingPkgFixedRFC3339,
		UpdatedAt:      billingPkgFixedRFC3339,

		// Fee-detail surface populated to PROVE it never leaks onto the wire.
		Label:                    "Monthly Volume Billing",
		Description:              &description,
		AssetCode:                &assetCode,
		FeeAmount:                &feeAmount,
		Tiers:                    []model.PricingTier{{MinQuantity: 0, UnitPrice: decimal.NewFromInt(1)}},
		DiscountTiers:            []model.DiscountTier{{MinQuantity: 1000, DiscountPercentage: decimal.NewFromInt(10)}},
		FreeQuota:                &freeQuota,
		EventFilter:              &model.EventFilter{TransactionRoute: "payment_route", Status: "APPROVED"},
		AccountTarget:            &model.AccountTarget{Aliases: []string{"account_alpha"}},
		DebitAccountAlias:        &debit,
		CreditAccountAlias:       &credit,
		MaintenanceCreditAccount: &maintCredit,
	}
}

// TestFeesBillingPackageCreatedDefinition_Key locks the canonical event key.
func TestFeesBillingPackageCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "fees-billing-package.created", events.FeesBillingPackageCreatedDefinition.Key())
	assert.Equal(t, "fees-billing-package", events.FeesBillingPackageCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.FeesBillingPackageCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.FeesBillingPackageCreatedDefinition.SchemaVersion)
}

// TestNewFeesBillingPackageCreated_MapsMinimal verifies the happy-path mapping
// for the simplest package: nil PricingModel/CountMode, nil Enable -> false.
func TestNewFeesBillingPackageCreated_MapsMinimal(t *testing.T) {
	bp := minimalBillingPackage()

	payload := events.NewFeesBillingPackageCreated(bp)

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

// TestNewFeesBillingPackageCreated_MapsAllOptional covers the path where
// PricingModel and CountMode are set and Enable is true.
func TestNewFeesBillingPackageCreated_MapsAllOptional(t *testing.T) {
	pricingModel := model.PricingModelTiered
	countMode := model.CountModePerRoute
	enable := true

	bp := minimalBillingPackage()
	bp.PricingModel = &pricingModel
	bp.CountMode = &countMode
	bp.Enable = &enable

	payload := events.NewFeesBillingPackageCreated(bp)

	require.NotNil(t, payload.PricingModel)
	assert.Equal(t, "tiered", *payload.PricingModel)

	require.NotNil(t, payload.CountMode)
	assert.Equal(t, "perRoute", *payload.CountMode)

	assert.True(t, payload.Enable)
}

// TestFeesBillingPackageCreatedPayload_ToEmitRequest verifies the ToEmitRequest
// helper composes a fully-populated EmitRequest and round-trips the payload.
func TestFeesBillingPackageCreatedPayload_ToEmitRequest(t *testing.T) {
	payload := events.NewFeesBillingPackageCreated(minimalBillingPackage())
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
// asserts that every fee-detail / account / monetary field is ABSENT even
// though the fixture populates them.
func TestFeesBillingPackageCreatedPayload_JSONShape(t *testing.T) {
	payload := events.NewFeesBillingPackageCreated(minimalBillingPackage())

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
