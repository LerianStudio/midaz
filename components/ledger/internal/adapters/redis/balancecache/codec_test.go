// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balancecache

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
)

func codecSnapshot() engine.BalanceSnapshot {
	return engine.BalanceSnapshot{
		BalanceRef: "@source#default", Alias: "@source", Key: "default",
		ID:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		AccountID:   uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		AccountType: "deposit", AssetCode: "USD", Direction: "credit", BalanceScope: "transactional",
		Available: decimal.RequireFromString("123456789012345678901234567890.123456789"),
		OnHold:    decimal.NewFromInt(17), OverdraftUsed: decimal.RequireFromString("0.000000001"),
		OverdraftLimit: decimal.NewFromInt(1000), Version: 7,
		AllowSending: true, AllowReceiving: true, AllowOverdraft: true, OverdraftLimitEnabled: true,
	}
}

func codecFields(t *testing.T, format Format) map[string]json.RawMessage {
	t.Helper()
	raw, err := Encode(codecSnapshot(), format)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))

	return fields
}

func marshalCodecFields(t *testing.T, fields map[string]json.RawMessage) []byte {
	t.Helper()
	raw, err := json.Marshal(fields)
	require.NoError(t, err)

	return raw
}

func TestCodecRoundTrip(t *testing.T) {
	for _, format := range []Format{FormatDual, FormatNewOnly} {
		t.Run(strconv.Itoa(int(format)), func(t *testing.T) {
			snapshot := codecSnapshot()
			snapshot.Version = math.MaxInt64
			raw, err := Encode(snapshot, format)
			require.NoError(t, err)
			decoded, err := Decode(raw)
			require.NoError(t, err)
			require.Equal(t, snapshot, decoded)
			require.Contains(t, string(raw), `"version":"9223372036854775807"`)
			if format == FormatDual {
				require.Contains(t, string(raw), `"Version":9223372036854775807`)
			}
			reencoded, err := Encode(decoded, format)
			require.NoError(t, err)
			require.Equal(t, raw, reencoded, "encoding must be deterministic")
		})
	}
}

func TestCodecLegacyMutationWinsOverStaleNewFields(t *testing.T) {
	fields := codecFields(t, FormatDual)
	fields["Available"] = json.RawMessage(`"140"`)
	fields["Version"] = json.RawMessage(`8`)
	fields["AllowOverdraft"] = json.RawMessage(`0`)
	fields["OverdraftLimit"] = json.RawMessage(`"2000"`)
	snapshot, err := Decode(marshalCodecFields(t, fields))
	require.NoError(t, err)
	require.True(t, snapshot.Available.Equal(decimal.NewFromInt(140)))
	require.EqualValues(t, 8, snapshot.Version)
	require.False(t, snapshot.AllowOverdraft)
	require.True(t, snapshot.OverdraftLimit.Equal(decimal.NewFromInt(2000)))

	fields["AllowOverdraft"] = json.RawMessage(`1`)
	patched, err := Decode(marshalCodecFields(t, fields))
	require.NoError(t, err)
	require.Equal(t, snapshot.Version, patched.Version, "settings changes do not require a monetary version increment")
	require.True(t, patched.AllowOverdraft)
}

func TestCodecNewOnlyCanReconstructDualRepresentation(t *testing.T) {
	fields := codecFields(t, FormatNewOnly)
	fields["available"] = json.RawMessage(`"42"`)
	snapshot, err := Decode(marshalCodecFields(t, fields))
	require.NoError(t, err)
	raw, err := Encode(snapshot, FormatDual)
	require.NoError(t, err)
	var dual map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &dual))
	for _, name := range fieldNames {
		require.Contains(t, dual, name)
		require.Contains(t, dual, lowerName(name))
	}
	require.JSONEq(t, `"42"`, string(dual["Available"]))
	require.JSONEq(t, `"42"`, string(dual["available"]))
	require.JSONEq(t, `1`, string(dual["AllowOverdraft"]))
	require.JSONEq(t, `true`, string(dual["allowOverdraft"]))
}

