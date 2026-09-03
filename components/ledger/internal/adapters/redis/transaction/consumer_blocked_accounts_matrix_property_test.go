//go:build integration && property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"testing"
	"testing/quick"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// =============================================================================
// BLOCK GATE — SEMANTIC MATRIX AS A PROPERTY
// =============================================================================
// The table-driven tests pin named cases. This pins the RULE they are instances
// of, over randomly generated points of the matrix:
//
//	deny  <=>  the account is in the index
//	           AND the operation carries no exception grant
//	           AND the transaction is not a cancel
//
// Anything else proceeds. Stating it as a biconditional is deliberate: a test
// that only checks "blocked and ungranted denies" passes just as happily on a
// gate that denies everything.

// blockGateCase is one point of the matrix. Its three axes are plain bools, so
// testing/quick draws them natively and every one of the eight combinations is
// reachable without a custom Generator.
type blockGateCase struct {
	Blocked bool
	Granted bool
	Cancel  bool
}

// wantDenial is the matrix, written once as the specification the gate is
// checked against.
func (c blockGateCase) wantDenial() bool {
	return c.Blocked && !c.Granted && !c.Cancel
}

// TestIntegration_BlockGateMatrix_DenialIsExactlyBlockedUngrantedNonCancel runs
// each drawn point through the real script against a real Redis.
func TestIntegration_BlockGateMatrix_DenialIsExactlyBlockedUngrantedNonCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	property := func(blocked, granted, cancel bool) bool {
		c := blockGateCase{Blocked: blocked, Granted: granted, Cancel: cancel}

		orgID, ledgerID := uuid.New(), uuid.New()
		accountID := uuid.New()

		blockedMembers := []uuid.UUID(nil)
		if c.Blocked {
			blockedMembers = []uuid.UUID{accountID}
		}

		if err := infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, blockedMembers); err != nil {
			t.Errorf("hydration failed: %v", err)

			return false
		}

		// A cancel is modelled as what it actually is: a pending RELEASE that
		// returns a hold to the account it came from.
		status, pending := constant.APPROVED, false
		operation, onHold := constant.DEBIT, decimal.Zero

		if c.Cancel {
			status, pending = constant.CANCELED, true
			operation, onHold = constant.RELEASE, decimal.NewFromInt(100)
		}

		ops := []mmodel.BalanceOperation{
			gateOp(orgID, ledgerID, accountID, "@matrix-"+accountID.String()[:8], c.Granted,
				decimal.NewFromInt(500), onHold, operation, decimal.NewFromInt(100)),
		}

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), status, pending, ops)

		var blockedErr AccountBlockedError
		gotDenial := errors.As(err, &blockedErr)

		if !gotDenial && err != nil {
			t.Errorf("case %+v produced an unexpected failure: %v", c, err)

			return false
		}

		if gotDenial != c.wantDenial() {
			t.Errorf("case %+v: got denial=%v, want %v", c, gotDenial, c.wantDenial())

			return false
		}

		// A denial must also have moved nothing, on every point of the matrix.
		if gotDenial {
			exists, existsErr := infra.redisContainer.Client.Exists(ctx, ops[0].InternalKey).Result()
			if existsErr != nil {
				t.Errorf("failed to probe the balance key: %v", existsErr)

				return false
			}

			if exists != 0 {
				t.Errorf("case %+v: a denied batch touched the balance", c)

				return false
			}
		}

		return true
	}

	require.NoError(t, quick.Check(property, &quick.Config{MaxCount: 48}))
}

// TestBlockGateGrantFlag_IsExactlyEitherCarrier is the unit-level half: the
// gate can only be as correct as the flag Go hands it, and that flag is the
// disjunction of the two grant carriers that coexist while the exception
// aggregate is being reshaped.
func TestBlockGateGrantFlag_IsExactlyEitherCarrier(t *testing.T) {
	property := func(hasExceptionID, bypassGranted bool) bool {
		grantedExceptionID := ""
		if hasExceptionID {
			grantedExceptionID = uuid.NewString()
		}

		repo := &RedisConsumerRepository{conn: newFailOnCallConnection(t)}

		plan, err := repo.buildBalanceAtomicOperationPlan(
			t.Context(), constant.APPROVED, false,
			[]mmodel.BalanceOperation{blockGateOp(t, grantedExceptionID, bypassGranted)},
		)
		if err != nil {
			t.Errorf("plan build failed: %v", err)

			return false
		}

		want := 0
		if hasExceptionID || bypassGranted {
			want = 1
		}

		return assert.ObjectsAreEqual(want, plan.args[luaArgsPerOperation-1])
	}

	require.NoError(t, quick.Check(property, &quick.Config{MaxCount: 64}))
}
