// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"database/sql"
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

// v2RevertURL / v1RevertURL build the concrete revert paths for a transaction. The v2 op is
// mounted by the SAME RegisterTransactionV2RoutesToApp seam as direct/hold/commit/cancel and
// reuses the v1 RevertTransactionHuma shell verbatim, so both surfaces enter the SAME
// revertTransaction eligibility gate and createRevertTransaction core.
func v2RevertURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/revert"
}

func v1RevertURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/revert"
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

// =============================================================================
// 16. REVERT VIA V2 (lifecycle parity, core): an APPROVED transaction reverted through the
//     v2 revert op produces the SAME reverse transaction a v1 revert of the equivalent
//     origin produces — the reverse is linked to its origin via ParentTransactionID, the
//     accounting direction is inverted leg-for-leg (the origin's debited source becomes the
//     reverse's credited destination and vice versa), the net balances return to the
//     pre-transaction state, and the persisted operation set plus the revert response are
//     indistinguishable between the two surfaces (ignoring IDs/timestamps).
// =============================================================================

func TestIntegration_TransactionV2Revert_ParityWithV1(t *testing.T) {
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

	// v1 lane: transfer -> revert, each step fully processed (create -> drain) BEFORE the v2
	// lane, because the balance-sync schedule ZSET is GLOBAL, not per-ledger (see the direct
	// parity test for the same discipline).
	v1OriginResp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerV1), equivalentV1Body, ""), nethttp.StatusCreated)
	v1OriginID := uuid.MustParse(v1OriginResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1OriginID), "the v1 origin transfer should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	v1RevertResp := decodeTxResponse(t, postTransaction(t, v1App, v1RevertURL(infra.orgID, ledgerV1, v1OriginID), "", ""), nethttp.StatusCreated)
	v1RevertID := uuid.MustParse(v1RevertResp["id"].(string))
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1RevertID), "the v1 reverse transaction should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	// v2 lane: transfer -> revert through the v2 revert op.
	v2OriginResp := decodeTxResponse(t, postTransaction(t, v2App, v2DirectURL(infra.orgID, ledgerV2), equivalentV2Body, ""), nethttp.StatusCreated)
	v2OriginID := uuid.MustParse(v2OriginResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2OriginID), "the v2 origin transfer should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	v2RevertResp := decodeTxResponse(t, postTransaction(t, v2App, v2RevertURL(infra.orgID, ledgerV2, v2OriginID), "", ""), nethttp.StatusCreated)
	v2RevertID := uuid.MustParse(v2RevertResp["id"].(string))
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2RevertID), "the v2 reverse transaction should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	// Parent link — the core assertion of the v2 revert surface. Read from the response
	// BEFORE stripVolatile (which deletes parentTransactionId in place) AND from the
	// persisted row, on BOTH surfaces.
	assert.Equal(t, v1OriginID.String(), v1RevertResp["parentTransactionId"], "the v1 reverse response must link back to its origin")
	assert.Equal(t, v2OriginID.String(), v2RevertResp["parentTransactionId"], "the v2 reverse response must link back to its origin")

	v1Parent := postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, v1RevertID)
	require.NotNil(t, v1Parent, "the persisted v1 reverse transaction must carry a parent")
	assert.Equal(t, v1OriginID, *v1Parent, "the persisted v1 reverse must point at the reverted transaction")

	v2Parent := postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, v2RevertID)
	require.NotNil(t, v2Parent, "the persisted v2 reverse transaction must carry a parent")
	assert.Equal(t, v2OriginID, *v2Parent, "the persisted v2 reverse must point at the reverted transaction")

	// The link is one-directional: reverting does not stamp a parent onto the origin, so the
	// origin stays revertable-once rather than becoming a revert itself.
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, v1OriginID), "the v1 origin must not acquire a parent")
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, v2OriginID), "the v2 origin must not acquire a parent")

	// Net balances back to the pre-transaction state: source restored to its full 1000, the
	// destination emptied back to 0, no on-hold anywhere, IDENTICAL on both surfaces.
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Src), "v1 source available restored after revert")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Src), "v2 source available restored after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Dst), "v1 dest drained back after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Dst), "v2 dest drained back after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Src), "v1 source on-hold after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Src), "v2 source on-hold after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Dst), "v1 dest on-hold after revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Dst), "v2 dest on-hold after revert")

	// Inverted accounting direction: the origin debits @src and credits @dst; each reverse
	// credits @src and debits @dst, for the same amount, landing the balance-after back on the
	// seeded state. Both legs of a reverse carry distinct aliases, so the alias join lines them
	// up across the origin/reverse pair.
	v1OriginOps := indexOpsByAlias(t, fetchOperationRows(t, infra.pgContainer.DB, v1OriginID))
	v1RevertOps := indexOpsByAlias(t, fetchOperationRows(t, infra.pgContainer.DB, v1RevertID))
	v2RevertOps := indexOpsByAlias(t, fetchOperationRows(t, infra.pgContainer.DB, v2RevertID))

	require.Len(t, v1OriginOps, 2, "the origin transfer should carry 2 operations")
	require.Len(t, v1RevertOps, 2, "the v1 reverse should carry 2 operations")
	require.Len(t, v2RevertOps, 2, "the v2 reverse should carry 2 operations")

	assert.Equal(t, cn.DEBIT, v1OriginOps["@src"].Type, "the origin debits the source")
	assert.Equal(t, cn.CREDIT, v1OriginOps["@dst"].Type, "the origin credits the destination")

	for lane, revertOps := range map[string]map[string]operationEconomicRow{"v1": v1RevertOps, "v2": v2RevertOps} {
		assert.Equal(t, cn.CREDIT, revertOps["@src"].Type, "%s reverse must CREDIT the originally debited source", lane)
		assert.Equal(t, cn.DEBIT, revertOps["@dst"].Type, "%s reverse must DEBIT the originally credited destination", lane)
		requireDecimalEqual(t, v1OriginOps["@src"].Amount, revertOps["@src"].Amount, "%s reverse source amount mirrors the origin", lane)
		requireDecimalEqual(t, v1OriginOps["@dst"].Amount, revertOps["@dst"].Amount, "%s reverse destination amount mirrors the origin", lane)
		requireDecimalEqual(t, decimal.NewFromInt(1000), revertOps["@src"].AvailableAfter, "%s reverse restores the source available balance", lane)
		requireDecimalEqual(t, decimal.Zero, revertOps["@dst"].AvailableAfter, "%s reverse drains the destination available balance", lane)
	}

	// Full economic parity of the two reverse operation sets: fetchOperationRows orders by
	// type, so the two 2-leg sets line up index-for-index.
	assertOperationSetsEqual(t,
		fetchOperationRows(t, infra.pgContainer.DB, v1RevertID),
		fetchOperationRows(t, infra.pgContainer.DB, v2RevertID))

	// Revert response deep-equal, ignoring IDs/timestamps: the v2 revert response (with the
	// reverse operations embedded) is indistinguishable from the v1 revert response.
	require.Equal(t, stripVolatile(v1RevertResp), stripVolatile(v2RevertResp),
		"v2 revert response must be indistinguishable from the v1 revert equivalent (ignoring IDs/timestamps)")

	// Exactly one origin + one reverse per ledger: neither surface produced a stray extra
	// transaction while reverting.
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, ledgerV1), "the v1 lane should hold exactly the origin and its reverse")
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, ledgerV2), "the v2 lane should hold exactly the origin and its reverse")
}

