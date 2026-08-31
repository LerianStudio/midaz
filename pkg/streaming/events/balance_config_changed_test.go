// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBalanceConfigChangedDefinition_Key(t *testing.T) {
	// EventType is the underscored canonical form (config_changed), so Key() is
	// balance.config_changed and reaches the wire that way. The payload
	// changeType discriminator (settings_updated / overdraft_enabled) is
	// separate payload data.
	assert.Equal(t, "balance.config_changed", events.BalanceConfigChangedDefinition.Key())
	assert.Equal(t, "balance", events.BalanceConfigChangedDefinition.ResourceType)
	assert.Equal(t, "config_changed", events.BalanceConfigChangedDefinition.EventType)
	assert.Equal(t, "1.0.0", events.BalanceConfigChangedDefinition.SchemaVersion)
}

func TestBalanceConfigChangeType_WireValues(t *testing.T) {
	assert.Equal(t, "settings_updated", events.BalanceConfigChangeTypeSettingsUpdated)
	assert.Equal(t, "overdraft_enabled", events.BalanceConfigChangeTypeOverdraftEnabled)
}

func TestNewBalanceConfigChanged_SettingsUpdatedBranch(t *testing.T) {
	b := minimalBalance()
	b.AllowSending = false
	b.AllowReceiving = true

	payload := events.NewBalanceConfigChanged(b, events.BalanceConfigChangeTypeSettingsUpdated)

	assert.Equal(t, balanceID, payload.ID)
	assert.Equal(t, balanceOrg, payload.OrganizationID)
	assert.Equal(t, balanceLed, payload.LedgerID)
	assert.Equal(t, balanceAccount, payload.AccountID)
	assert.Equal(t, "@cash", payload.Alias)
	assert.Equal(t, "default", payload.Key)
	assert.False(t, payload.AllowSending)
	assert.True(t, payload.AllowReceiving)
	assert.Equal(t, "credit", payload.Direction)
	assert.Equal(t, "settings_updated", payload.ChangeType)
	assert.Equal(t, "2026-05-13T12:34:56Z", payload.UpdatedAt)
}

func TestNewBalanceConfigChanged_OverdraftEnabledBranch_CarriesCompanionScope(t *testing.T) {
	// In the overdraft_enabled branch the payload reflects the COMPANION
	// balance's persisted state, including BalanceScope="internal" which
	// signals to consumers that this is the system-managed companion row.
	b := minimalBalance()
	b.Key = "overdraft"
	b.Direction = "debit"
	b.Settings = &mmodel.BalanceSettings{
		BalanceScope: mmodel.BalanceScopeInternal,
	}

	payload := events.NewBalanceConfigChanged(b, events.BalanceConfigChangeTypeOverdraftEnabled)

	assert.Equal(t, "overdraft", payload.Key)
	assert.Equal(t, "debit", payload.Direction)
	require.NotNil(t, payload.Settings)
	assert.Equal(t, mmodel.BalanceScopeInternal, payload.Settings.BalanceScope)
	assert.Equal(t, "overdraft_enabled", payload.ChangeType)
}

func TestBalanceConfigChangedPayload_ToEmitRequest_AssemblesStreamingEvent(t *testing.T) {
	payload := events.NewBalanceConfigChanged(minimalBalance(), events.BalanceConfigChangeTypeSettingsUpdated)

	req, err := payload.ToEmitRequest("tenant-7", fixedTime)
	require.NoError(t, err)

	assert.Equal(t, events.BalanceConfigChangedDefinition.Key(), req.DefinitionKey)
	assert.Equal(t, "tenant-7", req.TenantID)
	assert.Equal(t, payload.ID, req.Subject)
	assert.Equal(t, fixedTime, req.Timestamp)

	var roundTrip events.BalanceConfigChangedPayload
	require.NoError(t, json.Unmarshal(req.Payload, &roundTrip))
	assert.Equal(t, payload.ID, roundTrip.ID)
	assert.Equal(t, payload.ChangeType, roundTrip.ChangeType)
}

func TestBalanceConfigChangedPayload_JSONShape_MinimalIncludesRequiredFields(t *testing.T) {
	payload := events.NewBalanceConfigChanged(minimalBalance(), events.BalanceConfigChangeTypeSettingsUpdated)

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	for _, key := range []string{
		"id", "organizationId", "ledgerId", "accountId", "alias", "key",
		"allowSending", "allowReceiving", "direction",
		"changeType", "updatedAt",
	} {
		_, ok := generic[key]
		assert.Truef(t, ok, "wire payload must include %q", key)
	}

	// Money fields MUST NOT be on the wire for this event — it is a
	// configuration mutation signal, not a balance-movement signal.
	for _, key := range []string{"available", "onHold"} {
		_, has := generic[key]
		assert.Falsef(t, has, "%q must not appear on balance.config_changed (money movement is a separate event family)", key)
	}

	_, hasSettings := generic["settings"]
	assert.False(t, hasSettings, "settings is omitempty when nil")

	assert.Lenf(t, generic, 11, "expected 11 top-level fields, got %d (drift?)", len(generic))
}

// TestBalanceConfigChangedPayload_JSONShape_SettingsSubobjectLocked pins the
// nested settings object on balance.config_changed. BalanceSettingsPayload is
// declared in balance_created.go but reaches the wire through both events, so
// each event locks the subobject on its own side.
func TestBalanceConfigChangedPayload_JSONShape_SettingsSubobjectLocked(t *testing.T) {
	t.Run("full settings carries every key", func(t *testing.T) {
		b := minimalBalance()
		b.Settings = fullBalanceSettings()

		req, err := events.NewBalanceConfigChanged(b, events.BalanceConfigChangeTypeSettingsUpdated).
			ToEmitRequest("tenant-7", fixedTime)
		require.NoError(t, err)

		settings := settingsSubobject(t, req.Payload)

		assert.Lenf(t, settings, 4, "expected 4 settings keys, got %d (drift?)", len(settings))

		for _, key := range []string{"balanceScope", "allowOverdraft", "overdraftLimitEnabled", "overdraftLimit"} {
			assert.Containsf(t, settings, key, "settings must include %q", key)
		}
	})

	t.Run("minimal settings keeps only the two bools", func(t *testing.T) {
		b := minimalBalance()
		b.Settings = &mmodel.BalanceSettings{
			BalanceScope:   "",
			OverdraftLimit: nil,
		}

		req, err := events.NewBalanceConfigChanged(b, events.BalanceConfigChangeTypeSettingsUpdated).
			ToEmitRequest("tenant-7", fixedTime)
		require.NoError(t, err)

		settings := settingsSubobject(t, req.Payload)

		assert.Lenf(t, settings, 2, "expected 2 settings keys, got %d (drift?)", len(settings))

		for _, key := range []string{"allowOverdraft", "overdraftLimitEnabled"} {
			assert.Containsf(t, settings, key, "settings.%s is not omitempty and must always be present", key)
		}

		for _, key := range []string{"balanceScope", "overdraftLimit"} {
			assert.NotContainsf(t, settings, key, "settings.%s must omitempty when empty", key)
		}
	})
}
