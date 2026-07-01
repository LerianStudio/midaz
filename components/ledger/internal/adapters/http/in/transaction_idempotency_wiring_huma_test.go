// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	crmServices "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

// Money-write idempotency-header WIRING regression (end-to-end over app.Test).
//
// The pinning suite proved the header struct-tag names by reflection
// (TestHuma_CreateEnvelopes_CanonicalIdempotencyHeaders) and the revert TTL
// default by AST. Neither exercises the ACTUAL Huma header binder → core hop:
// that Huma reads the request's canonical "X-Idempotency" / "X-TTL" headers and
// hands the caller's key and TTL to the untouched createTransaction core. That
// hop is exactly where the fixed bug lived — a wrong `header:` tag silently binds
// nothing, the core falls back to key = payload-hash, and a keyed retry with a
// tweaked body mutates balances a second time.
//
// These tests drive a real request through the Huma terminal and capture what the
// core RECEIVES, using the core's first repo touch as the probe:
//
//   - createTransaction → Command.CreateOrCheckTransactionIdempotency, whose sole
//     fresh-key call is TransactionRedisRepo.SetNX(ctx, internalKey, "", ttl). The
//     internalKey is utils.IdempotencyInternalKey(org, ledger, <caller-key>) — the
//     caller key is a literal substring — and ttl is the resolved X-TTL. So the
//     SetNX args ARE the header→core wiring, observed without Redis.
//
// The probe returns the losing-claim replay branch (SetNX=false, Get="{}") so the
// request short-circuits to 201 + X-Idempotency-Replayed:true BEFORE touching the
// (nil) Query use case — proving the replay response header in the same pass. The
// full Redis-backed replay is covered by the integration suite
// (TestIntegration_TransactionHandler_IdempotencyReplay).
//
// THE TOOTH: if a `header:` tag regresses to "X-Idempotency-Key" / "X-Idempotency-TTL"
// (the pre-fix bug), Huma's ctx.Header("X-Idempotency") returns "", the core falls
// back to key = hash, and the captured internalKey embeds the 64-hex SHA-256 hash
// instead of stableKey — assertContainsStableKey fails, and the X-TTL default subtest
// fails because the TTL header is dropped. Not parallel: buildHumaTransactionApp
// mutates process-global huma state (see its header).

const (
	// stableKey is a caller-supplied idempotency key with a shape that could never
	// be mistaken for the 64-hex-char SHA-256 payload-hash fallback.
	stableKey    = "caller-stable-idem-key-2026"
	customTTLSec = "600"
)

// createBodyFor returns a minimal valid create body per variant. Each yields
// send.value = "100" > 0 (passing the core's non-positive guard) and reaches the
// idempotency claim; json/annotation share the full send, inflow drops source,
// outflow drops distribute — matching the three input structs' validation.
func createBodyFor(op string) string {
	const (
		source     = `"source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]}`
		distribute = `"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}`
	)

	switch op {
	case "inflow":
		return `{"send":{"asset":"USD","value":"100",` + distribute + `}}`
	case "outflow":
		return `{"send":{"asset":"USD","value":"100",` + source + `}}`
	default: // json, annotation
		return `{"send":{"asset":"USD","value":"100",` + source + `,` + distribute + `}}`
	}
}

// captureIdempotencyProbe wires a TransactionHandler whose Command is a real
// use case backed by a mocked Redis repo. SetNX captures the internalKey + ttl the
// core computed from the request headers, then returns the losing-claim path
// (false) so Get("{}") drives the replay short-circuit — the request returns 201
// before the nil Query is ever touched.
func captureIdempotencyProbe(t *testing.T, ctrl *gomock.Controller, gotKey *string, gotTTL *time.Duration) *TransactionHandler {
	t.Helper()

	redisMock := txRedis.NewMockRedisRepository(ctrl)

	redisMock.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), "", gomock.Any()).
		DoAndReturn(func(_ context.Context, key, _ string, ttl time.Duration) (bool, error) {
			*gotKey = key
			*gotTTL = ttl

			return false, nil // losing claim -> Get() -> replay branch
		}).Times(1)

	redisMock.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("{}", nil). // valid empty transaction.Transaction JSON -> replay
		Times(1)

	return &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisMock}}
}

