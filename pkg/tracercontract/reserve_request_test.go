// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func reserveRequest(t *testing.T) tracercontract.ReserveRequest {
	t.Helper()
	data, err := os.ReadFile("testdata/reserve_request.json")
	require.NoError(t, err)
	var request tracercontract.ReserveRequest
	require.NoError(t, json.Unmarshal(data, &request))
	require.Len(t, request.Context.Accounts, 1)
	require.Len(t, request.Context.Entries, 2)
	return request
}

func reserveScope() tracercontract.ReserveScope {
	return tracercontract.ReserveScope{TenantID: "tenant-a", IntegrationID: "integration-a", AssetNamespace: "origin-a"}
}

func TestReserveFingerprintGolden(t *testing.T) {
	t.Parallel()
	// Frozen preimages/hashes were calculated independently with Python struct,
	// uuid and hashlib, not serialized by the implementation under test.
	var vectors []struct {
		Name         string `json:"name"`
		TenantID     string `json:"tenantId"`
		SingleTenant bool   `json:"singleTenant"`
		Unicode      bool   `json:"unicode"`
		PreimageHex  string `json:"preimageHex"`
		SHA256       string `json:"sha256"`
	}
	data, err := os.ReadFile("testdata/reserve_fingerprints.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &vectors))
	require.Len(t, vectors, 3)
	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			scope := reserveScope()
			scope.TenantID, scope.SingleTenant = vector.TenantID, vector.SingleTenant
			preimage, err := hex.DecodeString(vector.PreimageHex)
			require.NoError(t, err)
			expected := sha256.Sum256(preimage)
			require.Equal(t, vector.SHA256, hex.EncodeToString(expected[:]))
			request := reserveRequest(t)
			if vector.Unicode {
				request.ContextID = "livro/ação"
				request.Context.Accounts[0].Type = "depósito"
			}
			before, err := json.Marshal(request)
			require.NoError(t, err)
			actual, err := request.Fingerprint(t.Context(), scope, testLimits())
			require.NoError(t, err)
			require.Equal(t, expected, actual)
			after, err := json.Marshal(request)
			require.NoError(t, err)
			require.Equal(t, before, after, "fingerprinting leaves all original facts unchanged")
		})
	}
}

func TestReserveFingerprintEquivalentRepresentations(t *testing.T) {
	t.Parallel()
	original := reserveRequest(t)
	expected, err := original.Fingerprint(t.Context(), reserveScope(), testLimits())
	require.NoError(t, err)
	encoded, err := json.Marshal(original)
	require.NoError(t, err)
	// UUID casing and equivalent offsets normalize when decoded; decimal scale
	// normalizes without ever passing through a floating-point representation.
	encoded = []byte(strings.ReplaceAll(string(encoded), "550e8400-e29b-41d4-a716-446655440001", "550E8400-E29B-41D4-A716-446655440001"))
	encoded = []byte(strings.ReplaceAll(string(encoded), "2026-09-24T12:00:00.123456789Z", "2026-09-24T09:00:00.123456789-03:00"))
	var decoded tracercontract.ReserveRequest
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	decoded.Amount += "00"
	decoded.Context.Entries[0].Amount += "00"
	actual, err := decoded.Fingerprint(t.Context(), reserveScope(), testLimits())
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	require.NotEqual(t, original.Amount, decoded.Amount, "fingerprinting must not rewrite the input")
	require.Equal(t, "-03:00", decoded.TransactionTimestamp.Format("Z07:00"))
	largerLimits := testLimits()
	largerLimits.MaxEntries *= 2
	actual, err = decoded.Fingerprint(t.Context(), reserveScope(), largerLimits)
	require.NoError(t, err)
	require.Equal(t, expected, actual, "resource configuration is not request identity")
}