func TestCodecLegacyDefaultsAndPerFieldFallback(t *testing.T) {
	fields := codecFields(t, FormatDual)
	delete(fields, "SchemaVersion")
	for _, name := range fieldNames {
		delete(fields, lowerName(name))
	}
	for _, name := range []string{"Direction", "BalanceScope", "OverdraftUsed", "OverdraftLimit", "AllowOverdraft", "OverdraftLimitEnabled"} {
		delete(fields, name)
	}
	fields["Available"] = json.RawMessage(`"10"`)
	snapshot, err := Decode(marshalCodecFields(t, fields))
	require.NoError(t, err)
	require.Empty(t, snapshot.Direction)
	require.Equal(t, "transactional", snapshot.BalanceScope)
	require.True(t, snapshot.OverdraftUsed.IsZero())
	require.True(t, snapshot.OverdraftLimit.IsZero())
	require.False(t, snapshot.AllowOverdraft)
	require.False(t, snapshot.OverdraftLimitEnabled)

	fields["SchemaVersion"] = json.RawMessage(`2`)
	delete(fields, "Available")
	fields["available"] = json.RawMessage(`"20"`)
	snapshot, err = Decode(marshalCodecFields(t, fields))
	require.NoError(t, err)
	require.True(t, snapshot.Available.Equal(decimal.NewFromInt(20)))
}

func TestCodecNoncanonicalLimitRequiresExplicitRepair(t *testing.T) {
	for _, value := range []string{"1E+3", "1000.00", "+1000", "01000", "-0"} {
		t.Run(value, func(t *testing.T) {
			fields := codecFields(t, FormatDual)
			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			fields["OverdraftLimit"] = encoded
			snapshot, err := Decode(marshalCodecFields(t, fields))
			var noncanonical *NoncanonicalLimitError
			require.ErrorAs(t, err, &noncanonical)
			require.Equal(t, value, noncanonical.Raw)
			require.Equal(t, decimal.RequireFromString(value).String(), noncanonical.Canonical)
			require.Equal(t, engine.BalanceSnapshot{}, snapshot)
		})
	}
}

func TestCodecDecodeForReadNormalizesNoncanonicalLimitWithoutMutatingRaw(t *testing.T) {
	for _, value := range []string{"1E+3", "1000.00", "+1000"} {
		t.Run(value, func(t *testing.T) {
			raw := []byte(`{"SchemaVersion":2,"ID":"00000000-0000-0000-0000-000000000001","AccountID":"00000000-0000-0000-0000-000000000002","AccountType":"deposit","AssetCode":"USD","Alias":"@source","Key":"default","Direction":"credit","BalanceScope":"transactional","Available":"10","OnHold":"0","OverdraftUsed":"0","Version":7,"AllowSending":1,"AllowReceiving":1,"AllowOverdraft":1,"OverdraftLimitEnabled":1,"OverdraftLimit":"` + value + `"}`)
			before := append([]byte(nil), raw...)
			snapshot, err := DecodeForRead(raw)
			require.NoError(t, err)
			require.True(t, snapshot.OverdraftLimit.Equal(decimal.NewFromInt(1000)))
			require.Equal(t, before, raw)
		})
	}
}

func TestCodecDecodeForReadLegacyAuthoritativeValueWins(t *testing.T) {
	raw := []byte(`{"SchemaVersion":2,"ID":"00000000-0000-0000-0000-000000000001","id":"00000000-0000-0000-0000-000000000001","AccountID":"00000000-0000-0000-0000-000000000002","accountId":"00000000-0000-0000-0000-000000000002","AccountType":"deposit","accountType":"deposit","AssetCode":"USD","assetCode":"USD","Alias":"@source","alias":"@source","Key":"default","key":"default","Available":"10","available":"20","OnHold":"0","onHold":"0","Version":7,"version":"8","AllowSending":1,"allowSending":false,"AllowReceiving":1,"allowReceiving":true,"AllowOverdraft":1,"allowOverdraft":false,"OverdraftLimitEnabled":1,"overdraftLimitEnabled":true,"OverdraftLimit":"1000.00","overdraftLimit":"2000","Direction":"credit","direction":"credit","BalanceScope":"transactional","balanceScope":"transactional","OverdraftUsed":"0","overdraftUsed":"0"}`)
	snapshot, err := DecodeForRead(raw)
	require.NoError(t, err)
	require.True(t, snapshot.Available.Equal(decimal.NewFromInt(10)))
	require.EqualValues(t, 7, snapshot.Version)
	require.True(t, snapshot.AllowOverdraft)
	require.True(t, snapshot.OverdraftLimit.Equal(decimal.NewFromInt(1000)))
}

