// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// This file is the v2 transaction LIFECYCLE integration + parity proof: it exercises the v2
// `commit` and `cancel` ops end-to-end against the SAME real-repo handler backed by
// testcontainers as the sibling create/hold/block file. It is split out of
// transaction_v2_handler_integration_test.go purely to keep each file under the size limit;
// both files share the same `in` package and reuse the shared request/decode/DB helpers,
// `seedTransfer`, `setupTestInfra`, and the app builders defined in the sibling files.
//
// NOT PARALLEL: the app builders call libProblem.Install() (process-global huma.NewError hook)
// and Huma validation uses process-global sync.Pools; concurrent builds cross-contaminate.
// Every test here stays sequential (see the create/hold file header for the full rationale).

// v2CommitURL / v2CancelURL build the concrete v2 lifecycle paths for a pending transaction.
// Both ops are mounted by the SAME RegisterTransactionV2RoutesToApp seam as direct/hold, so
// the v2 app built by buildHumaV2DirectApp serves them alongside the create ops. They are
// bodiless (POST with an empty body, 201 on success).
func v2CommitURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/commit"
}

func v2CancelURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/cancel"
}

// v1CancelURL builds the concrete v1 cancel path for a pending transaction. It is the cancel
// counterpart to v1CommitURL, mounted by both buildHumaTransactionApp (v1App, Huma) and the
// raw infra.app v1 chain over the SAME cancel core the v2 op reuses.
func v1CancelURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/cancel"
}

// =============================================================================
// 12. HOLD COMMIT VIA V2 (lifecycle parity, core): a v2-held transaction committed through
//     the v2 commit op settles IDENTICALLY to a v1-held transaction committed through the v1
//     commit op — the source on-hold releases, the destination credit applies, the status
//     flips to APPROVED, and the persisted operation set plus the commit response are
//     indistinguishable between the two surfaces (ignoring IDs/timestamps). Both surfaces
//     reuse the SAME commitTransaction core, so this proves the v2 lifecycle surface reaches
//     that core with the same effect the v1 surface has.
// =============================================================================

func TestIntegration_TransactionV2Hold_CommitViaV2_ParityWithV1(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// Two ledgers under the SAME org so both flows use IDENTICAL aliases and starting
	// balances; the only legitimate difference is then the ledger id (stripped) plus per-row
	// IDs/timestamps.
	ledgerV1 := infra.ledgerID
	ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

	v1Src, v1Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV1, "@src", "@dst", 1000)
	v2Src, v2Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV2, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// v1 lane: hold -> commit, each fully processed (create -> drain) BEFORE the v2 lane,
	// because the balance-sync schedule ZSET is GLOBAL, not per-ledger (see the direct parity
	// test for the same discipline).
	v1HoldResp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerV1), holdParityV1PendingBody, ""), nethttp.StatusCreated)
	v1TxID := uuid.MustParse(v1HoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 hold should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	v1CommitResp := decodeTxResponse(t, postTransaction(t, v1App, v1CommitURL(infra.orgID, ledgerV1, v1TxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 transaction should be APPROVED after v1 commit")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	// v2 lane: hold -> commit through the v2 commit op.
	v2HoldResp := decodeTxResponse(t, postTransaction(t, v2App, v2HoldURL(infra.orgID, ledgerV2), holdParityV2Body, ""), nethttp.StatusCreated)
	v2TxID := uuid.MustParse(v2HoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 hold should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	v2CommitResp := decodeTxResponse(t, postTransaction(t, v2App, v2CommitURL(infra.orgID, ledgerV2, v2TxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 transaction should be APPROVED after v2 commit")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	// Settled balances: source on-hold released (available 900, on-hold 0), destination
	// credited (available 100, on-hold 0), IDENTICAL on both surfaces.
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Src), "v1 source available after commit")
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Src), "v2 source available after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Src), "v1 source on-hold released after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Src), "v2 source on-hold released after commit")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Dst), "v1 dest credited after commit")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Dst), "v2 dest credited after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Dst), "v1 dest on-hold after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Dst), "v2 dest on-hold after commit")

	// Full lifecycle operation set: 1 ON_HOLD (hold) + 1 DEBIT (release) + 1 CREDIT (apply)
	// on each, with the economic projection IDENTICAL between the two surfaces. fetchOperationRows
	// orders by type, so the two 3-leg sets line up index-for-index.
	v1Ops := fetchOperationRows(t, infra.pgContainer.DB, v1TxID)
	v2Ops := fetchOperationRows(t, infra.pgContainer.DB, v2TxID)
	require.Len(t, v1Ops, 3, "committed v1 hold should carry 3 operations (ON_HOLD + DEBIT + CREDIT)")
	require.Len(t, v2Ops, 3, "committed v2 hold should carry 3 operations (ON_HOLD + DEBIT + CREDIT)")
	assertOperationSetsEqual(t, v1Ops, v2Ops)

	// Commit response deep-equal, ignoring IDs/timestamps: the v2 commit response (with the
	// settled operations embedded) is indistinguishable from the v1 commit response.
	require.Equal(t, stripVolatile(v1CommitResp), stripVolatile(v2CommitResp),
		"v2 commit response must be indistinguishable from the v1 commit equivalent (ignoring IDs/timestamps)")
}

