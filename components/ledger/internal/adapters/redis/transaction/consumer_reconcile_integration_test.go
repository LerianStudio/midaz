//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// =============================================================================
// POST-TIMEOUT RECONCILIATION INTEGRATION TESTS
// =============================================================================
// These tests cover the Go side of the idempotency contract: after a response
// the client never received, the adapter asks the apply marker whether the
// script actually applied, and answers the caller with the stored response when
// it did.
//
// The chaos suite drives the same helper through a real lost response; here the
// marker states are set up directly so each branch is deterministic — hit, miss,
// unreadable payload, and a caller context that is already dead.
//
// Scope is the adapter layer only: a real Valkey from testcontainers, no
// application environment.

// reconcileTestSpan is the span the helper decorates. A span pulled from a plain
// context is the non-recording one, which is what the unit of work under test
// needs: a real receiver for SetAttributes with nothing to assert on.
func reconcileTestSpan() trace.Span {
	return trace.SpanFromContext(context.Background())
}

// reconcileTestClient returns the raw client the helper is handed in production.
func reconcileTestClient(t *testing.T, infra *integrationTestInfra) redis.UniversalClient {
	t.Helper()

	rds, err := infra.repo.conn.GetClient(context.Background())
	require.NoError(t, err)

	return rds
}

// applyMarkerAppliedFixture runs one real, successful execution so the script
// writes its own marker, and returns everything the reconciliation needs to
// answer for it.
func applyMarkerAppliedFixture(t *testing.T, infra *integrationTestInfra, ctx context.Context, alias string) (
	first *mmodel.BalanceAtomicResult, markerKey string, mapBalances map[string]*mmodel.Balance,
) {
	t.Helper()

	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	op := overdraftOp(orgID, ledgerID, alias, "deposit", "credit",
		decimal.NewFromInt(900), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(400))

	first, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
	require.NoError(t, err)
	require.Len(t, first.After, 1)

	return first,
		applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.APPROVED),
		map[string]*mmodel.Balance{op.Alias: op.Balance}
}

// TestIntegration_Reconcile_MarkerHitReturnsStoredResult is the conversion the
// epic exists for: the marker proves the application, so the helper hands back
// the response the caller never received instead of letting a 500 roll back a
// posting that happened.
func TestIntegration_Reconcile_MarkerHitReturnsStoredResult(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	first, markerKey, mapBalances := applyMarkerAppliedFixture(t, infra, ctx, "@reconcile-hit")

	result, ok := infra.repo.reconcileFromApplyMarker(
		ctx, reconcileTestSpan(), reconcileTestClient(t, infra), markerKey, uuid.New().String(), mapBalances,
	)

	require.True(t, ok, "a marker that proves the application must reconcile")
	require.NotNil(t, result)
	require.Len(t, result.Before, 1)
	require.Len(t, result.After, 1)

	assert.Equal(t, first.Before[0].Available.String(), result.Before[0].Available.String())
	assert.Equal(t, first.After[0].Available.String(), result.After[0].Available.String())
	assert.Equal(t, first.After[0].OnHold.String(), result.After[0].OnHold.String())
	assert.Equal(t, first.After[0].Version, result.After[0].Version)
}

// TestIntegration_Reconcile_MarkerMissKeepsFailure covers the other half of the
// contract: with no marker there is no proof, so the helper refuses to convert
// and the caller's original error is what survives.
func TestIntegration_Reconcile_MarkerMissKeepsFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	op := overdraftOp(orgID, ledgerID, "@reconcile-miss", "deposit", "credit",
		decimal.NewFromInt(900), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(400))

	// The identity was never executed, so nothing wrote its marker.
	markerKey := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.APPROVED)

	result, ok := infra.repo.reconcileFromApplyMarker(
		ctx, reconcileTestSpan(), reconcileTestClient(t, infra), markerKey, txID.String(),
		map[string]*mmodel.Balance{op.Alias: op.Balance},
	)

	assert.False(t, ok, "an absent marker must not be read as an application")
	assert.Nil(t, result)
}