func TestCodecDecodeForReadRejectsMalformedAuthoritativeLimit(t *testing.T) {
	raw := []byte(`{"SchemaVersion":2,"ID":"00000000-0000-0000-0000-000000000001","AccountID":"00000000-0000-0000-0000-000000000002","AccountType":"deposit","AssetCode":"USD","Alias":"@source","Key":"default","Available":"10","OnHold":"0","Version":7,"AllowSending":1,"AllowReceiving":1,"AllowOverdraft":1,"OverdraftLimitEnabled":1,"OverdraftLimit":"bad","overdraftLimit":"1000"}`)
	snapshot, err := DecodeForRead(raw)
	require.Error(t, err)
	require.Equal(t, engine.BalanceSnapshot{}, snapshot)
}

func TestCodecDecodeForReadRejectsNegativeAndInvalidLimits(t *testing.T) {
	for _, value := range []string{"-1", "NaN", "bad"} {
		t.Run(value, func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`{"SchemaVersion":2,"ID":"00000000-0000-0000-0000-000000000001","AccountID":"00000000-0000-0000-0000-000000000002","AccountType":"deposit","AssetCode":"USD","Alias":"@source","Key":"default","Available":"10","OnHold":"0","Version":7,"AllowSending":1,"AllowReceiving":1,"AllowOverdraft":1,"OverdraftLimitEnabled":1,"OverdraftLimit":%q}`, value))
			for _, decode := range []func([]byte) (engine.BalanceSnapshot, error){Decode, DecodeForRead} {
				snapshot, err := decode(raw)
				require.Error(t, err)
				require.Equal(t, engine.BalanceSnapshot{}, snapshot)
			}
		})
	}
}

func TestCodecDecodeForReadNormalizesNewOnlyNoncanonicalLimit(t *testing.T) {
	raw := []byte(`{"SchemaVersion":2,"id":"00000000-0000-0000-0000-000000000001","accountId":"00000000-0000-0000-0000-000000000002","accountType":"deposit","assetCode":"USD","alias":"@source","key":"default","direction":"credit","balanceScope":"transactional","available":"10","onHold":"0","overdraftUsed":"0","version":"7","allowSending":true,"allowReceiving":true,"allowOverdraft":true,"overdraftLimitEnabled":true,"overdraftLimit":"1000.00"}`)
	snapshot, err := DecodeForRead(raw)
	require.NoError(t, err)
	require.True(t, snapshot.OverdraftLimit.Equal(decimal.NewFromInt(1000)))
	_, err = Decode(raw)
	var noncanonical *NoncanonicalLimitError
	require.ErrorAs(t, err, &noncanonical)
}

const legacyBalanceRedisFixture = `{"id":"820b976d-2fae-42eb-a20c-ca482c9a4a1e","alias":"","key":"","accountId":"6fd82a96-2858-41bb-8c4c-99e0ae69acee","assetCode":"USD","available":"100","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1,"direction":"","overdraftUsed":"","allowOverdraft":0,"overdraftLimitEnabled":0,"overdraftLimit":"","balanceScope":""}`

func TestCodecDecodeForReadHistoricalBalanceRedisShape(t *testing.T) {
	snapshot, err := DecodeForRead([]byte(legacyBalanceRedisFixture))
	require.NoError(t, err)
	require.Equal(t, "default", snapshot.Key)
	require.True(t, snapshot.Available.Equal(decimal.NewFromInt(100)))
	require.True(t, snapshot.OnHold.IsZero())
	require.True(t, snapshot.OverdraftUsed.IsZero())
	require.True(t, snapshot.OverdraftLimit.IsZero())
	require.True(t, snapshot.AllowSending)
	require.True(t, snapshot.AllowReceiving)
	_, err = Decode([]byte(legacyBalanceRedisFixture))
	require.Error(t, err)
}

