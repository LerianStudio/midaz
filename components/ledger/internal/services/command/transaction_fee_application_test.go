// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// fakeFeeApplier records invocations and applies a scripted mutation/error so
// the seam's contract can be asserted without the real fee engine.
type fakeFeeApplier struct {
	calls   int
	lastOrg uuid.UUID
	mutate  func(cf *model.FeeCalculate)
	err     error
}

func (f *fakeFeeApplier) CalculateFee(_ context.Context, cf *model.FeeCalculate, organizationID uuid.UUID) error {
	f.calls++
	f.lastOrg = organizationID

	if f.err != nil {
		return f.err
	}

	if f.mutate != nil {
		f.mutate(cf)
	}

	return nil
}

func baseTransaction() mtransaction.Transaction {
	return mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "BRL",
			Value: decimal.NewFromInt(1000),
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{{AccountAlias: "@payer"}},
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{{AccountAlias: "@payee"}},
			},
		},
	}
}

func TestApplyFees_NoOpOnAnnotation(t *testing.T) {
	applier := &fakeFeeApplier{mutate: func(cf *model.FeeCalculate) {
		cf.Transaction.Send.Value = decimal.NewFromInt(999) // would corrupt if ever run
	}}
	uc := &UseCase{FeeApplier: applier}

	input := baseTransaction()
	orgID, ledgerID := uuid.New(), uuid.New()

	err := uc.applyFees(context.Background(), &input, orgID, ledgerID, true /* isAnnotation */, false /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.Equal(t, 0, applier.calls, "fee engine must not run on the annotation path (NOTED is one-sided, no fee)")
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)), "annotation input must be untouched")
}

func TestApplyFees_NoOpWhenApplierNil(t *testing.T) {
	uc := &UseCase{FeeApplier: nil}

	input := baseTransaction()

	err := uc.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, false /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)))
}

func TestApplyFees_NoOpWhenSkipHonored(t *testing.T) {
	applier := &fakeFeeApplier{mutate: func(cf *model.FeeCalculate) {
		cf.Transaction.Send.Value = decimal.NewFromInt(999) // would corrupt if ever run
	}}
	uc := &UseCase{FeeApplier: applier}

	input := baseTransaction()

	err := uc.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, true /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.Equal(t, 0, applier.calls,
		"an honored fee skip must bypass the engine entirely — zero CalculateFee, hence zero FindByOrganizationIDAndLedgerID package lookup")
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)), "honored fee skip must leave the transaction unmutated")
}

func TestApplyFees_SkipHonoredTouchesNoFeeDependency(t *testing.T) {
	// nil FeeApplier AND nil FeesMongoManager: an honored skip must return before
	// reaching either, proving the bypass touches no fee dependency (no engine
	// call, no tenant Mongo resolution). A non-skip path would nil-deref the
	// applier or fall through to resolution; this returns nil cleanly.
	uc := &UseCase{FeeApplier: nil, FeesMongoManager: nil, MultiTenantEnabled: true}

	input := baseTransaction()

	err := uc.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, true /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(1000)), "honored fee skip must leave the transaction unmutated")
}

func TestApplyFees_FoldsMutatedSendBack(t *testing.T) {
	ledgerID := uuid.New()
	orgID := uuid.New()

	applier := &fakeFeeApplier{mutate: func(cf *model.FeeCalculate) {
		// Engine moves Send.Value (deductible fee) and appends a fee leg.
		require.Equal(t, ledgerID, cf.LedgerID, "applyFees must pass the ledger ID into the envelope")
		cf.Transaction.Send.Value = decimal.NewFromInt(950)
		cf.Transaction.Send.Distribute.To = append(cf.Transaction.Send.Distribute.To,
			mtransaction.FromTo{AccountAlias: "@fee_collector"})
		if cf.Transaction.Metadata == nil {
			cf.Transaction.Metadata = map[string]any{}
		}
		cf.Transaction.Metadata["packageAppliedID"] = "pkg-1"
	}}
	uc := &UseCase{FeeApplier: applier}

	input := baseTransaction()

	err := uc.applyFees(context.Background(), &input, orgID, ledgerID, false, false /* honoredFeeSkip */)

	require.NoError(t, err)
	assert.Equal(t, 1, applier.calls)
	assert.Equal(t, orgID, applier.lastOrg)
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(950)), "moved Send.Value must fold back into the caller's input")
	require.Len(t, input.Send.Distribute.To, 2, "appended fee leg must fold back into the caller's input")
	assert.Equal(t, "@fee_collector", input.Send.Distribute.To[1].AccountAlias)
	assert.Equal(t, "pkg-1", input.Metadata["packageAppliedID"])
}

func TestApplyFees_PropagatesBusinessError(t *testing.T) {
	applier := &fakeFeeApplier{err: pkg.ValidationError{
		Code:    "0199",
		Title:   "Package amount range overlap",
		Message: "transaction value is outside the package range",
	}}
	uc := &UseCase{FeeApplier: applier}

	input := baseTransaction()

	err := uc.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, false /* honoredFeeSkip */)

	require.Error(t, err)

	var businessErr pkg.ValidationError

	require.True(t, errors.As(err, &businessErr),
		"the fee engine returns a canonical pkg business error that must reach http.WithError unchanged so it surfaces as a 4xx, not a 500")
	assert.Equal(t, "0199", businessErr.Code)
	assert.Equal(t, "transaction value is outside the package range", businessErr.Message)
}

// =============================================================================
// resolveFeesTenantContext
// =============================================================================

