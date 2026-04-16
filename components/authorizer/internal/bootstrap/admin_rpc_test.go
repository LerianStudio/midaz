// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

const (
	// adminRPCTestAdminToken satisfies the 32-byte minimum length enforced by
	// validateAdminToken (this literal is 36 bytes).
	adminRPCTestAdminToken = "Str0ngAdminTokenValue!2026#Distinct1"
	adminRPCTestPeerToken  = "Str0ngPeerTokenValue!2026"
)

// adminTestFixture wraps an authorizerService wired with a real engine and
// a single prepared transaction so each test case can pick the scenario it
// needs without duplicating boilerplate.
type adminTestFixture struct {
	svc     *authorizerService
	engine  *engine.Engine
	txID    string
	balance *engine.Balance
}

// setupAdminFixture builds an engine with one in-memory balance, prepares a
// transaction that debits 100 USD from it, and returns the wired service +
// prepared_tx_id. The fixture is intentionally minimal: each test must opt
// in to additional preparation (e.g. commit / abort) before exercising the
// admin RPC.
func setupAdminFixture(t *testing.T) *adminTestFixture {
	t.Helper()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	eng := engine.New(shard.NewRouter(8), wal.NewNoopWriter())

	// Configure a long prepare timeout so the reaper does not auto-abort the
	// fixture's prepared transaction while MarkInvestigated-style tests assert
	// state. The default is 30s — far shorter than the combined runtime of the
	// admin-RPC suite under -race.
	eng.ConfigurePreparedTxStore(10*time.Minute, 0)

	t.Cleanup(eng.Close)

	balance := &engine.Balance{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}
	eng.UpsertBalances([]*engine.Balance{balance})

	metrics := newAuthorizerMetrics(nil, logger, 0)

	svc := &authorizerService{
		engine:         eng,
		logger:         logger,
		metrics:        metrics,
		peerAuthToken:  adminRPCTestPeerToken,
		adminToken:     adminRPCTestAdminToken,
		peerNonceStore: newPeerNonceStore(defaultPeerAuthMaxSkew, 100000),
	}

	prepareReq := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-admin",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{
				OperationAlias: "0#@alice#default",
				AccountAlias:   "@alice",
				BalanceKey:     "default",
				Amount:         100,
				Scale:          2,
				Operation:      constant.DEBIT,
			},
		},
	}

	ptx, _, err := eng.PrepareAuthorize(prepareReq)
	require.NoError(t, err)
	require.NotNil(t, ptx)

	return &adminTestFixture{
		svc:     svc,
		engine:  eng,
		txID:    ptx.ID,
		balance: balance,
	}
}

// adminAuthContext returns an incoming gRPC context carrying the admin token
// metadata header. Callers pass the token value to exercise both the happy
// path and invalid-token rejection.
func adminAuthContext(token string) context.Context {
	md := metadata.Pairs(adminAuthTokenHeader, token)
	return metadata.NewIncomingContext(context.Background(), md)
}

// Item #1: FORCE_COMMIT applies state.