func TestCodecDecodeForReadAcceptsLegacyMoneyRepresentations(t *testing.T) {
	for _, source := range []string{"uppercase", "historical-lower"} {
		for _, tc := range []struct {
			name  string
			field string
			raw   string
			want  string
		}{
			{name: "available-noncanonical", field: "Available", raw: `"100.00"`, want: "100"},
			{name: "on-hold-leading-zero", field: "OnHold", raw: `"010.00"`, want: "10"},
			{name: "overdraft-used-exponent", field: "OverdraftUsed", raw: `"1E+3"`, want: "1000"},
			{name: "available-exact-number", field: "Available", raw: `9007199254740993.000000001`, want: "9007199254740993.000000001"},
			{name: "on-hold-exact-number", field: "OnHold", raw: `9007199254740993.000000001`, want: "9007199254740993.000000001"},
			{name: "overdraft-used-exact-number", field: "OverdraftUsed", raw: `9007199254740993.000000001`, want: "9007199254740993.000000001"},
		} {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				var fields map[string]json.RawMessage
				field := tc.field
				if source == "uppercase" {
					fields = codecFields(t, FormatDual)
				} else {
					require.NoError(t, json.Unmarshal([]byte(legacyBalanceRedisFixture), &fields))
					field = lowerName(field)
				}
				fields[field] = json.RawMessage(tc.raw)

				snapshot, err := DecodeForRead(marshalCodecFields(t, fields))
				require.NoError(t, err)
				var got decimal.Decimal
				switch lowerName(field) {
				case "available":
					got = snapshot.Available
				case "onHold":
					got = snapshot.OnHold
				case "overdraftUsed":
					got = snapshot.OverdraftUsed
				}
				require.Equal(t, tc.want, got.String())
			})
		}
	}
}

func TestCodecDecodeForReadKeepsNewOnlyMoneyStrict(t *testing.T) {
	for _, tc := range []struct {
		field string
		raw   string
	}{
		{field: "available", raw: `"100.00"`},
		{field: "onHold", raw: `"010.00"`},
		{field: "overdraftUsed", raw: `"1E+3"`},
		{field: "available", raw: `9007199254740993.000000001`},
		{field: "onHold", raw: `9007199254740993.000000001`},
		{field: "overdraftUsed", raw: `9007199254740993.000000001`},
	} {
		t.Run(tc.field+"/"+tc.raw, func(t *testing.T) {
			fields := codecFields(t, FormatNewOnly)
			fields[tc.field] = json.RawMessage(tc.raw)
			snapshot, err := DecodeForRead(marshalCodecFields(t, fields))
			require.Error(t, err)
			require.Equal(t, engine.BalanceSnapshot{}, snapshot)
		})
	}
}

func TestCodecDecodeForReadRejectsInvalidLegacyMoneyWithoutFallback(t *testing.T) {
	for _, source := range []string{"uppercase", "historical-lower"} {
		for _, field := range []string{"Available", "OnHold", "OverdraftUsed", "OverdraftLimit"} {
			for _, raw := range []string{`null`, `true`, `{}`, `[]`, `"bad"`} {
				t.Run(source+"/"+field+"/"+raw, func(t *testing.T) {
					var fields map[string]json.RawMessage
					selectedField := field
					if source == "uppercase" {
						fields = codecFields(t, FormatDual)
					} else {
						require.NoError(t, json.Unmarshal([]byte(legacyBalanceRedisFixture), &fields))
						selectedField = lowerName(selectedField)
					}
					fields[selectedField] = json.RawMessage(raw)

					snapshot, err := DecodeForRead(marshalCodecFields(t, fields))
					require.Error(t, err)
					require.Equal(t, engine.BalanceSnapshot{}, snapshot)
				})
			}
		}
	}

	for _, source := range []string{"uppercase", "historical-lower"} {
		t.Run(source+"/overdraft-limit-number", func(t *testing.T) {
			var fields map[string]json.RawMessage
			field := "OverdraftLimit"
			if source == "uppercase" {
				fields = codecFields(t, FormatDual)
				fields["overdraftLimit"] = json.RawMessage(`"2000"`)
			} else {
				require.NoError(t, json.Unmarshal([]byte(legacyBalanceRedisFixture), &fields))
				field = "overdraftLimit"
			}
			fields[field] = json.RawMessage(`1000`)

			snapshot, err := DecodeForRead(marshalCodecFields(t, fields))
			require.Error(t, err)
			require.Equal(t, engine.BalanceSnapshot{}, snapshot)
		})
	}
}

