// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// fixedLimitUUID is a deterministic UUID used across limit event tests so
// subject/payload assertions match by exact value.
var fixedLimitUUID = uuid.MustParse("018f5e2a-1c3d-7a4b-9e6f-0a1b2c3daaaa")

// limitFenceName/Description are deliberately populated on fixtures so tests
// prove the fence keeps free text off the wire.
const (
	limitFenceName        = "Daily USD spending cap"
	limitFenceDescription = "Caps daily USD outflows for retail accounts"
)

// mustTimeOfDay builds a TimeOfDay from "HH:MM" for test fixtures, failing the
// test if the format is invalid.
func mustTimeOfDay(t *testing.T, s string) model.TimeOfDay {
	t.Helper()

	tod, err := model.NewTimeOfDay(s)
	require.NoError(t, err)

	return tod
}

// minimalLimit returns the smallest Limit that satisfies the limit.created
// contract with all nullable time fields nil and no scopes. The fenced fields
// (Name, Description, MaxAmount) are populated on purpose to prove they never
// leak onto the wire.
func minimalLimit() *model.Limit {
	desc := limitFenceDescription

	return &model.Limit{
		ID:          fixedLimitUUID,
		Name:        limitFenceName,
		Description: &desc,
		LimitType:   model.LimitTypeDaily,
		MaxAmount:   decimal.RequireFromString("1000.00"),
		Currency:    "USD",
		Scopes:      nil,
		Status:      model.LimitStatusDraft,
		CreatedAt:   fixedTime,
		UpdatedAt:   fixedTime,
	}
}

// fullLimit returns a Limit with every optional field populated so tests prove
// the nullable keys serialize their values (not null) and scopes map through.
func fullLimit(t *testing.T) *model.Limit {
	t.Helper()

	limit := minimalLimit()
	limit.LimitType = model.LimitTypeCustom
	limit.Status = model.LimitStatusActive

	start := mustTimeOfDay(t, "09:00")
	end := mustTimeOfDay(t, "17:00")
	limit.ActiveTimeStart = &start
	limit.ActiveTimeEnd = &end

	customStart := fixedTime
	customEnd := fixedTime
	resetAt := fixedTime
	limit.CustomStartDate = &customStart
	limit.CustomEndDate = &customEnd
	limit.ResetAt = &resetAt

	txType := model.TransactionTypeCard
	limit.Scopes = []model.Scope{{SegmentID: &scopeSegmentID, TransactionType: &txType}}

	return limit
}

func TestLimitCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "limit.created", events.LimitCreatedDefinition.Key())
	assert.Equal(t, "limit", events.LimitCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.LimitCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.LimitCreatedDefinition.SchemaVersion)
}

func TestNewLimitCreated_MapsMinimalLimit(t *testing.T) {
	payload := events.NewLimitCreated(minimalLimit())

	assert.Equal(t, fixedLimitUUID.String(), payload.ID)
	assert.Equal(t, "DRAFT", payload.Status)
	assert.Equal(t, "DAILY", payload.LimitType)
	assert.Equal(t, "USD", payload.Currency)
	require.NotNil(t, payload.Scopes)
	assert.Len(t, payload.Scopes, 0)
	assert.Nil(t, payload.ActiveTimeStart)
	assert.Nil(t, payload.ActiveTimeEnd)
	assert.Nil(t, payload.CustomStartDate)
	assert.Nil(t, payload.CustomEndDate)
	assert.Nil(t, payload.ResetAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewLimitCreated_MapsAllOptionalFields(t *testing.T) {
	payload := events.NewLimitCreated(fullLimit(t))

	assert.Equal(t, "ACTIVE", payload.Status)
	assert.Equal(t, "CUSTOM", payload.LimitType)

	require.NotNil(t, payload.ActiveTimeStart)
	assert.Equal(t, "09:00", *payload.ActiveTimeStart)
	require.NotNil(t, payload.ActiveTimeEnd)
	assert.Equal(t, "17:00", *payload.ActiveTimeEnd)

	require.NotNil(t, payload.CustomStartDate)
	assert.Equal(t, "2026-05-13T12:34:56Z", *payload.CustomStartDate)
	require.NotNil(t, payload.CustomEndDate)
	assert.Equal(t, "2026-05-13T12:34:56Z", *payload.CustomEndDate)
	require.NotNil(t, payload.ResetAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", *payload.ResetAt)

	require.Len(t, payload.Scopes, 1)
	require.NotNil(t, payload.Scopes[0].SegmentID)
	assert.Equal(t, scopeSegmentID.String(), *payload.Scopes[0].SegmentID)
	require.NotNil(t, payload.Scopes[0].TransactionType)
	assert.Equal(t, "CARD", *payload.Scopes[0].TransactionType)
}

func TestLimitCreatedPayload_ToEmitRequest(t *testing.T) {
	payload := events.NewLimitCreated(fullLimit(t))

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.LimitCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.LimitCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload, roundTrip)
}

func TestLimitCreatedPayload_JSONShape(t *testing.T) {
	// fullLimit populates Name/Description/MaxAmount on purpose to prove the
	// fence keeps them off the wire.
	data, err := json.Marshal(events.NewLimitCreated(fullLimit(t)))
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	expectedKeys := map[string]struct{}{
		"id":              {},
		"status":          {},
		"limitType":       {},
		"currency":        {},
		"scopes":          {},
		"activeTimeStart": {},
		"activeTimeEnd":   {},
		"customStartDate": {},
		"customEndDate":   {},
		"resetAt":         {},
		"createdAt":       {},
		"updatedAt":       {},
	}

	for key := range generic {
		_, ok := expectedKeys[key]
		assert.Truef(t, ok, "wire payload has unexpected top-level key %q (drift?)", key)
	}

	for key := range expectedKeys {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	// Fence: free text and the financial value MUST NOT appear.
	for _, forbidden := range []string{"name", "description", "maxAmount"} {
		_, present := generic[forbidden]
		assert.Falsef(t, present, "fenced field %q must NOT appear on the wire", forbidden)
	}

	assert.Lenf(t, generic, 12, "expected 12 top-level fields, got %d (drift?)", len(generic))
}

func TestLimitCreatedPayload_JSONShape_NullableKeysPresentWhenUnset(t *testing.T) {
	data, err := json.Marshal(events.NewLimitCreated(minimalLimit()))
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	// Nullable keys must be present with a null value (never omitempty).
	for _, key := range []string{"activeTimeStart", "activeTimeEnd", "customStartDate", "customEndDate", "resetAt"} {
		val, present := generic[key]
		require.Truef(t, present, "%q key must be present even when unset", key)
		assert.Nilf(t, val, "%q must serialize null when unset", key)
	}

	// scopes must serialize as [] (non-nil), not null.
	scopes, present := generic["scopes"]
	require.True(t, present)
	assert.NotNil(t, scopes, "scopes must serialize as [] not null")
}
