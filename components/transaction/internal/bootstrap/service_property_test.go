// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

// =============================================================================
// PROPERTY-BASED TESTS -- Service getter domain invariants (T-005)
//
// These tests verify that the domain invariants of the three opaque-handle
// getters on Service hold across hundreds of automatically-generated inputs.
//
// Invariants verified:
//   1. GettersReturnExactValue: whatever value is stored is exactly what is
//      returned (identity / referential-equality property).
//   2. NilFieldsReturnNil: when all three manager fields are nil, all three
//      getters return nil (single-tenant safety property).
//
// Run with:
//
//	go test -run TestProperty -v -count=1 \
//	    ./components/transaction/internal/bootstrap/
//
// =============================================================================

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

// TestProperty_Service_GettersReturnExactValue verifies the identity property:
// for any non-nil interface{} value stored in pgManager, mongoManager, or
// multiTenantConsumerPort, the corresponding getter returns that exact value.
//
// This guards against accidental wrapping, copying, or transformation of the
// opaque handle, which would break identity checks performed by callers that
// use the returned handle as a map key or compare it against a known instance.
func TestProperty_Service_GettersReturnExactValue(t *testing.T) {
	t.Parallel()

	// The property uses a string as the concrete value that quick.Check
	// generates. Strings are immutable and comparable by value in Go, so
	// assert.Equal / == works correctly for the identity check. Using a
	// primitive type also avoids any pointer aliasing ambiguity.
	property := func(pgVal, mgoVal, consumerVal string) bool {
		// Bound generated strings to prevent memory pressure.
		const maxLen = 256
		if len(pgVal) > maxLen {
			pgVal = pgVal[:maxLen]
		}

		if len(mgoVal) > maxLen {
			mgoVal = mgoVal[:maxLen]
		}

		if len(consumerVal) > maxLen {
			consumerVal = consumerVal[:maxLen]
		}

		// Store concrete string values as interface{}.
		svc := &Service{
			pgManager:               pgVal,
			mongoManager:            mgoVal,
			multiTenantConsumerPort: consumerVal,
		}

		// Identity property: getter must return the exact stored value.
		pgGot := svc.GetPGManager()
		if pgGot != pgVal {
			return false
		}

		mgoGot := svc.GetMongoManager()
		if mgoGot != mgoVal {
			return false
		}

		consumerGot := svc.GetMultiTenantConsumer()

		return consumerGot == consumerVal
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err,
		"Property violated: a getter did not return the exact value that was stored")
}

// TestProperty_Service_NilFieldsReturnNil verifies the nil-safety invariant:
// when all three manager fields are nil (single-tenant deployment), all three
// getters must return nil for any combination of other Service fields.
//
// quick.Check generates random bool values to vary an unrelated field
// (BalanceSyncWorkerEnabled) on each iteration, proving the nil-return
// property holds regardless of other Service state.
func TestProperty_Service_NilFieldsReturnNil(t *testing.T) {
	t.Parallel()

	property := func(balanceSyncEnabled bool) bool {
		// All three manager fields are nil — single-tenant mode.
		svc := &Service{
			pgManager:                nil,
			mongoManager:             nil,
			multiTenantConsumerPort:  nil,
			BalanceSyncWorkerEnabled: balanceSyncEnabled,
		}

		// All three getters must return nil regardless of other field values.
		return svc.GetPGManager() == nil &&
			svc.GetMongoManager() == nil &&
			svc.GetMultiTenantConsumer() == nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err,
		"Property violated: a getter returned non-nil when the manager field was nil")
}