func TestCodecStrictDecodeRejectsLegacyReadMoneyRepresentations(t *testing.T) {
	for _, tc := range []struct {
		field string
		raw   string
	}{
		{field: "Available", raw: `"100.00"`},
		{field: "OnHold", raw: `"010.00"`},
		{field: "OverdraftUsed", raw: `"1E+3"`},
		{field: "Available", raw: `9007199254740993.000000001`},
		{field: "OnHold", raw: `9007199254740993.000000001`},
		{field: "OverdraftUsed", raw: `9007199254740993.000000001`},
	} {
		t.Run(tc.field+"/"+tc.raw, func(t *testing.T) {
			fields := codecFields(t, FormatDual)
			fields[tc.field] = json.RawMessage(tc.raw)
			snapshot, err := Decode(marshalCodecFields(t, fields))
			require.Error(t, err)
			require.Equal(t, engine.BalanceSnapshot{}, snapshot)
		})
	}
}

func TestCodecDecodeForReadHistoricalShapeUppercaseSettingsAreAuthoritative(t *testing.T) {
	raw := []byte(`{"id":"820b976d-2fae-42eb-a20c-ca482c9a4a1e","accountId":"6fd82a96-2858-41bb-8c4c-99e0ae69acee","assetCode":"USD","available":"100","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1,"allowOverdraft":0,"overdraftLimitEnabled":0,"overdraftLimit":"0","AllowOverdraft":1,"OverdraftLimitEnabled":1,"OverdraftLimit":"1000.00"}`)
	snapshot, err := DecodeForRead(raw)
	require.NoError(t, err)
	require.True(t, snapshot.AllowOverdraft)
	require.True(t, snapshot.OverdraftLimit.Equal(decimal.NewFromInt(1000)))
}

func TestCodecDecodeForReadHistoricalShapeRejectsWrongTypesAndBadAuthority(t *testing.T) {
	for _, raw := range []string{
		`{"id":"820b976d-2fae-42eb-a20c-ca482c9a4a1e","accountId":"6fd82a96-2858-41bb-8c4c-99e0ae69acee","assetCode":"USD","available":"100","onHold":"0","version":"1","accountType":"deposit","allowSending":true,"allowReceiving":1}`,
		`{"id":"820b976d-2fae-42eb-a20c-ca482c9a4a1e","accountId":"6fd82a96-2858-41bb-8c4c-99e0ae69acee","assetCode":"USD","available":"100","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1,"overdraftLimit":"0","OverdraftLimit":"bad"}`,
	} {
		snapshot, err := DecodeForRead([]byte(raw))
		require.Error(t, err)
		require.Equal(t, engine.BalanceSnapshot{}, snapshot)
	}
}

func TestCodecDecodeForReadHistoricalShapeRejectsEmptyUppercaseAuthority(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
	}{
		{name: "Alias", field: `,"Alias":""`},
		{name: "OverdraftUsed", field: `,"OverdraftUsed":""`},
		{name: "OverdraftLimit", field: `,"OverdraftLimit":""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`{"id":"820b976d-2fae-42eb-a20c-ca482c9a4a1e","alias":"@source","accountId":"6fd82a96-2858-41bb-8c4c-99e0ae69acee","assetCode":"USD","available":"100","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1,"overdraftUsed":"0","overdraftLimit":"0"%s}`, tc.field))
			snapshot, err := DecodeForRead(raw)
			require.Error(t, err)
			require.Equal(t, engine.BalanceSnapshot{}, snapshot)
		})
	}
}

