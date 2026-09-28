// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// routeEventCase builds one accounting-route event payload for a route created under a
// ledger and for the same route created at organization level.
type routeEventCase struct {
	name          string
	definition    events.Definition
	withLedger    func() any
	withoutLedger func() any
	fieldsLedger  int
}

func routeEventCases() []routeEventCase {
	orgLevelTransactionRoute := func() *mmodel.TransactionRoute {
		tr := minimalTransactionRoute()
		tr.LedgerID = nil

		return tr
	}

	orgLevelOperationRoute := func() *mmodel.OperationRoute {
		o := minimalOperationRoute()
		o.LedgerID = nil

		return o
	}

	return []routeEventCase{
		{
			name:          "transaction_route.created",
			definition:    events.TransactionRouteCreatedDefinition,
			withLedger:    func() any { return events.NewTransactionRouteCreated(minimalTransactionRoute()) },
			withoutLedger: func() any { return events.NewTransactionRouteCreated(orgLevelTransactionRoute()) },
			fieldsLedger:  7,
		},
		{
			name:          "transaction_route.updated",
			definition:    events.TransactionRouteUpdatedDefinition,
			withLedger:    func() any { return events.NewTransactionRouteUpdated(minimalTransactionRoute()) },
			withoutLedger: func() any { return events.NewTransactionRouteUpdated(orgLevelTransactionRoute()) },
			fieldsLedger:  6,
		},
		{
			name:       "transaction_route.deleted",
			definition: events.TransactionRouteDeletedDefinition,
			withLedger: func() any {
				return events.NewTransactionRouteDeleted(transactionRouteID.String(), transactionRouteOrg.String(), transactionRouteLed.String(), fixedTime)
			},
			withoutLedger: func() any {
				return events.NewTransactionRouteDeleted(transactionRouteID.String(), transactionRouteOrg.String(), "", fixedTime)
			},
			fieldsLedger: 4,
		},
		{
			name:          "operation_route.created",
			definition:    events.OperationRouteCreatedDefinition,
			withLedger:    func() any { return events.NewOperationRouteCreated(minimalOperationRoute()) },
			withoutLedger: func() any { return events.NewOperationRouteCreated(orgLevelOperationRoute()) },
			fieldsLedger:  7,
		},
		{
			name:          "operation_route.updated",
			definition:    events.OperationRouteUpdatedDefinition,
			withLedger:    func() any { return events.NewOperationRouteUpdated(minimalOperationRoute()) },
			withoutLedger: func() any { return events.NewOperationRouteUpdated(orgLevelOperationRoute()) },
			fieldsLedger:  6,
		},
		{
			name:       "operation_route.deleted",
			definition: events.OperationRouteDeletedDefinition,
			withLedger: func() any {
				return events.NewOperationRouteDeleted(operationRouteID.String(), operationRouteOrg.String(), operationRouteLed.String(), fixedTime)
			},
			withoutLedger: func() any {
				return events.NewOperationRouteDeleted(operationRouteID.String(), operationRouteOrg.String(), "", fixedTime)
			},
			fieldsLedger: 4,
		},
	}
}

func wireFields(t *testing.T, payload any) map[string]any {
	t.Helper()

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(data, &generic))

	return generic
}

// TestRouteEvents_LedgerIDOnlyWhenTheRouteHasALedger locks the optional ledger on the
// six accounting-route events: a route created under a ledger carries ledgerId, and a
// route created at organization level carries no ledgerId key at all (never "").
func TestRouteEvents_LedgerIDOnlyWhenTheRouteHasALedger(t *testing.T) {
	for _, tc := range routeEventCases() {
		t.Run(tc.name, func(t *testing.T) {
			withLedger := wireFields(t, tc.withLedger())
			assert.NotEmptyf(t, withLedger["ledgerId"], "a route created under a ledger must carry ledgerId")
			assert.Lenf(t, withLedger, tc.fieldsLedger, "top-level field count with a ledger (drift?)")

			withoutLedger := wireFields(t, tc.withoutLedger())
			_, hasLedger := withoutLedger["ledgerId"]
			assert.Falsef(t, hasLedger, "a route created at organization level must omit ledgerId, got %v", withoutLedger["ledgerId"])
			assert.Lenf(t, withoutLedger, tc.fieldsLedger-1, "top-level field count without a ledger (drift?)")
		})
	}
}

// TestRouteEvents_SchemaVersionAnnouncesOptionalLedger locks the minor schema bump that
// announces ledgerId as optional on every accounting-route event.
func TestRouteEvents_SchemaVersionAnnouncesOptionalLedger(t *testing.T) {
	for _, tc := range routeEventCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.name, tc.definition.Key())
			assert.Equal(t, "1.1.0", tc.definition.SchemaVersion)
		})
	}
}