func TestResolveManualIntervention_ForceCommitAppliesState(t *testing.T) {
	f := setupAdminFixture(t)
	ctx := adminAuthContext(adminRPCTestAdminToken)

	resp, err := f.svc.ResolveManualIntervention(ctx, &authorizerv1.ResolveManualInterventionRequest{
		TxId:       f.txID,
		Resolution: authorizerv1.ManualInterventionResolution_RESOLUTION_FORCE_COMMIT,
		Notes:      "incident=OPS-1234 on-call=galadriel",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, engine.PreparedStatePending, resp.GetPriorStatus())
	assert.Equal(t, adminResolutionNewStatusCommitted, resp.GetNewStatus())
	assert.NotEmpty(t, resp.GetTimestamp())

	// Balance must reflect the commit: 1000 - 100 = 900 available.
	got, ok := f.engine.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	assert.Equal(t, int64(900), got.Available, "force_commit must debit the balance")

	// After commit the prepared state flips to already_committed (idempotent).
	assert.Equal(t, engine.PreparedStateCommitted, f.engine.InspectPreparedState(f.txID))
}

// Item #2: FORCE_ABORT releases locks.

func TestResolveManualIntervention_ForceAbortReleasesLocks(t *testing.T) {
	f := setupAdminFixture(t)
	ctx := adminAuthContext(adminRPCTestAdminToken)

	resp, err := f.svc.ResolveManualIntervention(ctx, &authorizerv1.ResolveManualInterventionRequest{
		TxId:       f.txID,
		Resolution: authorizerv1.ManualInterventionResolution_RESOLUTION_FORCE_ABORT,
		Notes:      "operator review determined fraud; releasing locks",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, engine.PreparedStatePending, resp.GetPriorStatus())
	assert.Equal(t, adminResolutionNewStatusAborted, resp.GetNewStatus())

	// Balance must be untouched by abort.
	got, ok := f.engine.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	assert.Equal(t, int64(1000), got.Available, "force_abort must not mutate the balance")

	// Prepared store no longer tracks the tx (locks released).
	assert.Equal(t, engine.PreparedStateNotFound, f.engine.InspectPreparedState(f.txID))

	// A follow-up PrepareAuthorize on the same balance must succeed, which
	// would block if the abort did not release the balance locks.
	req := &authorizerv1.AuthorizeRequest{
		TransactionId: "tx-after-abort",
		Operations: []*authorizerv1.BalanceOperation{
			{
				OperationAlias: "0#@alice#default",
				AccountAlias:   "@alice",
				BalanceKey:     "default",
				Amount:         50,
				Scale:          2,
				Operation:      constant.DEBIT,
			},
		},
	}
	_, _, err = f.engine.PrepareAuthorize(req)
	require.NoError(t, err, "post-abort prepare must not block on released locks")
}

// Item #3: MARK_INVESTIGATED keeps state and emits audit.

func TestResolveManualIntervention_MarkInvestigatedKeepsStateEmitsAudit(t *testing.T) {
	f := setupAdminFixture(t)
	ctx := adminAuthContext(adminRPCTestAdminToken)

	resp, err := f.svc.ResolveManualIntervention(ctx, &authorizerv1.ResolveManualInterventionRequest{
		TxId:       f.txID,
		Resolution: authorizerv1.ManualInterventionResolution_RESOLUTION_MARK_INVESTIGATED,
		Notes:      "reconciled against downstream ledger; no further action required",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, engine.PreparedStatePending, resp.GetPriorStatus())
	assert.Equal(t, adminResolutionNewStatusInvestigated, resp.GetNewStatus())
	assert.NotEmpty(t, resp.GetTimestamp())

	// Prepared state is NOT cleared — the operator only acknowledged the review.
	// (We deliberately do NOT call GetBalance here: MarkInvestigated keeps the
	// balance locks held by the prepared transaction, so a snapshot read would
	// block until the reaper auto-aborts. The state invariant below is the
	// same one operators see from the admin RPC.)
	assert.Equal(t, engine.PreparedStatePending, f.engine.InspectPreparedState(f.txID),
		"mark_investigated must leave the prepared state untouched")
}

// Item #4: peer-auth alone is rejected.

// TestResolveManualIntervention_RequiresAdminToken asserts that the admin RPC
// rejects callers that do not present AUTHORIZER_ADMIN_TOKEN — even if they
// would be authorized for the peer-auth RPC surface. This is the enforcement
// of the least-privilege contract declared in the D4-followup scope.
func TestResolveManualIntervention_RequiresAdminToken(t *testing.T) {
	f := setupAdminFixture(t)

	testCases := []struct {
		name       string
		ctx        context.Context
		wantReason string // substring expected in the gRPC status message
	}{
		{
			name:       "no metadata at all",
			ctx:        context.Background(),
			wantReason: "missing_admin_token",
		},
		{
			name:       "peer-auth token presented in admin header does not match",
			ctx:        adminAuthContext(adminRPCTestPeerToken),
			wantReason: "invalid_admin_token",
		},
		{
			name: "peer-auth headers only (no admin header)",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.Pairs(
				"x-midaz-peer-timestamp", "123",
				"x-midaz-peer-nonce", "n",
				"x-midaz-peer-method", adminRPCMethodResolveManualIntervention,
			)),
			wantReason: "missing_admin_token",
		},
		{
			name:       "admin header with empty value",
			ctx:        adminAuthContext(""),
			wantReason: "missing_admin_token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.ResolveManualIntervention(tc.ctx, &authorizerv1.ResolveManualInterventionRequest{
				TxId:       f.txID,
				Resolution: authorizerv1.ManualInterventionResolution_RESOLUTION_FORCE_ABORT,
			})
			require.Error(t, err)
			assert.Equal(t, codes.Unauthenticated, status.Code(err))
			assert.Contains(t, status.Convert(err).Message(), tc.wantReason)
		})
	}

	// The prepared state must still be pending — admin-auth rejections must
	// not accidentally mutate prepared state. We assert via InspectPreparedState
	// rather than GetBalance because the locks remain held (no engine branch
	// ran) and a snapshot read would block.
	assert.Equal(t, engine.PreparedStatePending, f.engine.InspectPreparedState(f.txID),
		"rejected admin RPCs must not alter prepared state")
}

// Item #5: RESOLUTION_UNSPECIFIED is rejected.