// =============================================================================
// 13. HOLD CANCEL VIA V2 (lifecycle parity, core): a v2-held transaction cancelled through
//     the v2 cancel op reverts IDENTICALLY to a v1-held transaction cancelled through the v1
//     cancel op — the source on-hold is released back to available (no destination credit),
//     the status flips to CANCELED, and the persisted operations plus the cancel response are
//     indistinguishable between the two surfaces (ignoring IDs/timestamps).
// =============================================================================

func TestIntegration_TransactionV2Hold_CancelViaV2_ParityWithV1(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	ledgerV1 := infra.ledgerID
	ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

	v1Src, v1Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV1, "@src", "@dst", 1000)
	v2Src, v2Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV2, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// v1 lane: hold -> cancel, fully processed BEFORE the v2 lane (global ZSET discipline).
	v1HoldResp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerV1), holdParityV1PendingBody, ""), nethttp.StatusCreated)
	v1TxID := uuid.MustParse(v1HoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 hold should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	v1CancelResp := decodeTxResponse(t, postTransaction(t, v1App, v1CancelURL(infra.orgID, ledgerV1, v1TxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 transaction should be CANCELED after v1 cancel")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	// v2 lane: hold -> cancel through the v2 cancel op.
	v2HoldResp := decodeTxResponse(t, postTransaction(t, v2App, v2HoldURL(infra.orgID, ledgerV2), holdParityV2Body, ""), nethttp.StatusCreated)
	v2TxID := uuid.MustParse(v2HoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 hold should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	v2CancelResp := decodeTxResponse(t, postTransaction(t, v2App, v2CancelURL(infra.orgID, ledgerV2, v2TxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 transaction should be CANCELED after v2 cancel")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	// Reverted balances: the on-hold reservation is released back to available (source
	// 1000 / 0), and the destination is never credited (0 / 0), IDENTICAL on both surfaces.
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Src), "v1 source available restored after cancel")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Src), "v2 source available restored after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Src), "v1 source on-hold released after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Src), "v2 source on-hold released after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Dst), "v1 dest never credited on cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Dst), "v2 dest never credited on cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Dst), "v1 dest on-hold after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Dst), "v2 dest on-hold after cancel")

	// Cancel response deep-equal, ignoring IDs/timestamps: the v2 cancel response (with its
	// persisted operations embedded) is indistinguishable from the v1 cancel response, which
	// proves cross-surface operation-level parity without pinning the release leg's Type.
	require.Equal(t, stripVolatile(v1CancelResp), stripVolatile(v2CancelResp),
		"v2 cancel response must be indistinguishable from the v1 cancel equivalent (ignoring IDs/timestamps)")
}