func TestReserveRequestRejectsIncompleteOrInvalidFacts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*tracercontract.ReserveRequest)
	}{
		{"missing revision", func(r *tracercontract.ReserveRequest) { r.ContractRevision = "" }},
		{"unknown revision", func(r *tracercontract.ReserveRequest) { r.ContractRevision = "context-reserve-2" }},
		{"missing transaction", func(r *tracercontract.ReserveRequest) { r.TransactionID = uuid.Nil }},
		{"missing request", func(r *tracercontract.ReserveRequest) { r.RequestID = uuid.Nil }},
		{"missing context id", func(r *tracercontract.ReserveRequest) { r.ContextID = "" }},
		{"context id too long", func(r *tracercontract.ReserveRequest) { r.ContextID = strings.Repeat("a", 257) }},
		{"context whitespace", func(r *tracercontract.ReserveRequest) { r.ContextID = " ledger" }},
		{"context nul", func(r *tracercontract.ReserveRequest) { r.ContextID = "ledger\x00a" }},
		{"invalid utf8", func(r *tracercontract.ReserveRequest) { r.ContextID = "ledger\xff" }},
		{"missing mode", func(r *tracercontract.ReserveRequest) { r.ValidationMode = "" }},
		{"unsupported mode", func(r *tracercontract.ReserveRequest) { r.ValidationMode = "rules" }},
		{"missing timestamp", func(r *tracercontract.ReserveRequest) { r.TransactionTimestamp = time.Time{} }},
		{"timestamp outside rfc3339", func(r *tracercontract.ReserveRequest) {
			r.TransactionTimestamp = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		}},
		{"missing long lived", func(r *tracercontract.ReserveRequest) { r.LongLived = nil }},
		{"missing amount", func(r *tracercontract.ReserveRequest) { r.Amount = "" }},
		{"negative amount", func(r *tracercontract.ReserveRequest) { r.Amount = "-1" }},
		{"zero amount", func(r *tracercontract.ReserveRequest) { r.Amount = "0.00" }},
		{"exponent amount", func(r *tracercontract.ReserveRequest) { r.Amount = "1e999999999" }},
		{"oversized amount", func(r *tracercontract.ReserveRequest) { r.Amount = tracercontract.Amount(strings.Repeat("9", 129)) }},
		{"missing asset", func(r *tracercontract.ReserveRequest) { r.Asset = tracercontract.AssetRef{} }},
		{"forged namespace", func(r *tracercontract.ReserveRequest) { r.Asset.Namespace = "forged" }},
		{"contradictory root code", func(r *tracercontract.ReserveRequest) { r.Asset.Code = "USD" }},
		{"invalid asset text", func(r *tracercontract.ReserveRequest) { r.Asset.ID = "asset\x00btc" }},
		{"invalid account text", func(r *tracercontract.ReserveRequest) { r.Context.Accounts[0].Type = "deposit\xff" }},
		{"missing accounts array", func(r *tracercontract.ReserveRequest) { r.Context.Accounts = nil }},
		{"missing entries array", func(r *tracercontract.ReserveRequest) { r.Context.Entries = nil }},
		{"too many accounts", func(r *tracercontract.ReserveRequest) { r.Context.Accounts = make([]tracercontract.Account, 11) }},
		{"too many entries", func(r *tracercontract.ReserveRequest) { r.Context.Entries = make([]tracercontract.Entry, 21) }},
		{"absent blocked", func(r *tracercontract.ReserveRequest) { r.Context.Accounts[0].Blocked = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reserveRequest(t)
			tt.change(&r)
			require.ErrorIs(t, r.Validate(t.Context(), "origin-a", testLimits()), constant.ErrInvalidRequestBody)
			fingerprint, err := r.Fingerprint(t.Context(), reserveScope(), testLimits())
			require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
			require.Zero(t, fingerprint, "invalid requests cannot yield a partial fingerprint")
		})
	}
}