// TestHuma_CreateTransaction_CanonicalIdempotencyHeaderReachesCore proves the
// canonical X-Idempotency / X-TTL headers bind through the Huma terminal and reach
// the createTransaction core intact, for all four money-write CREATE variants.
func TestHuma_CreateTransaction_CanonicalIdempotencyHeaderReachesCore(t *testing.T) {
	// NOT parallel: buildHumaTransactionApp mutates process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	for _, op := range createOpPaths {
		t.Run(op, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			var gotKey string

			var gotTTL time.Duration

			handler := captureIdempotencyProbe(t, ctrl, &gotKey, &gotTTL)
			app := buildHumaTransactionApp(t, handler, true)

			url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + op
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(createBodyFor(op)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(libConstants.IdempotencyKey, stableKey) // "X-Idempotency"
			req.Header.Set(libConstants.IdempotencyTTL, customTTLSec)

			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			body, _ := io.ReadAll(resp.Body)

			// The core received the caller's stable key (embedded in the internal Redis
			// key), NOT the payload-hash fallback. THIS is the regression that a wrong
			// `header:` tag reintroduces.
			assert.Contains(t, gotKey, stableKey,
				"core must receive the caller's X-Idempotency key; got internalKey=%q (a 64-hex hash here means the header tag drifted off %q)", gotKey, libConstants.IdempotencyKey)
			// ParseIdempotencyTTL returns the raw seconds count as a time.Duration
			// (the *time.Second multiply happens downstream in the Redis helper), so
			// "600" reaches the core as time.Duration(600). A wrong tag drops it to the
			// 300 default.
			assert.Equal(t, time.Duration(600), gotTTL,
				"core must receive the X-TTL header value (600); a wrong tag drops it to the 300 default")

			// Replay short-circuit: 201 + X-Idempotency-Replayed:true, no Query touch.
			assert.Equal(t, http.StatusCreated, resp.StatusCode, "replay returns 201, body: %s", string(body))
			assert.Equal(t, "true", resp.Header.Get(libConstants.IdempotencyReplayed),
				"a losing idempotency claim with a cached value replays -> X-Idempotency-Replayed:true")
		})
	}
}

// TestHuma_CreateTransaction_IdempotencyTTLDefaultsWhenAbsent proves the X-TTL
// header is read (not ignored): when absent, ParseIdempotencyTTL("") defaults to
// 300s at the core. A wrong `header:` tag would drop a PRESENT X-TTL to this same
// default, so this pins the read path's floor.
func TestHuma_CreateTransaction_IdempotencyTTLDefaultsWhenAbsent(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	var gotKey string

	var gotTTL time.Duration

	handler := captureIdempotencyProbe(t, ctrl, &gotKey, &gotTTL)
	app := buildHumaTransactionApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/json"
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(createBodyFor("json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(libConstants.IdempotencyKey, stableKey)
	// No X-TTL header on purpose.

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Contains(t, gotKey, stableKey, "core must still receive the X-Idempotency key when X-TTL is absent")
	assert.Equal(t, time.Duration(300), gotTTL, "absent X-TTL defaults to 300 at the core (ParseIdempotencyTTL(\"\"))")
}

// TestHuma_CreateHolder_CanonicalIdempotencyHeaderReachesCore is the holder analogue
// (the holder Huma shell carried the same header-name bug). The createHolder core
// builds services.HolderIdempotencyKey(org, <caller-key>) and claims it via
// Service.CreateOrCheckCRMIdempotency -> Idempotency.SetNX; the losing claim + a
// cached Get replays to 201 + X-Idempotency-Replayed:true. redis.MockRedisRepository
// structurally satisfies the narrow crmServices.IdempotencyRepo port (SetNX/Get/Set),
// so it reuses the transaction Redis mock. Same tooth: a "X-Idempotency-Key" tag
// regression drops the header and the captured key embeds the hash, not stableKey.
func TestHuma_CreateHolder_CanonicalIdempotencyHeaderReachesCore(t *testing.T) {
	// NOT parallel: buildHumaHolderApp mutates process-global huma state.
	orgID := uuid.New()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	var gotKey string

	var gotTTL time.Duration

	redisMock := txRedis.NewMockRedisRepository(ctrl)
	redisMock.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), "", gomock.Any()).
		DoAndReturn(func(_ context.Context, key, _ string, ttl time.Duration) (bool, error) {
			gotKey = key
			gotTTL = ttl

			return false, nil
		}).Times(1)
	redisMock.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("{}", nil). // valid empty mmodel.Holder JSON -> replay
		Times(1)

	handler := &HolderHandler{Service: &crmServices.UseCase{Idempotency: redisMock}}
	app := buildHumaHolderApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"type": "NATURAL_PERSON", "name": "John Doe", "document": "91315026015"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/holders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(libConstants.IdempotencyKey, stableKey)
	req.Header.Set(libConstants.IdempotencyTTL, customTTLSec)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Contains(t, gotKey, stableKey,
		"holder core must receive the caller's X-Idempotency key; got %q", gotKey)
	assert.Equal(t, time.Duration(600), gotTTL, "holder core must receive the X-TTL value (600)")
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "holder replay returns 201, body: %s", string(respBody))
	assert.Equal(t, "true", resp.Header.Get(libConstants.IdempotencyReplayed),
		"holder losing claim with a cached value replays -> X-Idempotency-Replayed:true")
}
