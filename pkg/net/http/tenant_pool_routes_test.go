// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	poolTestTenantID         = "tenant-a"
	poolTestModuleOnboarding = "onboarding"
	poolTestModuleTx         = "transaction"
)

// sentinelDB is an identity-only dbresolver.DB: tests compare handles and never
// call a method on them (the embedded nil interface would fail loudly if one did).
type sentinelDB struct {
	dbresolver.DB
	label string
}

func newSentinelDB(label string) *sentinelDB { return &sentinelDB{label: label} }

// constructorCall records one invocation of a TenantPoolConstructor.
type constructorCall struct {
	tenantID string
	pinned   dbresolver.DB
}

// recordingConstructor returns a constructor that hands back result and
// records every call it receives.
type recordingConstructor struct {
	mu     sync.Mutex
	calls  []constructorCall
	result dbresolver.DB
}

func (r *recordingConstructor) build(_ context.Context, tenantID string, pinned dbresolver.DB) dbresolver.DB {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, constructorCall{tenantID: tenantID, pinned: pinned})

	return r.result
}

func (r *recordingConstructor) recorded() []constructorCall {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]constructorCall(nil), r.calls...)
}

// poolSeed is the request context lib-commons' WithTenantDB would leave behind.
type poolSeed struct {
	tenantID string
	generic  dbresolver.DB
	modules  map[string]dbresolver.DB
}

