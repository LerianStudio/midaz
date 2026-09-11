// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestTransitionCrossGate_OppositeMarkerKeyByStatus pins which statuses arm the
// cross-transition gate. Only the two terminal transitions of a pending can
// re-execute each other, so only they resolve an opposite marker key; every
// other status resolves the empty string, which the script reads as "gate off"
// and answers with no extra EXISTS.
func TestTransitionCrossGate_OppositeMarkerKeyByStatus(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	transactionID := "33333333-3333-3333-3333-333333333333"

	testCases := []struct {
		name           string
		status         string
		wantOppositeOf string
	}{
		{name: "commit reads the cancel marker", status: constant.APPROVED, wantOppositeOf: constant.CANCELED},
		{name: "cancel reads the commit marker", status: constant.CANCELED, wantOppositeOf: constant.APPROVED},
		{name: "pending create arms nothing", status: constant.PENDING},
		{name: "direct create arms nothing", status: constant.CREATED},
		{name: "annotation arms nothing", status: constant.NOTED},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := utils.TransactionApplyMarkerOppositeKey(organizationID, ledgerID, transactionID, tc.status)

			if tc.wantOppositeOf == "" {
				assert.Empty(t, got, "status %q has no opposite terminal transition", tc.status)

				return
			}

			want := utils.TransactionApplyMarkerKey(organizationID, ledgerID, transactionID, tc.wantOppositeOf)

			assert.Equal(t, want, got)
			assert.NotEqual(t, utils.TransactionApplyMarkerKey(organizationID, ledgerID, transactionID, tc.status), got,
				"the gate must read the OPPOSITE marker, never this execution's own")
		})
	}
}

// TestTransitionCrossGate_HeaderCarriesTheOppositeMarker proves the opposite
// marker key reaches the script in the fixed header slot the Lua side reads it
// from, on both the grant and no-grant paths, and that an unarmed status leaves
// that slot empty instead of shrinking the header.
func TestTransitionCrossGate_HeaderCarriesTheOppositeMarker(t *testing.T) {
	t.Parallel()

	const (
		markerKey         = "transaction_apply_marker:{transactions}:o:l:t:APPROVED"
		oppositeMarkerKey = "transaction_apply_marker:{transactions}:o:l:t:CANCELED"
	)

	t.Run("armed", func(t *testing.T) {
		t.Parallel()

		var absent *accountBlockExceptionEval

		args := make([]any, absent.headerWidth())
		absent.writeHeader(args, markerKey, oppositeMarkerKey)

		require.Len(t, args, luaArgsHeaderFixedSize)
		assert.Equal(t, oppositeMarkerKey, args[4], "the opposite marker rides in the fifth fixed slot")
	})

	t.Run("unarmed keeps the slot", func(t *testing.T) {
		t.Parallel()

		eval := &accountBlockExceptionEval{
			alias:       "@source",
			amount:      "100",
			balanceKeys: []string{"balance:{transactions}:o:l:@source#default"},
		}

		args := make([]any, eval.headerWidth())
		eval.writeHeader(args, markerKey, "")

		require.Len(t, args, luaArgsHeaderFixedSize+1)
		assert.Equal(t, "", args[4], "an unarmed gate leaves the slot empty; it never shrinks the header")
		assert.Equal(t, "balance:{transactions}:o:l:@source#default", args[luaArgsHeaderFixedSize],
			"the bypassed balance keys still start right after the fixed header")
	})
}

// TestMapError_TransactionAlreadyTransitioned proves the script's cross-gate
// rejection crosses the Go↔Redis↔Lua boundary as the typed 409 business error,
// bare or wrapped in a redis error message.
func TestMapError_TransactionAlreadyTransitioned(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		luaErr string
	}{
		{name: "bare code", luaErr: "0511"},
		{name: "prefixed message", luaErr: "ERR 0511 opposite terminal transition already applied"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tracer := noop.NewTracerProvider().Tracer("test")
			_, span := tracer.Start(t.Context(), "test")

			defer span.End()

			mapped := mapBalanceAtomicScriptError(span, errors.New(tc.luaErr))

			expected := pkg.ValidateBusinessError(constant.ErrTransactionAlreadyTransitioned, constant.EntityTransaction)
			assert.Equal(t, expected.Error(), mapped.Error())

			var conflict pkg.EntityConflictError

			require.ErrorAs(t, mapped, &conflict, "0511 is a conflict, so the transport answers 409")
			assert.Equal(t, constant.ErrTransactionAlreadyTransitioned.Error(), conflict.Code)
		})
	}
}
