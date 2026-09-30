// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balancecache

import (
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// legacyEntryBlob is the uppercase-only balance blob the 4.0.x and 3.8.x
// engine scripts cache, where Alias carries the transaction entry key.
func legacyEntryBlob(alias, keyField, limit string) []byte {
	return []byte(fmt.Sprintf(`{"ID":"00000000-0000-0000-0000-000000000001","AccountID":"00000000-0000-0000-0000-000000000002","AccountType":"deposit","AssetCode":"USD","Alias":%q,%s"Available":"10","OnHold":"0","Version":7,"AllowSending":1,"AllowReceiving":1,"Direction":"credit","OverdraftUsed":"0","AllowOverdraft":1,"OverdraftLimitEnabled":1,"OverdraftLimit":%q,"BalanceScope":"transactional"}`, alias, keyField, limit))
}

func TestCodecReadsIndexedLegacyAliasAsLogicalAlias(t *testing.T) {
	decoders := map[string]func([]byte) (accounting.BalanceSnapshot, error){
		"Decode": Decode, "DecodeForRead": DecodeForRead,
	}

	for _, tc := range []struct {
		name     string
		alias    string
		keyField string
		want     string
		wantRef  string
	}{
		{name: "single digit index", alias: "0#@source#default", keyField: `"Key":"default",`, want: "@source", wantRef: "@source#default"},
		{name: "multi digit index", alias: "12#a:b#default", keyField: `"Key":"default",`, want: "a:b", wantRef: "a:b#default"},
		{name: "named key", alias: "3#@source#car", keyField: `"Key":"car",`, want: "@source", wantRef: "@source#car"},
		{name: "absent key defaults before matching", alias: "0#@source#default", keyField: "", want: "@source", wantRef: "@source#default"},
		{name: "digits only two part alias keeps its alias", alias: "123#default", keyField: `"Key":"default",`, want: "123", wantRef: "123#default"},
	} {
		for decoderName, decode := range decoders {
			t.Run(tc.name+"/"+decoderName, func(t *testing.T) {
				snapshot, err := decode(legacyEntryBlob(tc.alias, tc.keyField, "0"))
				require.NoError(t, err)
				require.Equal(t, tc.want, snapshot.Alias)
				require.Equal(t, tc.wantRef, snapshot.BalanceRef)
				require.Equal(t, "10", snapshot.Available.String())
				require.Equal(t, int64(7), snapshot.Version)
			})
		}
	}
}

func TestCodecIndexedAliasInMixedBlobKeepsUppercaseAuthority(t *testing.T) {
	raw := []byte(`{"SchemaVersion":2,` +
		`"ID":"00000000-0000-0000-0000-000000000001","AccountID":"00000000-0000-0000-0000-000000000002","AccountType":"deposit","AssetCode":"USD","Alias":"0#test:account:1#car","Key":"car","Available":"999","OnHold":"0","Version":2,"AllowSending":1,"AllowReceiving":1,` +
		`"id":"00000000-0000-0000-0000-000000000001","accountId":"00000000-0000-0000-0000-000000000002","accountType":"deposit","assetCode":"USD","alias":"test:account:1","key":"car","available":"1000","onHold":"0","version":"1","allowSending":true,"allowReceiving":true}`)

	snapshot, err := DecodeForRead(raw)
	require.NoError(t, err)
	require.Equal(t, "test:account:1", snapshot.Alias)
	require.Equal(t, "test:account:1#car", snapshot.BalanceRef)
	require.True(t, decimal.NewFromInt(999).Equal(snapshot.Available), "available = %s", snapshot.Available)
	require.Equal(t, int64(2), snapshot.Version)
}

func TestCodecRejectsIndexedAliasOutsideTheEntryKeyForm(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alias string
	}{
		{name: "third part names another key", alias: "0#@source#other"},
		{name: "fourth part", alias: "0#@source#default#x"},
		{name: "non digit index", alias: "a#@source#default"},
		{name: "empty index", alias: "#@source#default"},
		{name: "empty alias", alias: "0##default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(legacyEntryBlob(tc.alias, `"Key":"default",`, "0"))
			require.EqualError(t, err, "inconsistent balance alias and key")
		})
	}
}

func TestNormalizeLimitDualWritesLogicalModernAliasForIndexedLegacyAlias(t *testing.T) {
	repaired, err := NormalizeLimitDual(legacyEntryBlob("0#@source#default", `"Key":"default",`, "1000.00"), "@ignored")
	require.NoError(t, err)

	fields, err := decodeObject(repaired)
	require.NoError(t, err)
	require.Equal(t, `"0#@source#default"`, string(fields["Alias"]))
	require.Equal(t, `"@source"`, string(fields["alias"]))
	require.Equal(t, `"1000"`, string(fields["overdraftLimit"]))
	require.Equal(t, `2`, string(fields["SchemaVersion"]))

	snapshot, err := Decode(repaired)
	require.NoError(t, err)
	require.Equal(t, "@source", snapshot.Alias)
	require.Equal(t, "@source#default", snapshot.BalanceRef)
}

func TestPatchSettingsDualWritesLogicalModernAliasOnlyForMatchingIndexedAlias(t *testing.T) {
	patch := SettingsPatch{OverdraftLimit: "0", BalanceScope: "transactional"}

	for _, tc := range []struct {
		name        string
		alias       string
		keyField    string
		wantModern  string
		wantDecoded string
	}{
		{name: "default key", alias: "0#@source#default", keyField: `"Key":"default",`, wantModern: `"@source"`, wantDecoded: "@source"},
		{name: "named key", alias: "0#@source#car", keyField: `"Key":"car",`, wantModern: `"@source"`, wantDecoded: "@source"},
		{name: "absent key defaults before matching", alias: "0#@source#default", keyField: "", wantModern: `"@source"`, wantDecoded: "@source"},
		{name: "third part names another key", alias: "0#@source#other", keyField: `"Key":"default",`, wantModern: `"0#@source#other"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			patched, err := PatchSettingsDual(legacyEntryBlob(tc.alias, tc.keyField, "0"), patch)
			require.NoError(t, err)

			fields, err := decodeObject(patched)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("%q", tc.alias), string(fields["Alias"]))
			require.Equal(t, tc.wantModern, string(fields["alias"]))

			snapshot, err := Decode(patched)
			if tc.wantDecoded == "" {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantDecoded, snapshot.Alias)
		})
	}
}