func TestReserveRequestPresenceAndReplayTimestamp(t *testing.T) {
	t.Parallel()
	r := reserveRequest(t)
	r.ValidationMode = tracercontract.ValidationLimits
	r.ContextID = strings.Repeat("a", 256)
	// Structural validation must not apply a freshness window to stored replays.
	r.TransactionTimestamp = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.Validate(t.Context(), "origin-a", testLimits()))
	encoded, err := json.Marshal(r)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"longLived":false`)
	for _, replacement := range []string{``, `"longLived":null,`} {
		var absent tracercontract.ReserveRequest
		require.NoError(t, json.Unmarshal([]byte(strings.Replace(string(encoded), `"longLived":false,`, replacement, 1)), &absent))
		require.ErrorIs(t, absent.Validate(t.Context(), "origin-a", testLimits()), constant.ErrInvalidRequestBody)
	}
	// An explicit empty array is valid for external-only transactions.
	r.Context.Accounts = []tracercontract.Account{}
	r.Context.Entries = r.Context.Entries[1:]
	require.NoError(t, r.Validate(t.Context(), "origin-a", testLimits()))
	r.Context.Accounts = nil
	require.ErrorIs(t, r.Validate(t.Context(), "origin-a", testLimits()), constant.ErrInvalidRequestBody)
}

func TestReserveFingerprintDetectsContentChanges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*tracercontract.ReserveRequest)
	}{
		{"transaction", func(r *tracercontract.ReserveRequest) { r.TransactionID[0]++ }},
		{"request", func(r *tracercontract.ReserveRequest) { r.RequestID[0]++ }},
		{"context", func(r *tracercontract.ReserveRequest) { r.ContextID = "ledger-b" }},
		{"mode", func(r *tracercontract.ReserveRequest) { r.ValidationMode = tracercontract.ValidationLimits }},
		{"timestamp", func(r *tracercontract.ReserveRequest) {
			r.TransactionTimestamp = r.TransactionTimestamp.Add(time.Nanosecond)
		}},
		{"long lived", func(r *tracercontract.ReserveRequest) { *r.LongLived = true }},
		{"amount", func(r *tracercontract.ReserveRequest) { r.Amount = "9007199254740993.00000002" }},
		{"root asset identity", func(r *tracercontract.ReserveRequest) { r.Asset.ID = "other" }},
		{"account asset identity", func(r *tracercontract.ReserveRequest) {
			r.Context.Accounts[0].Asset.ID = "other"
			r.Context.Entries[0].Asset.ID = "other"
		}},
		{"asset code", func(r *tracercontract.ReserveRequest) {
			r.Asset.Code = "XBT"
			r.Context.Accounts[0].Asset.Code = "XBT"
			for i := range r.Context.Entries {
				r.Context.Entries[i].Asset.Code = "XBT"
			}
		}},
		{"account id", func(r *tracercontract.ReserveRequest) {
			r.Context.Accounts[0].ID[0]++
			r.Context.Entries[0].AccountID = r.Context.Accounts[0].ID
		}},
		{"account type", func(r *tracercontract.ReserveRequest) { r.Context.Accounts[0].Type = "current_assets" }},
		{"account status", func(r *tracercontract.ReserveRequest) { r.Context.Accounts[0].Status = "INACTIVE" }},
		{"blocked", func(r *tracercontract.ReserveRequest) { *r.Context.Accounts[0].Blocked = true }},
		{"direction", func(r *tracercontract.ReserveRequest) { r.Context.Entries[1].Direction = tracercontract.Debit }},
		{"entry amount", func(r *tracercontract.ReserveRequest) { r.Context.Entries[1].Amount = "0.00000002" }},
		{"entry asset", func(r *tracercontract.ReserveRequest) { r.Context.Entries[1].Asset.ID = "other" }},
		{"entry order", func(r *tracercontract.ReserveRequest) {
			r.Context.Entries[0], r.Context.Entries[1] = r.Context.Entries[1], r.Context.Entries[0]
		}},
		{"entry multiplicity", func(r *tracercontract.ReserveRequest) {
			r.Context.Entries = append(r.Context.Entries, r.Context.Entries[0])
		}},
		{"participant", func(r *tracercontract.ReserveRequest) {
			r.Context.Entries[1].External = false
			r.Context.Entries[1].AccountID = r.Context.Accounts[0].ID
		}},
	}
	expected, err := reserveRequest(t).Fingerprint(t.Context(), reserveScope(), testLimits())
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reserveRequest(t)
			tt.change(&r)
			actual, err := r.Fingerprint(t.Context(), reserveScope(), testLimits())
			require.NoError(t, err)
			require.NotEqual(t, expected, actual)
		})
	}
}

func TestReserveFingerprintPreservesAccountOrder(t *testing.T) {
	t.Parallel()
	r := reserveRequest(t)
	second := r.Context.Accounts[0]
	second.ID[0]++
	r.Context.Accounts = append(r.Context.Accounts, second)
	r.Context.Entries[1].External = false
	r.Context.Entries[1].AccountID = second.ID
	a, err := r.Fingerprint(t.Context(), reserveScope(), testLimits())
	require.NoError(t, err)
	r.Context.Accounts[0], r.Context.Accounts[1] = r.Context.Accounts[1], r.Context.Accounts[0]
	b, err := r.Fingerprint(t.Context(), reserveScope(), testLimits())
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestReserveFingerprintIncludesPrincipalAssetCode(t *testing.T) {
	t.Parallel()
	r := reserveRequest(t)
	// A principal asset distinct from entry assets has no sum/scale relationship
	// imposed by this structural contract. Its display code still affects replay.
	r.Asset.ID = "principal-asset"
	a, err := r.Fingerprint(t.Context(), reserveScope(), testLimits())
	require.NoError(t, err)
	r.Asset.Code = "TOKEN"
	b, err := r.Fingerprint(t.Context(), reserveScope(), testLimits())
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestReserveFingerprintTrustedScope(t *testing.T) {
	t.Parallel()
	r := reserveRequest(t)
	original, err := r.Fingerprint(t.Context(), reserveScope(), testLimits())
	require.NoError(t, err)
	for _, field := range []string{"tenant", "integration", "namespace"} {
		t.Run(field, func(t *testing.T) {
			r := reserveRequest(t)
			scope := reserveScope()
			switch field {
			case "tenant":
				scope.TenantID = "tenant-b"
			case "integration":
				scope.IntegrationID = "integration-b"
			case "namespace":
				scope.AssetNamespace = "origin-b"
				r.Asset.Namespace = scope.AssetNamespace
				r.Context.Accounts[0].Asset.Namespace = scope.AssetNamespace
				for i := range r.Context.Entries {
					r.Context.Entries[i].Asset.Namespace = scope.AssetNamespace
				}
			}
			actual, err := r.Fingerprint(t.Context(), scope, testLimits())
			require.NoError(t, err)
			require.NotEqual(t, original, actual)
		})
	}
	for _, scope := range []tracercontract.ReserveScope{
		{},
		{IntegrationID: "integration-a", AssetNamespace: "origin-a"},
		{TenantID: "tenant-a", AssetNamespace: "origin-a"},
		{TenantID: "tenant-a", IntegrationID: "integration-a", AssetNamespace: "forged"},
		{TenantID: "tenant\x00a", IntegrationID: "integration-a", AssetNamespace: "origin-a"},
		{TenantID: "tenant-a", IntegrationID: "integration\xff", AssetNamespace: "origin-a"},
		{TenantID: "tenant-a", IntegrationID: strings.Repeat("a", 257), AssetNamespace: "origin-a"},
	} {
		actual, err := r.Fingerprint(t.Context(), scope, testLimits())
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
		require.Zero(t, actual)
	}
	a, err := r.Fingerprint(t.Context(), tracercontract.ReserveScope{TenantID: "ab", IntegrationID: "c", AssetNamespace: "origin-a"}, testLimits())
	require.NoError(t, err)
	b, err := r.Fingerprint(t.Context(), tracercontract.ReserveScope{TenantID: "a", IntegrationID: "bc", AssetNamespace: "origin-a"}, testLimits())
	require.NoError(t, err)
	require.NotEqual(t, a, b, "field boundaries must not collide")
}

func TestReserveFingerprintCancellationAndInvalidLimits(t *testing.T) {
	t.Parallel()
	r := reserveRequest(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	actual, err := r.Fingerprint(ctx, reserveScope(), testLimits())
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, actual)
	actual, err = r.Fingerprint(t.Context(), reserveScope(), tracercontract.Limits{})
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	require.Zero(t, actual)
}

func TestReserveFingerprintRejectsFramingOverflow(t *testing.T) {
	t.Parallel()
	// Reject oversized configuration without allocating an oversized request.
	maximumInt := int(^uint(0) >> 1)
	limits := testLimits()
	limits.MaxIntegerDigits, limits.MaxFractionDigits = maximumInt, maximumInt
	_, err := reserveRequest(t).Fingerprint(t.Context(), reserveScope(), limits)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	if strconv.IntSize < 64 {
		return
	}
	for _, field := range []string{"accounts", "entries", "text", "integer", "fraction"} {
		t.Run(field, func(t *testing.T) {
			limits := testLimits()
			switch field {
			case "accounts":
				limits.MaxAccounts = maximumInt
			case "entries":
				limits.MaxEntries = maximumInt
			case "text":
				limits.MaxTextBytes = maximumInt
			case "integer":
				limits.MaxIntegerDigits = maximumInt
			case "fraction":
				limits.MaxFractionDigits = maximumInt
			}
			actual, err := reserveRequest(t).Fingerprint(t.Context(), reserveScope(), limits)
			require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
			require.Zero(t, actual)
		})
	}
}
