// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// operationRouteIds stays the full link set, as consumers of 1.1.0 read it, and
// optionalOperationRouteIds names the subset a transaction may leave unused.
func TestTransactionRouteEvents_CarryOptionalLinksAsASubset(t *testing.T) {
	tr := minimalTransactionRoute()
	tr.OptionalOperationRouteIDs = append(tr.OptionalOperationRouteIDs, transactionRouteOR2)

	all := []string{transactionRouteOR1.String(), transactionRouteOR2.String()}
	optional := []string{transactionRouteOR2.String()}

	created := events.NewTransactionRouteCreated(tr)
	assert.Equal(t, all, created.OperationRouteIDs)
	assert.Equal(t, optional, created.OptionalOperationRouteIDs)

	updated := events.NewTransactionRouteUpdated(tr)
	assert.Equal(t, all, updated.OperationRouteIDs)
	assert.Equal(t, optional, updated.OptionalOperationRouteIDs)
}

// An update that only moves links between required and optional keeps
// operationRouteIds and changes the optional subset.
func TestTransactionRouteUpdated_RetagChangesOnlyTheOptionalSubset(t *testing.T) {
	before := minimalTransactionRoute()
	before.OptionalOperationRouteIDs = append(before.OptionalOperationRouteIDs, transactionRouteOR2)

	after := minimalTransactionRoute()
	after.OptionalOperationRouteIDs = append(after.OptionalOperationRouteIDs, transactionRouteOR1)

	first, second := events.NewTransactionRouteUpdated(before), events.NewTransactionRouteUpdated(after)

	assert.Equal(t, first.OperationRouteIDs, second.OperationRouteIDs)
	assert.Equal(t, []string{transactionRouteOR1.String()}, second.OptionalOperationRouteIDs)
}

func TestTransactionRouteEvents_JSONShape_OptionalLinksAddOneField(t *testing.T) {
	wire := func(payload any) map[string]any {
		data, err := json.Marshal(payload)
		require.NoError(t, err)

		var generic map[string]any
		require.NoError(t, json.Unmarshal(data, &generic))

		return generic
	}

	required := minimalTransactionRoute()

	withOptional := minimalTransactionRoute()
	withOptional.OptionalOperationRouteIDs = append(withOptional.OptionalOperationRouteIDs, transactionRouteOR2)

	for name, tc := range map[string]struct {
		required, optional map[string]any
		fields             int
	}{
		"transaction_route.created": {wire(events.NewTransactionRouteCreated(required)), wire(events.NewTransactionRouteCreated(withOptional)), 7},
		"transaction_route.updated": {wire(events.NewTransactionRouteUpdated(required)), wire(events.NewTransactionRouteUpdated(withOptional)), 6},
	} {
		t.Run(name, func(t *testing.T) {
			_, present := tc.required["optionalOperationRouteIds"]
			assert.False(t, present, "a route without optional links keeps the 1.1.0 wire shape")
			assert.Len(t, tc.required, tc.fields)

			assert.Equal(t, []any{transactionRouteOR2.String()}, tc.optional["optionalOperationRouteIds"])
			assert.Len(t, tc.optional, tc.fields+1)
		})
	}
}