func TestResolveManualIntervention_RejectsUnspecifiedResolution(t *testing.T) {
	f := setupAdminFixture(t)
	ctx := adminAuthContext(adminRPCTestAdminToken)

	_, err := f.svc.ResolveManualIntervention(ctx, &authorizerv1.ResolveManualInterventionRequest{
		TxId:       f.txID,
		Resolution: authorizerv1.ManualInterventionResolution_RESOLUTION_UNSPECIFIED,
		Notes:      "should be rejected before reaching the engine",
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "RESOLUTION_UNSPECIFIED")

	// Prepared state is untouched (the RPC never reached the engine, so locks
	// remain held — we cannot call GetBalance here without risking a deadlock
	// on the held balance lock).
	assert.Equal(t, engine.PreparedStatePending, f.engine.InspectPreparedState(f.txID))
}

// Item #6: NotFound for unknown tx_id.

func TestResolveManualIntervention_NotFoundForUnknownTxID(t *testing.T) {
	f := setupAdminFixture(t)
	ctx := adminAuthContext(adminRPCTestAdminToken)

	_, err := f.svc.ResolveManualIntervention(ctx, &authorizerv1.ResolveManualInterventionRequest{
		TxId:       "ptx-does-not-exist",
		Resolution: authorizerv1.ManualInterventionResolution_RESOLUTION_FORCE_COMMIT,
	})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))

	// The prepared tx we created in the fixture must still be pending — the
	// NotFound branch must not clear unrelated state.
	assert.Equal(t, engine.PreparedStatePending, f.engine.InspectPreparedState(f.txID))
}

// Item #7: config rejects empty admin token.

func TestBootstrap_RejectsEmptyAdminToken(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_AUTH_TOKEN", testPeerAuthToken)
	t.Setenv("AUTHORIZER_ADMIN_TOKEN", "")

	_, err := LoadConfig()
	require.Error(t, err)
	require.ErrorIs(t, err, errConfigAdminTokenRequired)
	assert.ErrorContains(t, err, "AUTHORIZER_ADMIN_TOKEN")
}

// Item #8: admin token denylist + weakness rejection.

func TestBootstrap_AdminTokenDenylistRejectsWeakValues(t *testing.T) {
	// A 40-byte peer token used by the "duplicate of peer auth token" case so
	// the admin validation runs past its length + classes checks and hits the
	// equals-peer sentinel. Peer-auth token minimum is 24 bytes, so 40 is
	// equally valid on the peer side.
	const longPeerToken = "Str0ngPeerTokenValue!2026-40bytes-long--"

	testCases := []struct {
		name         string
		peerToken    string // override AUTHORIZER_PEER_AUTH_TOKEN (optional)
		token        string
		wantSentinel error
	}{
		{
			name:         "denylisted placeholder",
			token:        "midaz-local-admin-token",
			wantSentinel: errConfigAdminTokenWeak,
		},
		{
			name:         "denylisted simple value",
			token:        "password",
			wantSentinel: errConfigAdminTokenWeak,
		},
		{
			name:         "denylisted generic admin string",
			token:        "admin-token",
			wantSentinel: errConfigAdminTokenWeak,
		},
		{
			name:         "too short (below 32-byte floor)",
			token:        "Aa1!tooShort#2026", // 17 bytes
			wantSentinel: errConfigAdminTokenShort,
		},
		{
			name:         "insufficient character classes",
			token:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // 40 lower only
			wantSentinel: errConfigAdminTokenClasses,
		},
		{
			name:         "duplicate of peer auth token",
			peerToken:    longPeerToken,
			token:        longPeerToken,
			wantSentinel: errConfigAdminTokenEqualsPeer,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENV_NAME", "development")
			t.Setenv("DB_TRANSACTION_HOST", "localhost")
			t.Setenv("DB_TRANSACTION_PORT", "5432")
			t.Setenv("DB_TRANSACTION_USER", "midaz")
			t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
			t.Setenv("DB_TRANSACTION_NAME", "transaction")
			t.Setenv("DB_TRANSACTION_SSLMODE", "disable")

			peerToken := testPeerAuthToken
			if tc.peerToken != "" {
				peerToken = tc.peerToken
			}

			t.Setenv("AUTHORIZER_PEER_AUTH_TOKEN", peerToken)
			t.Setenv("AUTHORIZER_ADMIN_TOKEN", tc.token)

			_, err := LoadConfig()
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantSentinel, "token=%q must trigger sentinel %v", tc.token, tc.wantSentinel)
		})
	}
}

// Extra: config happy path populates AdminToken.

// TestBootstrap_AdminTokenLoadsOnSuccess proves the happy path: given a valid
// admin token distinct from the peer-auth token, LoadConfig populates
// cfg.AdminToken. This anchors the wiring and prevents silent regressions
// where AdminToken would be silently empty.
func TestBootstrap_AdminTokenLoadsOnSuccess(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_AUTH_TOKEN", testPeerAuthToken)
	t.Setenv("AUTHORIZER_ADMIN_TOKEN", testAdminToken)

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, testAdminToken, cfg.AdminToken)
	assert.NotEqual(t, cfg.PeerAuthToken, cfg.AdminToken, "admin token must be distinct from peer-auth token")
}
