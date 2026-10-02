// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	trcModel "github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestFirstSourceAccount(t *testing.T) {
	t.Parallel()

	balances := []*mmodel.Balance{
		{Alias: "@alice", Key: "default", AccountID: "acc-alice", AccountType: "deposit"},
		{Alias: "@bob", Key: "default", AccountID: "acc-bob", AccountType: "creditCard"},
		{Alias: "@alice", Key: constant.OverdraftBalanceKey, AccountID: "acc-alice-overdraft", AccountType: "overdraft"},
		{Alias: "@carol", Key: "default", AccountID: "", AccountType: "deposit"},
	}

	t.Run("resolves the first internal source account and its type", func(t *testing.T) {
		t.Parallel()

		got := firstSourceAccount([]string{"@alice#default", "@bob#default"}, balances)
		assert.Equal(t, tracer.ReserveAccount{AccountID: "acc-alice", Type: "deposit"}, got)
	})

	t.Run("skips the overdraft companion alias", func(t *testing.T) {
		t.Parallel()

		got := firstSourceAccount([]string{"@alice#overdraft", "@bob#default"}, balances)
		assert.Equal(t, tracer.ReserveAccount{AccountID: "acc-bob", Type: "creditCard"}, got,
			"companion sources must not be chosen as the account scope")
	})

	t.Run("skips a balance with no account id", func(t *testing.T) {
		t.Parallel()

		got := firstSourceAccount([]string{"@carol#default", "@bob#default"}, balances)
		assert.Equal(t, tracer.ReserveAccount{AccountID: "acc-bob", Type: "creditCard"}, got)
	})

	t.Run("returns the zero account when no internal source resolves", func(t *testing.T) {
		t.Parallel()

		got := firstSourceAccount([]string{"@external/BRL#default"}, balances)
		assert.Equal(t, tracer.ReserveAccount{}, got, "an external-only source has no internal account scope")
	})

	t.Run("empty inputs return the zero account", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, tracer.ReserveAccount{}, firstSourceAccount(nil, balances))
		assert.Equal(t, tracer.ReserveAccount{}, firstSourceAccount([]string{"@alice#default"}, nil))
	})
}

func TestReserveMetadata(t *testing.T) {
	t.Parallel()

	t.Run("nil and empty metadata send nothing", func(t *testing.T) {
		t.Parallel()

		got, dropped := reserveMetadata(nil)
		assert.Nil(t, got)
		assert.Zero(t, dropped)

		got, dropped = reserveMetadata(map[string]any{})
		assert.Nil(t, got)
		assert.Zero(t, dropped)
	})

	t.Run("scalar values are rendered as strings", func(t *testing.T) {
		t.Parallel()

		got, dropped := reserveMetadata(map[string]any{
			"s":       "text",
			"b":       true,
			"i":       7,
			"i64":     int64(-42),
			"f":       1.5,
			"f_large": 1e21,
			"f_int":   float64(10),
			"n":       json.Number("12.50"),
			"d":       decimal.RequireFromString("0.000001"),
		})

		assert.Zero(t, dropped)
		assert.Equal(t, map[string]string{
			"s":       "text",
			"b":       "true",
			"i":       "7",
			"i64":     "-42",
			"f":       "1.5",
			"f_large": "1000000000000000000000",
			"f_int":   "10",
			"n":       "12.5",
			"d":       "0.000001",
		}, got)
	})

	t.Run("keys the tracer refuses and non-scalar values are dropped and counted", func(t *testing.T) {
		t.Parallel()

		got, dropped := reserveMetadata(map[string]any{
			"ok":                     "kept",
			"bad-key":                "x",
			"with space":             "x",
			"":                       "x",
			strings.Repeat("k", 65):  "x",
			strings.Repeat("k", 64):  "kept",
			"nil_value":              nil,
			"map_value":              map[string]any{"a": 1},
			"slice_value":            []any{1, 2},
			"struct_value":           struct{}{},
			"malformed_number_value": json.Number("not-a-number"),
		})

		assert.Equal(t, map[string]string{"ok": "kept", strings.Repeat("k", 64): "kept"}, got)
		assert.Equal(t, 9, dropped)
	})

	t.Run("only dropped entries send nothing", func(t *testing.T) {
		t.Parallel()

		got, dropped := reserveMetadata(map[string]any{"bad-key": "x"})
		assert.Nil(t, got, "an all-dropped map must serialize as absent, not as {}")
		assert.Equal(t, 1, dropped)
	})

	t.Run("caps at the tracer entry limit keeping the lexicographically first keys", func(t *testing.T) {
		t.Parallel()

		md := make(map[string]any, 60)
		for i := range 60 {
			md[fmt.Sprintf("key_%02d", i)] = "v"
		}

		got, dropped := reserveMetadata(md)

		require.Len(t, got, 50)
		assert.Equal(t, 10, dropped)

		for i := range 50 {
			assert.Contains(t, got, fmt.Sprintf("key_%02d", i))
		}

		for i := 50; i < 60; i++ {
			assert.NotContains(t, got, fmt.Sprintf("key_%02d", i))
		}
	})
}