// seedEmptyReversalTransaction persists an APPROVED transaction whose reversal is degenerate
// — zero send value and no asset code — plus the single operation row the revert read path
// inner-joins on. The create path cannot produce this shape, so the row is seeded directly to
// reach the empty-reversal branch of the eligibility gate.
func seedEmptyReversalTransaction(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID) uuid.UUID {
	t.Helper()

	params := postgrestestutil.DefaultTransactionParams()
	params.Status = cn.APPROVED
	params.Amount = decimal.Zero
	params.AssetCode = ""

	txID := postgrestestutil.CreateTestTransaction(t, db, orgID, ledgerID, params)

	postgrestestutil.CreateTestOperation(t, db, orgID, ledgerID, postgrestestutil.OperationParams{
		TransactionID: txID,
		Description:   "degenerate reversal leg",
		Type:          cn.DEBIT,
		AccountID:     uuid.Must(libCommons.GenerateUUIDv7()),
		AccountAlias:  "@degenerate",
		BalanceID:     uuid.Must(libCommons.GenerateUUIDv7()),
		AssetCode:     "",
		Amount:        decimal.Zero,
		Status:        cn.APPROVED,
	})

	return txID
}

// =============================================================================
// 17. REVERT INELIGIBILITY + ID ERRORS: the v2 revert op surfaces the SAME eligibility and
//     path errors the v1 op does, because it reuses the SAME revertTransaction gate and the
//     SAME ParseUUIDPathParameters Fiber guard. Each gate has its own sentinel and its own
//     status CLASS:
//       - origin that was ALREADY reverted (a child revert exists)
//             -> 409 ErrTransactionIDHasAlreadyParentTransaction (0087, EntityConflictError)
//       - the reverse transaction itself (carries a ParentTransactionID)
//             -> 409 ErrTransactionIDIsAlreadyARevert (0088, EntityConflictError)
//       - a non-APPROVED transaction (PENDING or CANCELED)
//             -> 409 ErrCommitTransactionNotPending (0099, EntityConflictError)
//       - a degenerate transaction whose reversal is empty
//             -> 422 ErrTransactionCantRevert (0089, UnprocessableOperationError)
//       - an unknown (well-formed) transaction_id
//             -> 409 ErrCommitTransactionNotPending (0099): the revert read path returns a
//                ZERO-VALUE transaction rather than a not-found error, so the status gate —
//                not a 404 — is what rejects it. Shared with v1 by construction.
//       - a malformed transaction_id
//             -> 400 ErrInvalidPathParameter (0065, blocked on the Fiber chain)
//
//     The non-bidirectional-route gate (ErrRouteNotBidirectional) is NOT reachable here: the
//     harness ledger carries default settings (route validation off) and the transaction-only
//     container holds no operation_route rows, so every seeded operation has a nil route_id
//     and the bidirectional loop never executes.
// =============================================================================

