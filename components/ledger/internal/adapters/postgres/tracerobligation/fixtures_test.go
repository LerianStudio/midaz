// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracerobligation

import (
	"os"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func obligationFixture(t *testing.T) (tracerreservation.Intent, tracerreservation.Config) {
	t.Helper()
	cfg := tracerreservation.Config{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxBodyBytes: 65536}
	raw, err := os.ReadFile("../../../../../../pkg/tracercontract/testdata/reserve_request.json")
	require.NoError(t, err)
	request, err := tracercontract.DecodeReserveJSON(t.Context(), raw, cfg.MaxBodyBytes, cfg.Bounds)
	require.NoError(t, err)
	key := tracerreservation.Key{OrganizationID: uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338"), LedgerID: uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10"), TransactionID: request.TransactionID}
	request.ContextID = key.LedgerID.String()
	instant := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	intent, err := tracerreservation.NewIntent(t.Context(), key, uuid.MustParse("5639dfb6-862e-4c2c-8a91-1f4f3ff54c9a"), tracercontract.ReserveScope{TenantID: "tenant-a", IntegrationID: "producer", AssetNamespace: "origin-a"}, request, instant, instant.Add(time.Second), cfg)
	require.NoError(t, err)
	return intent, cfg
}