// fakeFeesDBResolver returns a per-tenant *mongo.Database handle, modelling the
// tenant-manager Mongo manager's GetDatabaseForTenant without a live connection.
type fakeFeesDBResolver struct {
	dbs map[string]*mongo.Database
	err error
}

func (f *fakeFeesDBResolver) GetDatabaseForTenant(_ context.Context, tenantID string) (*mongo.Database, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.dbs[tenantID], nil
}

// newDisconnectedFeeDatabase builds a *mongo.Database handle without dialling.
// mongo.Connect is lazy: no network call happens until a query runs, so a named
// database handle is pure in-memory metadata — enough to prove two tenants
// resolve to DISTINCT databases through the seam.
func newDisconnectedFeeDatabase(t *testing.T, dbName string) *mongo.Database {
	t.Helper()

	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	return client.Database(dbName)
}

// TestResolveFeesTenantContext_TwoTenantsResolveDifferentDatabases is the F1
// regression guard: two different tenants MUST resolve to different fee Mongo
// databases on the GENERIC MB key, and the request ctx MUST stay untouched.
func TestResolveFeesTenantContext_TwoTenantsResolveDifferentDatabases(t *testing.T) {
	dbA := newDisconnectedFeeDatabase(t, "fees_tenant_a")
	dbB := newDisconnectedFeeDatabase(t, "fees_tenant_b")

	uc := &UseCase{
		MultiTenantEnabled: true,
		FeesMongoManager: &fakeFeesDBResolver{dbs: map[string]*mongo.Database{
			"tenant-a": dbA,
			"tenant-b": dbB,
		}},
	}

	reqCtxA := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	derivedA, err := uc.resolveFeesTenantContext(reqCtxA)
	require.NoError(t, err)

	reqCtxB := tmcore.ContextWithTenantID(context.Background(), "tenant-b")
	derivedB, err := uc.resolveFeesTenantContext(reqCtxB)
	require.NoError(t, err)

	gotA := tmcore.GetMBContext(derivedA)
	gotB := tmcore.GetMBContext(derivedB)

	require.NotNil(t, gotA)
	require.NotNil(t, gotB)
	assert.Same(t, dbA, gotA, "tenant-a must resolve to its own fee DB on the generic key")
	assert.Same(t, dbB, gotB, "tenant-b must resolve to its own fee DB on the generic key")
	assert.NotSame(t, gotA, gotB, "two tenants must NOT share a fee database")

	// Request ctx untouched: the generic MB key must NOT leak onto the caller's ctx.
	assert.Nil(t, tmcore.GetMBContext(reqCtxA), "request ctx must not carry the resolved fee DB")
	assert.Nil(t, tmcore.GetMBContext(reqCtxB), "request ctx must not carry the resolved fee DB")
}

// TestResolveFeesTenantContext_SingleTenantNoOp proves the seam is a no-op when
// multi-tenant is disabled: the static fee connection is correct there, so the
// generic key must NOT be set and the same ctx is returned.
func TestResolveFeesTenantContext_SingleTenantNoOp(t *testing.T) {
	uc := &UseCase{
		MultiTenantEnabled: false,
		FeesMongoManager: &fakeFeesDBResolver{dbs: map[string]*mongo.Database{
			"tenant-a": newDisconnectedFeeDatabase(t, "fees_tenant_a"),
		}},
	}

	reqCtx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

	derived, err := uc.resolveFeesTenantContext(reqCtx)
	require.NoError(t, err)
	assert.Equal(t, reqCtx, derived, "single-tenant mode must return the ctx unchanged")
	assert.Nil(t, tmcore.GetMBContext(derived), "single-tenant mode must not set the generic MB key")
}

// TestResolveFeesTenantContext_MissingTenantFailsCleanly proves that when MT is
// enabled but no tenant ID is on the ctx, the seam fails with a typed error
// instead of falling through to the shared single-tenant fee DB.
func TestResolveFeesTenantContext_MissingTenantFailsCleanly(t *testing.T) {
	uc := &UseCase{
		MultiTenantEnabled: true,
		FeesMongoManager:   &fakeFeesDBResolver{dbs: map[string]*mongo.Database{}},
	}

	_, err := uc.resolveFeesTenantContext(context.Background())
	require.Error(t, err, "missing tenant must fail, never fall through to the shared DB")
}

// TestResolveFeesTenantContext_ResolutionErrorMapped proves a resolver failure
// is surfaced (mapped), not swallowed into the shared DB.
func TestResolveFeesTenantContext_ResolutionErrorMapped(t *testing.T) {
	uc := &UseCase{
		MultiTenantEnabled: true,
		FeesMongoManager:   &fakeFeesDBResolver{err: errors.New("tenant manager unreachable")},
	}

	reqCtx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

	_, err := uc.resolveFeesTenantContext(reqCtx)
	require.Error(t, err)
}

func TestApplyFees_RunsEngine(t *testing.T) {
	// The counterpart to every no-op gate: with no skip and no annotation the seam
	// must still drive the engine.
	applier := &fakeFeeApplier{mutate: func(cf *model.FeeCalculate) {
		cf.Transaction.Send.Value = decimal.NewFromInt(950)
	}}
	uc := &UseCase{FeeApplier: applier}

	input := baseTransaction()

	err := uc.applyFees(context.Background(), &input, uuid.New(), uuid.New(), false, false)

	require.NoError(t, err)
	assert.Equal(t, 1, applier.calls, "the fee seam must reach the engine when no gate fires")
	assert.True(t, input.Send.Value.Equal(decimal.NewFromInt(950)))
}
