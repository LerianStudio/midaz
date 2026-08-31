// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"reflect"
	"strings"
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

// fullBalanceSettings returns settings with every field populated, so the
// settings subobject reaches the wire at its maximum key set.
func fullBalanceSettings() *mmodel.BalanceSettings {
	limit := "1000"

	return &mmodel.BalanceSettings{
		BalanceScope:          "transactional",
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	}
}

// settingsSubobject decodes an emitted payload and returns its settings object.
func settingsSubobject(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var generic map[string]any
	require.NoError(t, json.Unmarshal(raw, &generic))

	settings, ok := generic["settings"].(map[string]any)
	require.True(t, ok, "settings must serialize as an object")

	return settings
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

func TestNewBalanceCreated_SettingsOverdraftLimitNilStaysNil(t *testing.T) {
	b := minimalBalance()
	b.Settings = &mmodel.BalanceSettings{
		AllowOverdraft: true,
		OverdraftLimit: nil,
	}

	payload := events.NewBalanceCreated(b)

	require.NotNil(t, payload.Settings)
	assert.True(t, payload.Settings.AllowOverdraft)
	assert.Nil(t, payload.Settings.OverdraftLimit)
}

func TestNewBalanceCreated_SettingsDoesNotAliasDomainOverdraftLimit(t *testing.T) {
	b := minimalBalance()
	limit := "1000"
	b.Settings = &mmodel.BalanceSettings{
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
	}

	payload := events.NewBalanceCreated(b)

	require.NotNil(t, payload.Settings)
	require.NotNil(t, payload.Settings.OverdraftLimit)
	assert.NotSame(t, b.Settings.OverdraftLimit, payload.Settings.OverdraftLimit,
		"wire payload must not alias the domain OverdraftLimit pointer")
	assert.Equal(t, *b.Settings.OverdraftLimit, *payload.Settings.OverdraftLimit)
}

// jsonTagNames returns the wire key every field of v contributes, in
// declaration order. Unexported fields and fields tagged `json:"-"` carry no
// wire key and are skipped; an untagged exported field marshals under its Go
// name, so that is what it contributes.
func jsonTagNames(v any) []string {
	typ := reflect.TypeOf(v)
	names := make([]string, 0, typ.NumField())

	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}

		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = field.Name
		}

		names = append(names, name)
	}

	return names
}

// TestBalanceSettingsPayload_MirrorsDomainJSONTags locks the wire mirror to the
// domain type it mirrors, comparing the JSON key each side contributes rather
// than field arity: a same-arity domain refactor — one field dropped, another
// added — has to surface here. The JSON shape locks only pin the payload's own
// key set, which does not move when mmodel.BalanceSettings grows a field, so
// without this test a new domain field is silently dropped from both
// balance.created and balance.config_changed with the whole suite green.
func TestBalanceSettingsPayload_MirrorsDomainJSONTags(t *testing.T) {
	domain := jsonTagNames(mmodel.BalanceSettings{})
	mirror := jsonTagNames(events.BalanceSettingsPayload{})

	assert.ElementsMatch(t, domain, mirror,
		"events.BalanceSettingsPayload must carry the same wire keys as mmodel.BalanceSettings. "+
			"A domain field must either be mirrored onto BalanceSettingsPayload — updating the settings "+
			"key-count assertions in both balance event tests and docs/streaming/ledger-events.md — or be "+
			"consciously withheld from the wire, in which case allowlist it here with the reason it is "+
			"withheld. A changed wire shape needs a ce-schemaversion bump on balance.created and "+
			"balance.config_changed.")
}

// TestBalanceCreatedPayload_JSONShape_SettingsSubobjectLocked pins the nested
// settings object on balance.created: its full key set, the two omitempty tags,
// and that it decodes to the same document as the domain type.
func TestBalanceCreatedPayload_JSONShape_SettingsSubobjectLocked(t *testing.T) {
	t.Run("full settings carries every key", func(t *testing.T) {
		b := minimalBalance()
		b.Settings = fullBalanceSettings()

		req, err := events.NewBalanceCreated(b).ToEmitRequest("tenant-1", fixedTime)
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

		req, err := events.NewBalanceCreated(b).ToEmitRequest("tenant-1", fixedTime)
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

	// Decoded documents, not raw bytes: JSON object key order carries no
	// meaning, so reordering the mirror's fields must not fail here.
	t.Run("settings serializes to the same document as the domain", func(t *testing.T) {
		b := minimalBalance()
		b.Settings = fullBalanceSettings()

		payload := events.NewBalanceCreated(b)

		mirroredRaw, err := json.Marshal(payload.Settings)
		require.NoError(t, err)

		domainRaw, err := json.Marshal(b.Settings)
		require.NoError(t, err)

		var mirrored, domain map[string]any
		require.NoError(t, json.Unmarshal(mirroredRaw, &mirrored))
		require.NoError(t, json.Unmarshal(domainRaw, &domain))

		assert.Equal(t, domain, mirrored,
			"events.BalanceSettingsPayload must serialize to the same JSON document as "+
				"mmodel.BalanceSettings. This assertion is permanent: it documents that the wire mirror "+
				"has not diverged from the domain. Divergence is allowed, but it is a wire change — bump "+
				"ce-schemaversion on balance.created and balance.config_changed and update "+
				"docs/streaming/ledger-events.md before relaxing this.")
	})
}