func TestIntegration_TransactionV2Revert_IneligibilityAndIDErrors(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	// Subject pair 1: an APPROVED direct transfer and its reverse. After this pair the origin
	// HAS a child revert and the reverse IS one, so each is the subject of its own gate.
	originResp := decodeTxResponse(t, postTransaction(t, v2App, v2DirectURL(infra.orgID, infra.ledgerID), equivalentV2Body, ""), nethttp.StatusCreated)
	originTxID := uuid.MustParse(originResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originTxID), "the origin transfer should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	revertResp := decodeTxResponse(t, postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originTxID), "", ""), nethttp.StatusCreated)
	revertTxID := uuid.MustParse(revertResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, revertTxID), "the reverse transaction should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Subject 2: a hold left PENDING. Its description differs from the cancel subject below so
	// the two no-key holds hash into DISTINCT idempotency slots instead of replaying.
	pendingResp := decodeTxResponse(t, postTransaction(t, v2App, v2HoldURL(infra.orgID, infra.ledgerID),
		`{"description":"revert gate pending subject","asset":"USD","amount":"100","from":"@src","to":"@dst"}`, ""), nethttp.StatusCreated)
	pendingTxID := uuid.MustParse(pendingResp["id"].(string))
	require.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingTxID), "the hold subject should be PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Subject 3: a hold cancelled through the v2 cancel op -> CANCELED.
	cancelResp := decodeTxResponse(t, postTransaction(t, v2App, v2HoldURL(infra.orgID, infra.ledgerID),
		`{"description":"revert gate cancel subject","asset":"USD","amount":"100","from":"@src","to":"@dst"}`, ""), nethttp.StatusCreated)
	canceledTxID := uuid.MustParse(cancelResp["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	_ = decodeTxResponse(t, postTransaction(t, v2App, v2CancelURL(infra.orgID, infra.ledgerID, canceledTxID), "", ""), nethttp.StatusCreated)
	require.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, canceledTxID), "the cancel subject should be CANCELED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Subject 4: an APPROVED transaction whose reversal is empty.
	emptyReversalTxID := seedEmptyReversalTransaction(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID)

	unknownTxID := uuid.Must(libCommons.GenerateUUIDv7())

	cases := []struct {
		name       string
		url        string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "revert of an already reverted transaction conflicts on the existing child",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, originTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrTransactionIDHasAlreadyParentTransaction.Error(),
		},
		{
			name:       "revert of a reverse transaction conflicts because it is already a revert",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, revertTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrTransactionIDIsAlreadyARevert.Error(),
		},
		{
			name:       "revert of a PENDING transaction conflicts on the non-APPROVED status",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, pendingTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrCommitTransactionNotPending.Error(),
		},
		{
			name:       "revert of a CANCELED transaction conflicts on the non-APPROVED status",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, canceledTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrCommitTransactionNotPending.Error(),
		},
		{
			name:       "revert of a transaction with an empty reversal is unprocessable",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, emptyReversalTxID),
			wantStatus: nethttp.StatusUnprocessableEntity,
			wantCode:   cn.ErrTransactionCantRevert.Error(),
		},
		{
			// The revert read path (FindWithOperations) returns a ZERO-VALUE transaction for a
			// missing id instead of a not-found error, so the empty status "" trips the
			// non-APPROVED gate: the observable contract is 409/0099, NOT 404/0007. The v1 leg
			// asserted after this table proves the same id behaves identically there.
			name:       "revert of an unknown transaction id conflicts on the empty status",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, unknownTxID),
			wantStatus: nethttp.StatusConflict,
			wantCode:   cn.ErrCommitTransactionNotPending.Error(),
		},
		{
			// Malformed transaction_id is rejected by ParseUUIDPathParameters on the Fiber
			// chain BEFORE the Huma terminal — canonical 400, never a native Huma 422.
			name:       "revert with a malformed transaction id is a bad request",
			url:        "/v2/organizations/" + infra.orgID.String() + "/ledgers/" + infra.ledgerID.String() + "/transactions/not-a-uuid/revert",
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

	// v1↔v2 ineligibility parity, sampled on the two gates whose contract is easiest to get
	// wrong: the same subjects rejected through the EXISTING v1 revert endpoint return the
	// byte-identical problem envelope, so the v2 surface adds no divergent error behavior.
	v1App := buildHumaTransactionApp(t, infra.handler, true)

	for _, tc := range []struct {
		name string
		txID uuid.UUID
	}{
		{"already reverted origin", originTxID},
		{"unknown transaction id", unknownTxID},
	} {
		v1Resp := postTransaction(t, v1App, v1RevertURL(infra.orgID, infra.ledgerID, tc.txID), "", "")
		v1Body := drainBody(t, v1Resp)

		v2Resp := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, tc.txID), "", "")
		v2Body := drainBody(t, v2Resp)

		assert.Equal(t, v1Resp.StatusCode, v2Resp.StatusCode, "%s: v1 and v2 revert must reject with the same status", tc.name)
		assert.JSONEq(t, string(v1Body), string(v2Body), "%s: v1 and v2 revert must reject with the same problem envelope", tc.name)
	}

	// Every rejected revert left its subject exactly as it was and persisted nothing: the four
	// created transactions plus the seeded degenerate one, and no extra reverse.
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originTxID), "the origin must stay APPROVED after the rejected reverts")
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, revertTxID), "the reverse must stay APPROVED after the rejected reverts")
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingTxID), "the PENDING subject must stay PENDING")
	assert.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, canceledTxID), "the CANCELED subject must stay CANCELED")
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, emptyReversalTxID), "the degenerate subject must not acquire a reverse")
	assert.Equal(t, 5, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "rejected reverts must not persist new transactions")
}
