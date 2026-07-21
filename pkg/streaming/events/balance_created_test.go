// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	balanceID      = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab10").String()
	balanceOrg     = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab11").String()
	balanceLed     = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab12").String()
	balanceAccount = uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab13").String()
)

func minimalBalance() *mmodel.Balance {
	return &mmodel.Balance{
		ID:             balanceID,
		OrganizationID: balanceOrg,
		LedgerID:       balanceLed,
		AccountID:      balanceAccount,
		Alias:          "@cash",
		Key:            "default",
		AssetCode:      "USD",
		AccountType:    "deposit",
		Available:      decimal.Zero,
		OnHold:         decimal.Zero,
		AllowSending:   true,
		AllowReceiving: true,
		Direction:      "credit",
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	}
}

func TestBalanceCreatedDefinition_Key(t *testing.T) {
	assert.Equal(t, "balance.created", events.BalanceCreatedDefinition.Key())
	assert.Equal(t, "balance", events.BalanceCreatedDefinition.ResourceType)
	assert.Equal(t, "created", events.BalanceCreatedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.BalanceCreatedDefinition.SchemaVersion)
}

func TestNewBalanceCreated_MapsMinimalBalance(t *testing.T) {
	b := minimalBalance()

	payload := events.NewBalanceCreated(b)

	assert.Equal(t, balanceID, payload.ID)
	assert.Equal(t, balanceOrg, payload.OrganizationID)
	assert.Equal(t, balanceLed, payload.LedgerID)
	assert.Equal(t, balanceAccount, payload.AccountID)
	assert.Equal(t, "@cash", payload.Alias)
	assert.Equal(t, "default", payload.Key)
	assert.Equal(t, "USD", payload.AssetCode)
	assert.Equal(t, "deposit", payload.AccountType)
	assert.True(t, payload.Available.IsZero())
	assert.True(t, payload.OnHold.IsZero())
	assert.True(t, payload.AllowSending)
	assert.True(t, payload.AllowReceiving)
	assert.Equal(t, "credit", payload.Direction)
	assert.Nil(t, payload.Settings)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.CreatedAt)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewBalanceCreated_MapsSettings(t *testing.T) {
	b := minimalBalance()
	limit := "1000"
	b.Settings = &mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
		BalanceScope:          "transactional",
	}

	payload := events.NewBalanceCreated(b)

	require.NotNil(t, payload.Settings)
	assert.True(t, payload.Settings.AllowOverdraft)
	assert.True(t, payload.Settings.OverdraftLimitEnabled)
	require.NotNil(t, payload.Settings.OverdraftLimit)
	assert.Equal(t, "1000", *payload.Settings.OverdraftLimit)
	assert.Equal(t, "transactional", payload.Settings.BalanceScope)
}

func TestBalanceCreatedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewBalanceCreated(minimalBalance())

	req, err := payload.ToEmitRequest("tenant-1", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.BalanceCreatedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-1", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.BalanceCreatedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload.ID, roundTrip.ID)
	assert.Equal(t, payload.OrganizationID, roundTrip.OrganizationID)
}

func TestBalanceCreatedPayload_JSONShape_MinimalIncludesRequiredFields(t *testing.T) {
	payload := events.NewBalanceCreated(minimalBalance())

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"id", "organizationId", "ledgerId", "accountId", "alias", "key",
		"assetCode", "accountType", "available", "onHold",
		"allowSending", "allowReceiving", "direction",
		"createdAt", "updatedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	// settings is omitempty when nil
	_, hasSettings := generic["settings"]
	assert.False(t, hasSettings, "settings must be omitted when nil")

	// scale is deliberately not on the wire (asset-level property)
	_, hasScale := generic["scale"]
	assert.False(t, hasScale, "scale is intentionally omitted from the wire payload")

	assert.Lenf(t, generic, 15, "expected 15 top-level fields with all the always-present fields, got %d (drift?)", len(generic))
}

func TestBalanceCreatedPayload_JSONShape_OmitsEmptyOptionals(t *testing.T) {
	b := minimalBalance()
	b.Alias = ""
	b.AccountType = ""
	b.Direction = ""

	payload := events.NewBalanceCreated(b)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{"alias", "accountType", "direction"} {
		_, has := generic[key]
		assert.Falsef(t, has, "%q must omitempty when empty", key)
	}
}
