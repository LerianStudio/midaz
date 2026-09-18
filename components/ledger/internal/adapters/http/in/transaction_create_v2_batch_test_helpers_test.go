// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const (
	batchTestOrganizationID = "00000000-0000-0000-0000-000000000001"
	batchTestLedgerID       = "00000000-0000-0000-0000-000000000002"
)

func validAtomicBatchV2Request(source, destination, ledgerID string) CreateTransactionV2Request {
	return CreateTransactionV2Request{
		Asset:   "USD",
		Amount:  "1",
		Debits:  []TransactionV2LegRequest{validAtomicBatchV2Leg(source, ledgerID)},
		Credits: []TransactionV2LegRequest{validAtomicBatchV2Leg(destination, ledgerID)},
	}
}

func validAtomicBatchV2Leg(alias, ledgerID string) TransactionV2LegRequest {
	return TransactionV2LegRequest{
		Alias:          alias,
		OrganizationID: batchTestOrganizationID,
		LedgerID:       ledgerID,
		Amount:         "1",
	}
}

func mustMarshalAtomicBatchV2Item(t *testing.T, request CreateTransactionV2Request) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(request)
	require.NoError(t, err)

	return raw
}

func marshalAtomicBatchV2Wrapper(t *testing.T, items ...json.RawMessage) []byte {
	t.Helper()

	raw, err := json.Marshal(map[string]any{"transactions": items})
	require.NoError(t, err)

	return raw
}

func assertAtomicBatchV2Problem(t *testing.T, err error, code string) pkgHTTP.Detail {
	t.Helper()
	require.Error(t, err)

	rendered := pkgHTTP.HumaProblem(err)
	detail, ok := rendered.(*pkgHTTP.Detail)
	require.True(t, ok)
	assert.Equal(t, code, detail.Code)

	return *detail
}

func atomicBatchV2ProblemLocations(detail pkgHTTP.Detail) []string {
	locations := make([]string, 0, len(detail.Errors))
	for _, field := range detail.Errors {
		locations = append(locations, field.Location)
	}

	return locations
}
