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

// feesPackageID is the deterministic aggregate ID reused across fees-package
// tests so Subject/ID assertions are exact-match.
const feesPackageID = "0190d9e1-7c2a-7000-8000-0000000000f1"

// feesPackageOrgID is the deterministic organization scope for fees-package
// event tests.
const feesPackageOrgID = "0190d9e1-7c2a-7000-8000-0000000000f3"

// feesPackageLedgerID is the deterministic ledger scope for fees-package event
// tests.
const feesPackageLedgerID = "0190d9e1-7c2a-7000-8000-0000000000f2"

func TestFeesPackageCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "fee-packages.created", events.FeesPackageCreatedDefinition.Key())
	assert.Equal(t, "fee-packages", events.FeesPackageCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.FeesPackageCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.FeesPackageCreatedDefinition.SchemaVersion)
}

func TestNewFeesPackageCreated_MapsMinimalPackage(t *testing.T) {
	payload := events.NewFeesPackageCreated(
		feesPackageID, feesPackageOrgID, feesPackageLedgerID,
		nil, nil, true, fixedTime, fixedTime,
	)

	assert.Equal(t, feesPackageID, payload.ID)
	assert.Equal(t, feesPackageOrgID, payload.OrganizationID)
	assert.Equal(t, feesPackageLedgerID, payload.LedgerID)
	assert.Nil(t, payload.SegmentID)
	assert.Nil(t, payload.TransactionRoute)
	assert.True(t, payload.Enable)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewFeesPackageCreated_MapsAllOptionalFields(t *testing.T) {
	segmentID := "0190d9e1-7c2a-7000-8000-0000000000f4"
	route := "debitoted"

	payload := events.NewFeesPackageCreated(
		feesPackageID, feesPackageOrgID, feesPackageLedgerID,
		&segmentID, &route, false, fixedTime, fixedTime,
	)

	require.NotNil(t, payload.SegmentID)
	assert.Equal(t, segmentID, *payload.SegmentID)
	require.NotNil(t, payload.TransactionRoute)
	assert.Equal(t, route, *payload.TransactionRoute)
	assert.False(t, payload.Enable)
}

func TestFeesPackageCreatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewFeesPackageCreated(
		feesPackageID, feesPackageOrgID, feesPackageLedgerID,
		nil, nil, true, fixedTime, fixedTime,
	)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.FeesPackageCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.FeesPackageCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestFeesPackageCreatedPayload_JSONShape locks the wire JSON layout against
// field drift AND proves the excluded fee-detail/label surface never crosses
// the wire.
func TestFeesPackageCreatedPayload_JSONShape(t *testing.T) {
	segmentID := "0190d9e1-7c2a-7000-8000-0000000000f4"
	route := "debitoted"

	payload := events.NewFeesPackageCreated(
		feesPackageID, feesPackageOrgID, feesPackageLedgerID,
		&segmentID, &route, true, fixedTime, fixedTime,
	)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"id", "organizationId", "ledgerId", "segmentId",
		"transactionRoute", "enable", "createdAt", "updatedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, forbidden := range []string{
		"feeGroupLabel", "description", "minimumAmount",
		"maximumAmount", "fees", "waivedAccounts",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include excluded key %q", forbidden)
	}

	assert.Lenf(t, generic, 8, "expected 8 top-level fields, got %d (drift?)", len(generic))
}
