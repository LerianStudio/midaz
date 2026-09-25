// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracerreservation

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func intentFixture(t *testing.T) (Intent, tracercontract.ReserveRequest, Config) {
	t.Helper()
	raw, err := os.ReadFile("../../../../../pkg/tracercontract/testdata/reserve_request.json")
	require.NoError(t, err)
	cfg := Config{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxBodyBytes: 65536}
	request, err := tracercontract.DecodeReserveJSON(t.Context(), raw, cfg.MaxBodyBytes, cfg.Bounds)
	require.NoError(t, err)
	key := Key{OrganizationID: uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338"), LedgerID: uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10"), TransactionID: request.TransactionID}
	request.ContextID = key.LedgerID.String()
	scope := tracercontract.ReserveScope{TenantID: "tenant", IntegrationID: "producer", AssetNamespace: "origin-a"}
	created := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	intent, err := NewIntent(t.Context(), key, uuid.MustParse("5639dfb6-862e-4c2c-8a91-1f4f3ff54c9a"), scope, request, created, created.Add(time.Second), cfg)
	require.NoError(t, err)
	return intent, request, cfg
}

func TestIntentFreezesScopedRequest(t *testing.T) {
	intent, request, cfg := intentFixture(t)
	decoded, err := intent.Request(t.Context(), cfg)
	require.NoError(t, err)
	require.Equal(t, request, decoded)
	request.Context.Accounts[0].Type = "changed"
	again, err := intent.Request(t.Context(), cfg)
	require.NoError(t, err)
	require.NotEqual(t, request.Context.Accounts[0].Type, again.Context.Accounts[0].Type)
	require.Error(t, intent.Validate(t.Context(), Config{}))
	for _, change := range []func(*Intent){
		func(i *Intent) { i.Key.TransactionID = uuid.Nil },
		func(i *Intent) { i.Key.LedgerID = i.Key.OrganizationID },
		func(i *Intent) { i.ExecutionID = uuid.Nil },
		func(i *Intent) { i.Scope.AssetNamespace = "forged" },
		func(i *Intent) { i.Scope.IntegrationID = "other" },
		func(i *Intent) { i.Scope.TenantID = "other" },
		func(i *Intent) { i.Fingerprint[0] ^= 1 },
		func(i *Intent) { i.Payload = []byte(`{}`) },
		func(i *Intent) { i.PrepareDeadline = i.CreatedAt },
	} {
		altered := intent
		change(&altered)
		require.Error(t, altered.Validate(t.Context(), cfg))
	}
}

func TestObligationStateTransitions(t *testing.T) {
	for _, tc := range []struct {
		from, to State
		allowed  bool
	}{
		{Prepared, Executing, true},
		{Prepared, Released, true},
		{Prepared, Confirmed, false},
		{Executing, Confirmed, true},
		{Executing, Released, true},
		{Executing, Prepared, false},
		{Confirmed, Confirmed, true},
		{Confirmed, Released, false},
		{Released, Confirmed, false},
		{Released, Released, true},
		{Confirmed, Executing, false},
		{State("invalid"), Released, false},
	} {
		t.Run(string(tc.from)+"/"+string(tc.to), func(t *testing.T) { require.Equal(t, tc.allowed, tc.from.CanTransition(tc.to)) })
	}
}
