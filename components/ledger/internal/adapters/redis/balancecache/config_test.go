// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balancecache

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

const (
	configBalanceID = "00000000-0000-0000-0000-000000000001"
	configAccountID = "00000000-0000-0000-0000-000000000002"
)

// Money is written on purpose in non-canonical, legacy representations: a copy
// that parsed and re-encoded it would change these bytes.
const (
	dualBlobV4      = `{"SchemaVersion":2,"ID":"00000000-0000-0000-0000-000000000001","id":"00000000-0000-0000-0000-000000000001","AccountID":"00000000-0000-0000-0000-000000000002","accountId":"00000000-0000-0000-0000-000000000002","AccountType":"deposit","accountType":"deposit","AssetCode":"USD","assetCode":"USD","Alias":"@payer","alias":"@payer","Key":"default","key":"default","Direction":"credit","direction":"credit","BalanceScope":"transactional","balanceScope":"transactional","Available":120.500,"available":"120.5","OnHold":"0.10","onHold":"0.1","OverdraftUsed":0,"overdraftUsed":"0","OverdraftLimit":"0","overdraftLimit":"0","Version":9,"version":"9","AllowSending":1,"allowSending":true,"AllowReceiving":1,"allowReceiving":true,"Blocked":1,"blocked":true,"AllowOverdraft":0,"allowOverdraft":false,"OverdraftLimitEnabled":0,"overdraftLimitEnabled":false,"balanceConfigVersion":"4","accountConfigVersion":"7","extension":{"a": [1, 2]}}`
	legacyBlob      = `{"ID":"00000000-0000-0000-0000-000000000001","AccountID":"00000000-0000-0000-0000-000000000002","AccountType":"deposit","AssetCode":"USD","Alias":"0#@payer#default","Key":"default","Available":120.500,"OnHold":1E1,"Version":9,"AllowSending":1,"AllowReceiving":1,"Direction":"credit","OverdraftUsed":"0"}`
	newOnlyBlob     = `{"SchemaVersion":2,"id":"00000000-0000-0000-0000-000000000001","accountId":"00000000-0000-0000-0000-000000000002","accountType":"deposit","assetCode":"USD","alias":"@payer","key":"default","available":"120.5","onHold":"0","version":"9","allowSending":true,"allowReceiving":true,"blocked":false}`
	lowerLegacyBlob = `{"id":"00000000-0000-0000-0000-000000000001","alias":"","key":"","accountId":"00000000-0000-0000-0000-000000000002","assetCode":"USD","available":"100","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1,"overdraftUsed":""}`
)

func copiedBalanceConfig(version int64) accounting.BalanceConfig {
	return accounting.BalanceConfig{
		BalanceID:             uuid.MustParse(configBalanceID),
		Version:               version,
		AllowSending:          false,
		AllowReceiving:        true,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        decimal.RequireFromString("250.5"),
		BalanceScope:          mmodel.BalanceScopeInternal,
	}
}

func rawFields(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()

	fields, err := decodeObject(raw)
	require.NoError(t, err)

	return fields
}

// requireUntouched asserts every field outside the replaced family kept its exact
// bytes, and that only the family and its version field may have appeared.
func requireUntouched(t *testing.T, before, after []byte, replaced []string, versionField string) {
	t.Helper()

	original := rawFields(t, before)
	copied := rawFields(t, after)

	skip := map[string]bool{versionField: true}
	for _, name := range replaced {
		skip[name] = true
		skip[lowerName(name)] = true
	}

	for name, value := range original {
		if skip[name] {
			continue
		}

		require.Contains(t, copied, name)
		assert.Equal(t, string(value), string(copied[name]), "field %s must keep its exact bytes", name)
	}

	for name := range copied {
		if _, existed := original[name]; !existed {
			assert.True(t, skip[name], "the copy may only add its family and version, added %s", name)
		}
	}
}

var balanceFamily = []string{"AllowSending", "AllowReceiving", "AllowOverdraft", "OverdraftLimitEnabled", "OverdraftLimit", "BalanceScope"}

