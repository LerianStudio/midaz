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

func TestFeesPackageDeletedDefinition_Key(t *testing.T) {
	assert.Equal(t, "fee-packages.deleted", events.FeesPackageDeletedDefinition.Key())
	assert.Equal(t, "fee-packages", events.FeesPackageDeletedDefinition.ResourceType)
	assert.Equal(t, "deleted", events.FeesPackageDeletedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.FeesPackageDeletedDefinition.SchemaVersion)
}

func TestNewFeesPackageDeleted_MapsMinimalPackage(t *testing.T) {
	payload := events.NewFeesPackageDeleted(
		feesPackageID, feesPackageOrgID, feesPackageLedgerID, fixedTime,
	)

	assert.Equal(t, feesPackageID, payload.ID)
	assert.Equal(t, feesPackageOrgID, payload.OrganizationID)
	assert.Equal(t, feesPackageLedgerID, payload.LedgerID)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.DeletedAt)
}

func TestFeesPackageDeletedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewFeesPackageDeleted(
		feesPackageID, feesPackageOrgID, feesPackageLedgerID, fixedTime,
	)

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.FeesPackageDeletedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.FeesPackageDeletedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

// TestFeesPackageDeletedPayload_JSONShape locks the wire JSON layout and proves
// the excluded fee-detail/label surface never crosses the wire.
func TestFeesPackageDeletedPayload_JSONShape(t *testing.T) {
	payload := events.NewFeesPackageDeleted(
		feesPackageID, feesPackageOrgID, feesPackageLedgerID, fixedTime,
	)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"id", "organizationId", "ledgerId", "deletedAt"} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	for _, forbidden := range []string{
		"feeGroupLabel", "description", "minimumAmount",
		"maximumAmount", "fees", "waivedAccounts",
		"segmentId", "transactionRoute", "enable",
	} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "wire payload must NOT include excluded key %q", forbidden)
	}

	assert.Lenf(t, generic, 4, "expected 4 top-level fields, got %d (drift?)", len(generic))
}