// =============================================================================
// 14. CROSS-SURFACE LIFECYCLE (core reuse both ways): the hold and its lifecycle transition
//     are decoupled across surfaces because both share the SAME commitTransaction core.
//     (a) a v2-held transaction is committable through the EXISTING v1 commit endpoint, and
//     (b) a v1-held transaction is committable through the v2 commit op — both settle to
//     APPROVED with the identical ledger effect (source hold released, destination credited).
// =============================================================================

func TestIntegration_TransactionV2Hold_CrossSurfaceCommit(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// Two ledgers under the SAME org, identical seed. Ledger A carries the v2-hold -> v1-commit
	// direction; ledger B carries the v1-hold -> v2-commit direction.
	ledgerA := infra.ledgerID
	ledgerB := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerB)

	aSrc, aDst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerA, "@src", "@dst", 1000)
	bSrc, bDst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerB, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// (a) v2-held, committed through the EXISTING v1 commit endpoint (v1App). Fully processed
	// BEFORE ledger B (global ZSET discipline).
	aHoldResp := decodeTxResponse(t, postTransaction(t, v2App, v2HoldURL(infra.orgID, ledgerA), holdParityV2Body, ""), nethttp.StatusCreated)
	aTxID := uuid.MustParse(aHoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, aTxID), "v2 hold on ledger A should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerA)

	_ = decodeTxResponse(t, postTransaction(t, v1App, v1CommitURL(infra.orgID, ledgerA, aTxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, aTxID), "a v2-held transaction must commit through the v1 commit endpoint")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerA)

	// (b) v1-held (pending JSON), committed through the v2 commit op (v2App).
	bHoldResp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerB), holdParityV1PendingBody, ""), nethttp.StatusCreated)
	bTxID := uuid.MustParse(bHoldResp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, bTxID), "v1 hold on ledger B should open PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerB)

	_ = decodeTxResponse(t, postTransaction(t, v2App, v2CommitURL(infra.orgID, ledgerB, bTxID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, bTxID), "a v1-held transaction must commit through the v2 commit op")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerB)

	// Both directions settle IDENTICALLY: source hold released (900 / 0), destination
	// credited (100 / 0) — the transition surface does not change the ledger effect.
	for _, bal := range []struct {
		src, dst uuid.UUID
		lane     string
	}{
		{aSrc, aDst, "ledger A (v2 hold -> v1 commit)"},
		{bSrc, bDst, "ledger B (v1 hold -> v2 commit)"},
	} {
		requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, bal.src), "%s source available after commit", bal.lane)
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, bal.src), "%s source on-hold released after commit", bal.lane)
		requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, bal.dst), "%s dest credited after commit", bal.lane)
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, bal.dst), "%s dest on-hold after commit", bal.lane)
	}
}

// =============================================================================
// 15. LIFECYCLE STATE + ID ERRORS: the v2 commit/cancel ops surface the SAME state and
//     path errors the v1 ops do, because they reuse the SAME commitTransaction core and the
//     SAME ParseUUIDPathParameters Fiber guard:
//       - commit of a non-PENDING (already-APPROVED direct) tx -> 409 ErrCommitTransactionNotPending (0099, EntityConflictError)
//       - cancel of the same already-APPROVED tx              -> 409 ErrCommitTransactionNotPending (0099, EntityConflictError)
//       - commit of an unknown (well-formed) transaction_id   -> 404 ErrEntityNotFound (0007, generic query not-found)
//       - cancel of an unknown (well-formed) transaction_id   -> 404 ErrEntityNotFound (0007, generic query not-found)
//       - commit with a malformed transaction_id              -> 400 ErrInvalidPathParameter (0065, blocked on the chain)
//       - cancel with a malformed transaction_id              -> 400 ErrInvalidPathParameter (0065, blocked on the chain)
// =============================================================================