func TestBalanceConfigKeyValue_CanonicalAndRoundTrips(t *testing.T) {
	t.Parallel()

	limit := "100.50"
	balance := &mmodel.Balance{
		ID:             configBalanceID,
		ConfigVersion:  3,
		AllowSending:   true,
		AllowReceiving: false,
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        &limit,
		},
	}

	config, err := BalanceConfigFromBalance(balance)
	require.NoError(t, err)
	assert.Equal(t, mmodel.BalanceScopeTransactional, config.BalanceScope, "an empty scope reads as transactional")

	encoded, err := EncodeBalanceConfig(config)
	require.NoError(t, err)
	assert.JSONEq(t, `{"balanceId":"`+configBalanceID+`","configVersion":"3","allowSending":true,"allowReceiving":false,"allowOverdraft":true,"overdraftLimitEnabled":true,"overdraftLimit":"100.5","balanceScope":"transactional"}`, string(encoded))

	decoded, err := DecodeBalanceConfig(encoded)
	require.NoError(t, err)
	assert.True(t, decoded.Equal(config))

	again, err := EncodeBalanceConfig(decoded)
	require.NoError(t, err)
	assert.Equal(t, string(encoded), string(again), "equal configurations encode to the same bytes")

	legacy, err := BalanceConfigFromBalance(&mmodel.Balance{ID: configBalanceID, AllowSending: true, AllowReceiving: true})
	require.NoError(t, err)
	assert.Equal(t, accounting.BalanceConfig{
		BalanceID: uuid.MustParse(configBalanceID), AllowSending: true, AllowReceiving: true,
		OverdraftLimit: decimal.Zero, BalanceScope: mmodel.BalanceScopeTransactional,
	}, legacy, "a row without settings takes the legacy defaults")

	_, err = BalanceConfigFromBalance(&mmodel.Balance{ID: configBalanceID, Settings: &mmodel.BalanceSettings{BalanceScope: "shared"}})
	require.Error(t, err, "an unknown scope is refused")
}

func TestBalanceConfigKeyValue_DecodeRejectsNonCanonicalValues(t *testing.T) {
	t.Parallel()

	valid := `"balanceId":"` + configBalanceID + `","allowSending":true,"allowReceiving":true,"allowOverdraft":false,"overdraftLimitEnabled":false,"balanceScope":"transactional"`

	for name, raw := range map[string]string{
		"padded version":      `{` + valid + `,"configVersion":"05","overdraftLimit":"0"}`,
		"numeric version":     `{` + valid + `,"configVersion":5,"overdraftLimit":"0"}`,
		"negative version":    `{` + valid + `,"configVersion":"-1","overdraftLimit":"0"}`,
		"non canonical limit": `{` + valid + `,"configVersion":"5","overdraftLimit":"10.50"}`,
		"missing flag":        `{"balanceId":"` + configBalanceID + `","configVersion":"5","allowSending":true,"allowReceiving":true,"allowOverdraft":false,"overdraftLimit":"0","balanceScope":"transactional"}`,
		"unknown scope":       `{"balanceId":"` + configBalanceID + `","configVersion":"5","allowSending":true,"allowReceiving":true,"allowOverdraft":false,"overdraftLimitEnabled":false,"overdraftLimit":"0","balanceScope":"shared"}`,
	} {
		_, err := DecodeBalanceConfig([]byte(raw))
		assert.Error(t, err, name)
	}

	config, err := DecodeBalanceConfig([]byte(`{` + valid + `,"configVersion":"5","overdraftLimit":"0","futureField":1}`))
	require.NoError(t, err, "unknown fields are ignored")
	assert.Equal(t, int64(5), config.Version)
}

// A key left by a deleted balance under the same alias#key describes another
// balance: the recreated balance reads it as absent, not as its configuration.
func TestBalanceConfigKeyValue_OtherBalanceReadsAsAbsent(t *testing.T) {
	t.Parallel()

	stale, err := EncodeBalanceConfig(copiedBalanceConfig(5))
	require.NoError(t, err)

	recreated := uuid.MustParse("00000000-0000-0000-0000-0000000000f2")

	_, found, err := DecodeBalanceConfigFor(stale, recreated)
	require.NoError(t, err)
	assert.False(t, found)

	config, found, err := DecodeBalanceConfigFor(stale, uuid.MustParse(configBalanceID))
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, int64(5), config.Version)

	_, found, err = DecodeBalanceConfigFor(nil, recreated)
	require.NoError(t, err)
	assert.False(t, found, "a missing key is absent")
}