func TestReserveTransaction_RecordsDroppedMetadata(t *testing.T) {
	t.Parallel()

	ctx, span, ended := recordingSpan(t)
	capturing := &capturingReserver{result: &tracer.ReserveResult{}}
	uc := &UseCase{TracerReserver: capturing}

	uc.reserveTransaction(ctx, span, &libLog.NopLogger{},
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccount,
		map[string]any{"ok": "v", "bad-key": "v", "nested": map[string]any{}}, fixedReserveTimestamp, reservationTTLDefault, reservationForCreate, false)

	attrs := spanAttributes(ended())
	assert.Equal(t, int64(2), attrs["app.tracer.metadata_dropped"].AsInt64())
	assert.Equal(t, map[string]string{"ok": "v"}, capturing.lastReq.Metadata)
}

// TestReserveMetadata_MatchesTracerBounds locks the anchor's metadata filter to
// the tracer's own reserve validation. The tracer refuses the WHOLE reserve when
// one key breaks its bounds, which an enforce ledger answers with 0532, so a
// bound that drifts on either side turns ordinary metadata into a rejected
// transaction.
func TestReserveMetadata_MatchesTracerBounds(t *testing.T) {
	t.Parallel()

	assert.Equal(t, trcConstant.MaxMetadataEntries, reserveMetadataMaxEntries, "entry cap drifted from the tracer")
	assert.Equal(t, trcConstant.MaxMetadataKeyLength, reserveMetadataMaxKeyLength, "key length drifted from the tracer")

	tracerAccepts := func(t *testing.T, md map[string]any) bool {
		t.Helper()

		req := trcModel.ValidationRequest{
			RequestID:            uuid.MustParse("66666666-6666-4666-8666-666666666666"),
			Amount:               decimal.NewFromInt(1),
			Asset:                "BRL",
			TransactionTimestamp: fixedReserveTimestamp,
			Metadata:             md,
		}

		return req.ValidateForReserve(fixedReserveTimestamp) == nil
	}

	keys := []string{
		"ok", "OK_1", "_", "123", "snake_case_key",
		"", "bad-key", "with space", "dot.key", "ümlaut", "tab\tkey", "emoji_😀",
		strings.Repeat("k", reserveMetadataMaxKeyLength),
		strings.Repeat("k", reserveMetadataMaxKeyLength+1),
	}

	for _, key := range keys {
		t.Run(fmt.Sprintf("key %q", key), func(t *testing.T) {
			t.Parallel()

			forwarded, _ := reserveMetadata(map[string]any{key: "v"})
			_, ledgerKeeps := forwarded[key]

			assert.Equal(t, tracerAccepts(t, map[string]any{key: "v"}), ledgerKeeps,
				"the anchor must forward a key exactly when the tracer accepts it")
		})
	}

	t.Run("the capped map is accepted and one more entry is not", func(t *testing.T) {
		t.Parallel()

		md := make(map[string]any, reserveMetadataMaxEntries+1)
		for i := range reserveMetadataMaxEntries + 1 {
			md[fmt.Sprintf("key_%03d", i)] = "v"
		}

		forwarded, dropped := reserveMetadata(md)
		require.Equal(t, 1, dropped)

		asAny := make(map[string]any, len(forwarded))
		for k, v := range forwarded {
			asAny[k] = v
		}

		assert.True(t, tracerAccepts(t, asAny), "the tracer must accept everything the anchor forwards")
		assert.False(t, tracerAccepts(t, md), "the tracer must refuse one entry past the cap")
	})
}