func TestCodecRejectsMalformedAuthoritativeFields(t *testing.T) {
	for _, tc := range []struct{ field, value string }{
		{"Available", `null`},
		{"Available", `100`},
		{"Available", `"1e2"`},
		{"OnHold", `"-1"`},
		{"OverdraftUsed", `"bad"`},
		{"OverdraftLimit", `null`},
		{"OverdraftLimit", `1000`},
		{"OverdraftLimit", `"NaN"`},
		{"AllowOverdraft", `true`},
		{"AllowReceiving", `2`},
		{"Version", `"7"`},
		{"Version", `1.5`},
		{"Version", `1e2`},
		{"Version", `9223372036854775808`},
		{"Version", `-1`},
		{"ID", `"not-a-uuid"`},
		{"AccountID", `"00000000-0000-0000-0000-000000000000"`},
		{"SchemaVersion", `3`},
		{"SchemaVersion", `null`},
		{"Direction", `"CREDIT"`},
		{"BalanceScope", `"unknown"`},
		{"Alias", `"@source#other"`},
		{"Key", `"@source#default"`},
	} {
		t.Run(tc.field+tc.value, func(t *testing.T) {
			fields := codecFields(t, FormatDual)
			fields[tc.field] = json.RawMessage(tc.value)
			snapshot, err := Decode(marshalCodecFields(t, fields))
			require.Error(t, err)
			require.Equal(t, engine.BalanceSnapshot{}, snapshot)
		})
	}
}

func TestCodecRejectsMalformedShapesAndDuplicateFields(t *testing.T) {
	valid, err := Encode(codecSnapshot(), FormatDual)
	require.NoError(t, err)
	for _, raw := range []string{
		`null`, `[]`, `{}`, `"balance"`, string(valid) + `{}`,
		`{"ID":"a","ID":"b"}`, `{"ID":"a","\u0049D":"b"}`,
		`{"Extension":NaN}`, `{"Extension":01}`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := Decode([]byte(raw))
			require.Error(t, err)
		})
	}
}

func TestCodecUnknownExtensionsDoNotChangeKnownFields(t *testing.T) {
	fields := codecFields(t, FormatDual)
	fields["Extension"] = json.RawMessage(`{"counter":9007199254740993,"items":[],"raw":"1e3"}`)
	snapshot, err := Decode(marshalCodecFields(t, fields))
	require.NoError(t, err)
	require.Equal(t, codecSnapshot(), snapshot)
}

func TestCodecLegacyColdSeedRequiresTrustedIdentityCompletion(t *testing.T) {
	fields := codecFields(t, FormatDual)
	delete(fields, "Alias")
	delete(fields, "alias")
	snapshot, err := Decode(marshalCodecFields(t, fields))
	require.NoError(t, err)
	require.Empty(t, snapshot.Alias)
	require.Empty(t, snapshot.BalanceRef)
	require.Equal(t, codecSnapshot().ID, snapshot.ID)
	require.Equal(t, codecSnapshot().AccountID, snapshot.AccountID)
	require.Equal(t, "default", snapshot.Key)
	require.Equal(t, "USD", snapshot.AssetCode)
	_, err = Encode(snapshot, FormatDual)
	require.Error(t, err, "a writer cannot publish an unresolved identity")

	snapshot.Alias = codecSnapshot().Alias
	snapshot.BalanceRef = codecSnapshot().BalanceRef
	_, err = Encode(snapshot, FormatDual)
	require.NoError(t, err)

	fields["Alias"] = json.RawMessage(`""`)
	_, err = Decode(marshalCodecFields(t, fields))
	require.Error(t, err, "present but empty legacy alias is not absence")
	fields = codecFields(t, FormatNewOnly)
	delete(fields, "alias")
	_, err = Decode(marshalCodecFields(t, fields))
	require.Error(t, err, "new-only blobs must supply complete logical identity")
}

func TestCodecQualifiedAliasAndInvalidEncode(t *testing.T) {
	snapshot := codecSnapshot()
	snapshot.Alias = "@source#default"
	raw, err := Encode(snapshot, FormatNewOnly)
	require.NoError(t, err)
	decoded, err := Decode(raw)
	require.NoError(t, err)
	require.Equal(t, "@source", decoded.Alias)
	require.Equal(t, "@source#default", decoded.BalanceRef)

	_, err = Encode(snapshot, Format(0))
	require.Error(t, err)
	snapshot.BalanceRef = "@other#default"
	_, err = Encode(snapshot, FormatDual)
	require.Error(t, err)
	require.False(t, errors.As(err, new(*NoncanonicalLimitError)))
}