func TestAccountConfigKeyValue_RoundTrips(t *testing.T) {
	t.Parallel()

	blocked := true
	config, err := AccountConfigFromAccount(&mmodel.Account{ID: configAccountID, Blocked: &blocked, ConfigVersion: 2})
	require.NoError(t, err)

	encoded, err := EncodeAccountConfig(config)
	require.NoError(t, err)
	assert.JSONEq(t, `{"accountId":"`+configAccountID+`","configVersion":"2","blocked":true}`, string(encoded))

	decoded, err := DecodeAccountConfig(encoded)
	require.NoError(t, err)
	assert.Equal(t, config, decoded)

	unblocked, err := AccountConfigFromAccount(&mmodel.Account{ID: configAccountID})
	require.NoError(t, err)
	assert.False(t, unblocked.Blocked, "an absent flag is an account that is not blocked")

	_, err = DecodeAccountConfig([]byte(`{"accountId":"` + configAccountID + `","configVersion":"2"}`))
	assert.Error(t, err, "the blocked flag is required")
}

func TestCopyBalanceConfig_WritesTheWholeFamilyAndKeepsEveryOtherByte(t *testing.T) {
	t.Parallel()

	replacement, current, err := CopyBalanceConfig([]byte(dualBlobV4), copiedBalanceConfig(6))
	require.NoError(t, err)
	require.False(t, current)

	requireUntouched(t, []byte(dualBlobV4), replacement, balanceFamily, BlobBalanceConfigVersionField)

	fields := rawFields(t, replacement)
	for name, want := range map[string]string{
		"AllowSending": `0`, "allowSending": `false`,
		"AllowReceiving": `1`, "allowReceiving": `true`,
		"AllowOverdraft": `1`, "allowOverdraft": `true`,
		"OverdraftLimitEnabled": `1`, "overdraftLimitEnabled": `true`,
		"OverdraftLimit": `"250.5"`, "overdraftLimit": `"250.5"`,
		"BalanceScope": `"internal"`, "balanceScope": `"internal"`,
		BlobBalanceConfigVersionField: `"6"`,
	} {
		assert.Equal(t, want, string(fields[name]), name)
	}

	snapshot, err := DecodeForRead(replacement)
	require.NoError(t, err, "the copy stays a valid balance blob")
	assert.Equal(t, int64(6), snapshot.ConfigVersion)
	assert.Equal(t, int64(7), snapshot.AccountConfigVersion)
	assert.True(t, snapshot.AllowOverdraft)
	assert.Equal(t, mmodel.BalanceScopeInternal, snapshot.BalanceScope)
}

func TestCopyBalanceConfig_NeverMovesTheBlobBack(t *testing.T) {
	t.Parallel()

	for _, version := range []int64{3, 4} {
		replacement, current, err := CopyBalanceConfig([]byte(dualBlobV4), copiedBalanceConfig(version))
		require.NoError(t, err)
		assert.True(t, current, "a blob holding version 4 is current for version %d", version)
		assert.Nil(t, replacement)
	}
}

func TestCopyBalanceConfig_KeepsTheShapeOfLegacyAndNewOnlyBlobs(t *testing.T) {
	t.Parallel()

	t.Run("uppercase legacy blob with numeric money and an indexed alias", func(t *testing.T) {
		t.Parallel()

		replacement, current, err := CopyBalanceConfig([]byte(legacyBlob), copiedBalanceConfig(1))
		require.NoError(t, err)
		require.False(t, current, "a blob without a copy holds version 0")

		requireUntouched(t, []byte(legacyBlob), replacement, balanceFamily, BlobBalanceConfigVersionField)

		fields := rawFields(t, replacement)
		assert.Equal(t, `120.500`, string(fields["Available"]))
		assert.Equal(t, `1E1`, string(fields["OnHold"]))
		assert.Equal(t, `"0#@payer#default"`, string(fields["Alias"]))
		assert.Equal(t, `1`, string(fields["AllowOverdraft"]))
		assert.Equal(t, `"250.5"`, string(fields["OverdraftLimit"]))
		assert.NotContains(t, fields, "allowOverdraft", "a legacy blob gains no lowercase field")
		assert.NotContains(t, fields, "SchemaVersion")
	})

	t.Run("new-only blob", func(t *testing.T) {
		t.Parallel()

		replacement, _, err := CopyBalanceConfig([]byte(newOnlyBlob), copiedBalanceConfig(1))
		require.NoError(t, err)

		requireUntouched(t, []byte(newOnlyBlob), replacement, balanceFamily, BlobBalanceConfigVersionField)

		fields := rawFields(t, replacement)
		assert.Equal(t, `false`, string(fields["allowSending"]))
		assert.Equal(t, `true`, string(fields["allowOverdraft"]))
		assert.NotContains(t, fields, "AllowOverdraft", "a new-only blob gains no uppercase field")
	})

	t.Run("pre-schema lowercase blob keeps numeric flags", func(t *testing.T) {
		t.Parallel()

		replacement, _, err := CopyBalanceConfig([]byte(lowerLegacyBlob), copiedBalanceConfig(1))
		require.NoError(t, err)

		requireUntouched(t, []byte(lowerLegacyBlob), replacement, balanceFamily, BlobBalanceConfigVersionField)

		fields := rawFields(t, replacement)
		assert.Equal(t, `0`, string(fields["allowSending"]))
		assert.Equal(t, `1`, string(fields["allowOverdraft"]))
	})
}