func TestIntegration_TransactionV2Lifecycle_StateAndIDErrors(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	// A v2 `direct` transfer settles immediately (APPROVED): it is the non-PENDING subject for
	// the commit/cancel state-error cases below.
	directResp := decodeTxResponse(t, postTransaction(t, v2App, v2DirectURL(infra.orgID, infra.ledgerID), equivalentV2Body, ""), nethttp.StatusCreated)
	approvedTxID := uuid.MustParse(directResp["id"].(string))
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, approvedTxID), "the v2 direct transaction should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	unknownTxID := uuid.Must(libCommons.GenerateUUIDv7())

	cases := []struct {
		name       string
		url        string
		wantStatus int
		wantCode   string
	}{
		{
			// A non-PENDING transaction is an invalid-status conflict: ErrCommitTransactionNotPending
			// maps to EntityConflictError -> 409 (see pkg/errors.go), not 422.
			name:       "commit of an already-APPROVED transaction is not pending",
			url:        v2CommitURL(infra.orgID, infra.ledgerID, approvedTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrCommitTransactionNotPending.Error(),
		},
		{
			name:       "cancel of an already-APPROVED transaction is not pending",
			url:        v2CancelURL(infra.orgID, infra.ledgerID, approvedTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrCommitTransactionNotPending.Error(),
		},
		{
			// The query not-found path (write-behind miss -> GetTransactionByID miss) returns the
			// generic ErrEntityNotFound (0007) -> 404, entityType "Transaction".
			name:       "commit of an unknown transaction id is not found",
			url:        v2CommitURL(infra.orgID, infra.ledgerID, unknownTxID),
			wantStatus: nethttp.StatusNotFound,
			wantCode:   cn.ErrEntityNotFound.Error(),
		},
		{
			// Cancel mirrors commit on the same unknown (well-formed) id: the query not-found
			// path returns the generic ErrEntityNotFound (0007) -> 404.
			name:       "cancel of an unknown transaction id is not found",
			url:        v2CancelURL(infra.orgID, infra.ledgerID, unknownTxID),
			wantStatus: nethttp.StatusNotFound,
			wantCode:   cn.ErrEntityNotFound.Error(),
		},
		{
			// Malformed transaction_id is rejected by ParseUUIDPathParameters on the Fiber
			// chain BEFORE the Huma terminal — canonical 400, never a native Huma 422.
			name:       "commit with a malformed transaction id is a bad request",
			url:        "/v2/organizations/" + infra.orgID.String() + "/ledgers/" + infra.ledgerID.String() + "/transactions/not-a-uuid/commit",
			wantStatus: nethttp.StatusBadRequest,
			wantCode:   cn.ErrInvalidPathParameter.Error(),
		},
		{
			// Cancel mirrors commit on a malformed transaction_id: ParseUUIDPathParameters blocks
			// it on the Fiber chain BEFORE the Huma terminal — canonical 400, never a Huma 422.
			name:       "cancel with a malformed transaction id is a bad request",
			url:        "/v2/organizations/" + infra.orgID.String() + "/ledgers/" + infra.ledgerID.String() + "/transactions/not-a-uuid/cancel",
			wantStatus: nethttp.StatusBadRequest,
			wantCode:   cn.ErrInvalidPathParameter.Error(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postTransaction(t, v2App, tc.url, "", "")
			body := drainBody(t, resp)

			assert.Equal(t, tc.wantStatus, resp.StatusCode, "%s: unexpected status; body: %s", tc.name, string(body))

			// Field-level check: unmarshal the RFC 9457 problem+json envelope and assert the
			// `code` field EQUALS the expected code (not a whole-body substring match, which
			// could pass on an incidental occurrence anywhere in the payload).
			var problem struct {
				Code string `json:"code"`
			}
			require.NoErrorf(t, json.Unmarshal(body, &problem), "%s: response body should be valid problem+json; body: %s", tc.name, string(body))
			assert.Equalf(t, tc.wantCode, problem.Code, "%s: problem+json code field should equal %s; body: %s", tc.name, tc.wantCode, string(body))
		})
	}

	// The rejected lifecycle calls left the direct transaction APPROVED and created no extra
	// transaction: exactly the one direct transfer is persisted.
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, approvedTxID), "the direct transaction must stay APPROVED after the rejected commit/cancel")
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "rejected lifecycle calls must not persist new transactions")
}