// runPoolResolution mounts a seeding middleware standing in for
// [authAssertion, WithTenantDB], then the handler under test, and returns the
// context observed once the chain has returned.
func runPoolResolution(t *testing.T, seed poolSeed, handler fiber.Handler) context.Context {
	t.Helper()

	var observed context.Context

	app := fiber.New()

	// Same capture shape as protected_routes_test.go: read c.Context() after
	// the whole chain has returned.
	app.Use(func(c fiber.Ctx) error {
		err := c.Next()
		observed = c.Context()

		return err
	})
	app.Use(func(c fiber.Ctx) error {
		ctx := c.Context()

		if seed.tenantID != "" {
			ctx = tmcore.ContextWithTenantID(ctx, seed.tenantID)
		}

		if seed.generic != nil {
			ctx = tmcore.ContextWithPG(ctx, seed.generic)
		}

		for module, db := range seed.modules {
			ctx = tmcore.ContextWithPG(ctx, db, module)
		}

		c.SetContext(ctx)

		return c.Next()
	})
	app.Use(handler)
	app.Get("/test", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/test", nil))
	require.NoError(t, err)
	require.Equal(t, fiber.StatusNoContent, resp.StatusCode)
	require.NotNil(t, observed, "the chain must have run")

	return observed
}

func TestWithTenantPoolResolution_ReplacesPresentModulesWithConstructedHandle(t *testing.T) {
	t.Parallel()

	pinnedOnboarding := newSentinelDB("pinned-onboarding")
	pinnedTx := newSentinelDB("pinned-transaction")
	lazyOnboarding := &recordingConstructor{result: newSentinelDB("lazy-onboarding")}
	lazyTx := &recordingConstructor{result: newSentinelDB("lazy-transaction")}

	ctx := runPoolResolution(t,
		poolSeed{
			tenantID: poolTestTenantID,
			modules: map[string]dbresolver.DB{
				poolTestModuleOnboarding: pinnedOnboarding,
				poolTestModuleTx:         pinnedTx,
			},
		},
		WithTenantPoolResolution(
			TenantPoolModule{Name: poolTestModuleOnboarding, New: lazyOnboarding.build},
			TenantPoolModule{Name: poolTestModuleTx, New: lazyTx.build},
		),
	)

	assert.Same(t, lazyOnboarding.result, tmcore.GetPGContext(ctx, poolTestModuleOnboarding))
	assert.Same(t, lazyTx.result, tmcore.GetPGContext(ctx, poolTestModuleTx))
	assert.Equal(t, []constructorCall{{tenantID: poolTestTenantID, pinned: pinnedOnboarding}}, lazyOnboarding.recorded())
	assert.Equal(t, []constructorCall{{tenantID: poolTestTenantID, pinned: pinnedTx}}, lazyTx.recorded())
	assert.Nil(t, tmcore.GetPGContext(ctx), "the generic key must not be created")
}

func TestWithTenantPoolResolution_DuplicateModuleNameKeepsFirst(t *testing.T) {
	t.Parallel()

	pinnedOnboarding := newSentinelDB("pinned-onboarding")
	first := &recordingConstructor{result: newSentinelDB("lazy-first")}
	second := &recordingConstructor{result: newSentinelDB("lazy-second")}

	ctx := runPoolResolution(t,
		poolSeed{
			tenantID: poolTestTenantID,
			modules:  map[string]dbresolver.DB{poolTestModuleOnboarding: pinnedOnboarding},
		},
		WithTenantPoolResolution(
			TenantPoolModule{Name: poolTestModuleOnboarding, New: first.build},
			TenantPoolModule{Name: poolTestModuleOnboarding, New: second.build},
		),
	)

	assert.Equal(t, []constructorCall{{tenantID: poolTestTenantID, pinned: pinnedOnboarding}}, first.recorded(),
		"the first constructor runs once, with the pool WithTenantDB stored")
	assert.Empty(t, second.recorded(), "a repeated module name is ignored")
	assert.Same(t, first.result, tmcore.GetPGContext(ctx, poolTestModuleOnboarding))
}

func TestWithTenantPoolResolution_AbsentModuleKeyIsNotCreated(t *testing.T) {
	t.Parallel()

	lazyOnboarding := &recordingConstructor{result: newSentinelDB("lazy-onboarding")}
	lazyTx := &recordingConstructor{result: newSentinelDB("lazy-transaction")}

	ctx := runPoolResolution(t,
		poolSeed{
			tenantID: poolTestTenantID,
			modules:  map[string]dbresolver.DB{poolTestModuleOnboarding: newSentinelDB("pinned-onboarding")},
		},
		WithTenantPoolResolution(
			TenantPoolModule{Name: poolTestModuleOnboarding, New: lazyOnboarding.build},
			TenantPoolModule{Name: poolTestModuleTx, New: lazyTx.build},
		),
	)

	assert.Same(t, lazyOnboarding.result, tmcore.GetPGContext(ctx, poolTestModuleOnboarding))
	assert.Nil(t, tmcore.GetPGContext(ctx, poolTestModuleTx), "absent module key must stay absent")
	assert.Empty(t, lazyTx.recorded(), "no constructor call for an absent module")
	assert.Nil(t, tmcore.GetPGContext(ctx), "the generic key must not be created")
}

func TestWithTenantPoolResolution_LeavesGenericKeyUntouched(t *testing.T) {
	t.Parallel()

	generic := newSentinelDB("generic")
	lazyOnboarding := &recordingConstructor{result: newSentinelDB("lazy-onboarding")}

	ctx := runPoolResolution(t,
		poolSeed{
			tenantID: poolTestTenantID,
			generic:  generic,
			modules:  map[string]dbresolver.DB{poolTestModuleOnboarding: newSentinelDB("pinned-onboarding")},
		},
		WithTenantPoolResolution(TenantPoolModule{Name: poolTestModuleOnboarding, New: lazyOnboarding.build}),
	)

	assert.Same(t, lazyOnboarding.result, tmcore.GetPGContext(ctx, poolTestModuleOnboarding))
	assert.Same(t, generic, tmcore.GetPGContext(ctx), "the generic key must keep the value WithTenantDB stored")
}

func TestWithTenantPoolResolution_LeavesModulesOutsideTheChainUntouched(t *testing.T) {
	t.Parallel()

	pinnedTx := newSentinelDB("pinned-transaction")
	lazyOnboarding := &recordingConstructor{result: newSentinelDB("lazy-onboarding")}

	ctx := runPoolResolution(t,
		poolSeed{
			tenantID: poolTestTenantID,
			modules: map[string]dbresolver.DB{
				poolTestModuleOnboarding: newSentinelDB("pinned-onboarding"),
				poolTestModuleTx:         pinnedTx,
			},
		},
		WithTenantPoolResolution(TenantPoolModule{Name: poolTestModuleOnboarding, New: lazyOnboarding.build}),
	)

	assert.Same(t, lazyOnboarding.result, tmcore.GetPGContext(ctx, poolTestModuleOnboarding))
	assert.Same(t, pinnedTx, tmcore.GetPGContext(ctx, poolTestModuleTx))
}

func TestWithTenantPoolResolution_NoTenantLeavesContextUntouched(t *testing.T) {
	t.Parallel()

	pinnedOnboarding := newSentinelDB("pinned-onboarding")
	lazyOnboarding := &recordingConstructor{result: newSentinelDB("lazy-onboarding")}

	ctx := runPoolResolution(t,
		poolSeed{modules: map[string]dbresolver.DB{poolTestModuleOnboarding: pinnedOnboarding}},
		WithTenantPoolResolution(TenantPoolModule{Name: poolTestModuleOnboarding, New: lazyOnboarding.build}),
	)

	assert.Same(t, pinnedOnboarding, tmcore.GetPGContext(ctx, poolTestModuleOnboarding))
	assert.Nil(t, tmcore.GetPGContext(ctx, poolTestModuleTx))
	assert.Nil(t, tmcore.GetPGContext(ctx))
	assert.Empty(t, lazyOnboarding.recorded())
}

func TestWithTenantPoolResolution_ConstructorReturningNilKeepsPinned(t *testing.T) {
	t.Parallel()

	pinnedOnboarding := newSentinelDB("pinned-onboarding")
	lazyOnboarding := &recordingConstructor{}

	ctx := runPoolResolution(t,
		poolSeed{
			tenantID: poolTestTenantID,
			modules:  map[string]dbresolver.DB{poolTestModuleOnboarding: pinnedOnboarding},
		},
		WithTenantPoolResolution(TenantPoolModule{Name: poolTestModuleOnboarding, New: lazyOnboarding.build}),
	)

	assert.Same(t, pinnedOnboarding, tmcore.GetPGContext(ctx, poolTestModuleOnboarding))
	assert.Len(t, lazyOnboarding.recorded(), 1)
}

func TestWithTenantPoolResolution_PassThroughWithoutUsableModules(t *testing.T) {
	t.Parallel()

	generic := newSentinelDB("generic")

	tests := []struct {
		name    string
		modules []TenantPoolModule
	}{
		{name: "nil modules", modules: nil},
		{name: "empty modules", modules: []TenantPoolModule{}},
		{name: "empty module name never addresses the generic key", modules: []TenantPoolModule{{
			Name: "",
			New: func(context.Context, string, dbresolver.DB) dbresolver.DB {
				return newSentinelDB("must-not-be-stored")
			},
		}}},
		{name: "nil constructor", modules: []TenantPoolModule{{Name: poolTestModuleOnboarding}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pinnedOnboarding := newSentinelDB("pinned-onboarding")

			ctx := runPoolResolution(t,
				poolSeed{
					tenantID: poolTestTenantID,
					generic:  generic,
					modules:  map[string]dbresolver.DB{poolTestModuleOnboarding: pinnedOnboarding},
				},
				WithTenantPoolResolution(tt.modules...),
			)

			assert.Same(t, pinnedOnboarding, tmcore.GetPGContext(ctx, poolTestModuleOnboarding))
			assert.Same(t, generic, tmcore.GetPGContext(ctx))
			assert.Nil(t, tmcore.GetPGContext(ctx, poolTestModuleTx))
		})
	}
}