func TestCopyBalanceConfig_RefusesAnotherBalancesBlob(t *testing.T) {
	t.Parallel()

	config := copiedBalanceConfig(6)
	config.BalanceID = uuid.MustParse("00000000-0000-0000-0000-0000000000f2")

	_, _, err := CopyBalanceConfig([]byte(dualBlobV4), config)
	require.ErrorIs(t, err, ErrBlobOfAnotherBalance)
}

func TestCopyBalanceConfig_RefusesAMalformedCopyVersion(t *testing.T) {
	t.Parallel()

	fields := rawFields(t, []byte(dualBlobV4))
	fields[BlobBalanceConfigVersionField] = json.RawMessage(`4`)

	_, _, err := CopyBalanceConfig(encodeRawObject(fields), copiedBalanceConfig(6))
	require.Error(t, err)
}

func TestCopyAccountConfig_WritesBlockedAndKeepsEveryOtherByte(t *testing.T) {
	t.Parallel()

	config := accounting.AccountConfig{AccountID: uuid.MustParse(configAccountID), Version: 8, Blocked: false}

	replacement, current, err := CopyAccountConfig([]byte(dualBlobV4), config)
	require.NoError(t, err)
	require.False(t, current)

	requireUntouched(t, []byte(dualBlobV4), replacement, []string{"Blocked"}, BlobAccountConfigVersionField)

	fields := rawFields(t, replacement)
	assert.Equal(t, `0`, string(fields["Blocked"]))
	assert.Equal(t, `false`, string(fields["blocked"]))
	assert.Equal(t, `"8"`, string(fields[BlobAccountConfigVersionField]))
	assert.Equal(t, `"4"`, string(fields[BlobBalanceConfigVersionField]), "the balance family is not touched")

	config.Version = 7
	_, current, err = CopyAccountConfig([]byte(dualBlobV4), config)
	require.NoError(t, err)
	assert.True(t, current)

	config.AccountID = uuid.MustParse("00000000-0000-0000-0000-0000000000f3")
	config.Version = 9
	_, _, err = CopyAccountConfig([]byte(dualBlobV4), config)
	require.ErrorIs(t, err, ErrBlobOfAnotherBalance)
}

func TestBlobConfigVersions_AbsentCopyReadsZero(t *testing.T) {
	t.Parallel()

	balanceVersion, accountVersion, err := BlobConfigVersions([]byte(legacyBlob))
	require.NoError(t, err)
	assert.Zero(t, balanceVersion)
	assert.Zero(t, accountVersion)

	balanceVersion, accountVersion, err = BlobConfigVersions([]byte(dualBlobV4))
	require.NoError(t, err)
	assert.Equal(t, int64(4), balanceVersion)
	assert.Equal(t, int64(7), accountVersion)
}

// The cache codec carries the copy versions through a round trip, and leaves
// them out of a blob that holds no copy, which is what an older writer produces.
func TestCodecCarriesTheConfigurationCopyVersions(t *testing.T) {
	t.Parallel()

	snapshot := codecSnapshot()

	plain, err := Encode(snapshot, FormatDual)
	require.NoError(t, err)
	assert.NotContains(t, string(plain), BlobBalanceConfigVersionField)
	assert.NotContains(t, string(plain), BlobAccountConfigVersionField)

	snapshot.ConfigVersion = 12
	snapshot.AccountConfigVersion = 3

	versioned, err := Encode(snapshot, FormatDual)
	require.NoError(t, err)

	decoded, err := Decode(versioned)
	require.NoError(t, err)
	assert.Equal(t, int64(12), decoded.ConfigVersion)
	assert.Equal(t, int64(3), decoded.AccountConfigVersion)
}