// TestIntegration_Reconcile_UnreadableMarkerKeepsFailure plants a payload the
// decoder cannot use. The helper must fail closed and stay silent about it: a
// decode error of its own must never replace the caller's original error.
func TestIntegration_Reconcile_UnreadableMarkerKeepsFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	op := overdraftOp(orgID, ledgerID, "@reconcile-corrupt", "deposit", "credit",
		decimal.NewFromInt(900), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(400))

	markerKey := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.APPROVED)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, markerKey, `{"before":`, time.Hour).Err())

	assert.NotPanics(t, func() {
		result, ok := infra.repo.reconcileFromApplyMarker(
			ctx, reconcileTestSpan(), reconcileTestClient(t, infra), markerKey, txID.String(),
			map[string]*mmodel.Balance{op.Alias: op.Balance},
		)

		assert.False(t, ok, "an undecodable marker proves nothing")
		assert.Nil(t, result)
	})
}

// TestIntegration_Reconcile_MarkerHitMissingAliasKeepsFailure covers Fix 1's
// strict decode path for a marker-derived response: a repeated pending-transition
// can rebuild mapBalances from a companion set that no longer agrees with the
// plan that wrote the marker. Even though the marker DOES prove the application,
// an alias it names but the current mapBalances cannot resolve must not convert
// into a truncated success — the caller's original lost-response error has to
// survive so a retry never persists a transaction missing an Operation record
// for a balance mutation that already happened.
func TestIntegration_Reconcile_MarkerHitMissingAliasKeepsFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	_, markerKey, _ := applyMarkerAppliedFixture(t, infra, ctx, "@reconcile-missing-alias")

	// The retry's mapBalances is rebuilt WITHOUT the plan's alias, unlike the
	// fixture's own map — the shape a diverging enrichOverdraftOperations
	// companion set produces on a repeated pending-transition.
	result, ok := infra.repo.reconcileFromApplyMarker(
		ctx, reconcileTestSpan(), reconcileTestClient(t, infra), markerKey, uuid.New().String(),
		map[string]*mmodel.Balance{},
	)

	assert.False(t, ok, "a marker response naming an alias absent from mapBalances must not convert")
	assert.Nil(t, result)
}

// TestIntegration_Reconcile_SurvivesDeadCallerContext is the reason the lookup
// runs on a detached context. The expired or cancelled caller deadline is
// typically the very thing that produced the lost response, so a reconciliation
// inheriting it could never run.
func TestIntegration_Reconcile_SurvivesDeadCallerContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	first, markerKey, mapBalances := applyMarkerAppliedFixture(t, infra, ctx, "@reconcile-deadctx")
	rds := reconcileTestClient(t, infra)

	deadlineCtx, cancelDeadline := context.WithDeadline(ctx, time.Unix(0, 0))
	defer cancelDeadline()

	canceledCtx, cancelNow := context.WithCancel(ctx)
	cancelNow()

	for name, deadCtx := range map[string]context.Context{
		"expired deadline": deadlineCtx,
		"cancelled caller": canceledCtx,
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, deadCtx.Err(), "the fixture context must already be dead")

			result, ok := infra.repo.reconcileFromApplyMarker(
				ctx, reconcileTestSpan(), rds, markerKey, uuid.New().String(), mapBalances,
			)
			require.True(t, ok)

			resultFromDeadCtx, okFromDeadCtx := infra.repo.reconcileFromApplyMarker(
				deadCtx, reconcileTestSpan(), rds, markerKey, uuid.New().String(), mapBalances,
			)

			require.True(t, okFromDeadCtx, "the detached lookup must outlive the caller's context")
			require.NotNil(t, resultFromDeadCtx)
			require.Len(t, resultFromDeadCtx.After, 1)
			assert.Equal(t, first.After[0].Available.String(), resultFromDeadCtx.After[0].Available.String())
			assert.Equal(t, result.After[0].Version, resultFromDeadCtx.After[0].Version)
		})
	}
}
