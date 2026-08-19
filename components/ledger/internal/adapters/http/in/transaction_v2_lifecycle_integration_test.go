// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"sync"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// This file is the v2 transaction LIFECYCLE integration + parity proof: it exercises the v2
// `commit`, `cancel`, and `revert` ops end-to-end against the SAME real-repo handler backed by
// testcontainers as the sibling create/hold/block file. The split from
// transaction_v2_handler_integration_test.go is topical, not budgetary (this repo enforces no
// file-size limit): that file covers the CREATE surface (direct/hold/block/unblock), this one
// covers what happens to a transaction AFTER it exists. Both files share the same `in` package
// and reuse the shared request/decode/DB helpers, `seedTransfer`, `setupTestInfra`, and the app
// builders defined in the sibling files.
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
	v2HoldResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, ledgerV2, holdParityV2Body, ""), nethttp.StatusCreated)
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
	// on each, with the economic projection IDENTICAL between the two surfaces.
	v1Ops := fetchOperationRows(t, infra.pgContainer.DB, v1TxID)
	v2Ops := fetchOperationRows(t, infra.pgContainer.DB, v2TxID)
	require.Len(t, v1Ops, 3, "committed v1 hold should carry 3 operations (ON_HOLD + DEBIT + CREDIT)")
	require.Len(t, v2Ops, 3, "committed v2 hold should carry 3 operations (ON_HOLD + DEBIT + CREDIT)")
	assertOperationSetsEqual(t, v1Ops, v2Ops)

	// Commit response deep-equal, ignoring IDs/timestamps: the v2 commit response (with the
	// settled operations embedded) is indistinguishable from the v1 commit response.
	require.Equal(t, stripVolatile(v1CommitResp), stripVolatile(renameV2LegKeys(v2CommitResp)),
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
	v2HoldResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, ledgerV2, holdParityV2Body, ""), nethttp.StatusCreated)
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
	require.Equal(t, stripVolatile(v1CancelResp), stripVolatile(renameV2LegKeys(v2CancelResp)),
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
	aHoldResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, ledgerA, holdParityV2Body, ""), nethttp.StatusCreated)
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
//
//     The unknown-id rows here are the CORRECT contract for a missing entity (404/0007).
//     Revert diverges on that same input class and answers 409/0099 — a known defect recorded
//     in section 17's unknown-id case, which names the root cause and the fix contract.
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
	directResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
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
			assertProblemCode(t, postTransaction(t, v2App, tc.url, "", ""), tc.wantStatus, tc.wantCode)
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

	v1RevertRaw := postTransaction(t, v1App, v1RevertURL(infra.orgID, ledgerV1, v1OriginID), "", "")
	assert.Equal(t, "false", v1RevertRaw.Header.Get("X-Idempotency-Replayed"),
		"a fresh v1 revert must advertise X-Idempotency-Replayed: false — the header is what distinguishes it from a replay")

	v1RevertResp := decodeTxResponse(t, v1RevertRaw, nethttp.StatusCreated)
	v1RevertID := uuid.MustParse(v1RevertResp["id"].(string))
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1RevertID), "the v1 reverse transaction should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	// v2 lane: transfer -> revert through the v2 revert op.
	v2OriginResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, ledgerV2, equivalentV2Body, ""), nethttp.StatusCreated)
	v2OriginID := uuid.MustParse(v2OriginResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2OriginID), "the v2 origin transfer should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	v2RevertRaw := postTransaction(t, v2App, v2RevertURL(infra.orgID, ledgerV2, v2OriginID), "", "")
	assert.Equal(t, "false", v2RevertRaw.Header.Get("X-Idempotency-Replayed"),
		"a fresh v2 revert must advertise X-Idempotency-Replayed: false, byte-identically to the v1 surface")

	v2RevertResp := decodeTxResponse(t, v2RevertRaw, nethttp.StatusCreated)
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

	// Full economic parity of the two reverse operation sets.
	assertOperationSetsEqual(t,
		fetchOperationRows(t, infra.pgContainer.DB, v1RevertID),
		fetchOperationRows(t, infra.pgContainer.DB, v2RevertID))

	// Revert response deep-equal, ignoring IDs/timestamps: the v2 revert response (with the
	// reverse operations embedded) is indistinguishable from the v1 revert response.
	require.Equal(t, stripVolatile(v1RevertResp), stripVolatile(renameV2LegKeys(v2RevertResp)),
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
	// The default params carry a 100 USD send body, which would contradict the zero/no-asset
	// columns above. The revert gate reads the columns and the operations, never the body, so
	// the row is left body-less to stay self-consistent (the column is nullable).
	params.Body = nil

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

// assertProblemCode drains an error response and asserts its HTTP status plus the `code`
// field of its RFC 9457 problem+json envelope. The code is compared as a FIELD, not as a
// whole-body substring, so an incidental occurrence of the same digits anywhere else in the
// payload cannot make the assertion pass. The failing body is dumped on every mismatch.
func assertProblemCode(t *testing.T, resp *nethttp.Response, wantStatus int, wantCode string) {
	t.Helper()

	body := drainBody(t, resp)

	assert.Equalf(t, wantStatus, resp.StatusCode, "unexpected status; body: %s", string(body))

	var problem struct {
		Code string `json:"code"`
	}

	require.NoErrorf(t, json.Unmarshal(body, &problem), "response body should be valid problem+json; body: %s", string(body))
	assert.Equalf(t, wantCode, problem.Code, "problem+json code field should equal %s; body: %s", wantCode, string(body))
}

// =============================================================================
// 17. REVERT INELIGIBILITY + ID ERRORS: the v2 revert op surfaces the SAME eligibility and
//     path errors the v1 op does, because it reuses the SAME revertTransaction gate and the
//     SAME ParseUUIDPathParameters Fiber guard. Each gate has its own sentinel and its own
//     status CLASS:
//       - origin that was ALREADY reverted (a child revert exists)
//             -> 201 with the exact durable reverse and X-Idempotency-Replayed=true
//       - the reverse transaction itself (carries a ParentTransactionID)
//             -> 409 ErrTransactionIDIsAlreadyARevert (0088, EntityConflictError)
//       - a non-APPROVED transaction (PENDING or CANCELED)
//             -> 409 ErrCommitTransactionNotPending (0099, EntityConflictError)
//       - a degenerate transaction whose reversal is empty
//             -> 422 ErrTransactionCantRevert (0089, UnprocessableOperationError)
//       - a transaction whose operations point at a NON-bidirectional operation route
//             -> 422 ErrRouteNotBidirectional (0150, UnprocessableOperationError)
//       - an unknown (well-formed) transaction_id
//             -> 409 ErrCommitTransactionNotPending (0099) — a KNOWN DEFECT, not the intended
//                contract. See the case body for the root cause and the fix contract; commit
//                and cancel correctly return 404/0007 for the same input class (section 15).
//       - a malformed transaction_id
//             -> 400 ErrInvalidPathParameter (0065, blocked on the Fiber chain)
// =============================================================================

func TestIntegration_TransactionV2Revert_IneligibilityAndIDErrors(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	// Both apps are built up-front, before any request runs: the builders call the
	// process-global libProblem.Install() (see file header), so building one mid-test would
	// re-install that hook underneath a surface already in use.
	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// Subject pair 1: an APPROVED direct transfer and its reverse. After this pair the origin
	// MUST replay its durable reverse while the reverse itself remains ineligible.
	originResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originTxID := uuid.MustParse(originResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originTxID), "the origin transfer should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	revertResp := decodeTxResponse(t, postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originTxID), "", ""), nethttp.StatusCreated)
	revertTxID := uuid.MustParse(revertResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, revertTxID), "the reverse transaction should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Subject 2: a hold left PENDING. Its description differs from the cancel subject below so
	// the two no-key holds hash into DISTINCT idempotency slots instead of replaying.
	pendingResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID,
		`{"description":"revert gate pending subject","asset":"USD","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`, ""), nethttp.StatusCreated)
	pendingTxID := uuid.MustParse(pendingResp["id"].(string))
	require.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingTxID), "the hold subject should be PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Subject 3: a hold cancelled through the v2 cancel op -> CANCELED.
	cancelResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID,
		`{"description":"revert gate cancel subject","asset":"USD","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`, ""), nethttp.StatusCreated)
	canceledTxID := uuid.MustParse(cancelResp["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	_ = decodeTxResponse(t, postTransaction(t, v2App, v2CancelURL(infra.orgID, infra.ledgerID, canceledTxID), "", ""), nethttp.StatusCreated)
	require.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, canceledTxID), "the cancel subject should be CANCELED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Subject 4: an APPROVED transaction whose reversal is empty.
	emptyReversalTxID := seedEmptyReversalTransaction(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID)

	// Subject 5: an APPROVED direct transfer whose operations are stamped with a
	// NON-bidirectional operation route, which is the only input that reaches the
	// bidirectional-route gate. The route is created and stamped AFTER the transfer settles,
	// so nothing about the create path changes; the gate reads only op.RouteID off the
	// transaction's operations and resolves the route by id, so the ledger's route-validation
	// setting (off in this harness) is irrelevant to reaching it.
	nonBidiResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID,
		`{"description":"revert gate non bidirectional subject","asset":"USD","amount":"100","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"100"}]}`, ""), nethttp.StatusCreated)
	nonBidiTxID := uuid.MustParse(nonBidiResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, nonBidiTxID), "the non-bidirectional subject should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	sourceRouteID := postgrestestutil.CreateTestOperationRouteSimple(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "Revert Gate Source Route", "source")
	postgrestestutil.StampOperationRoute(t, infra.pgContainer.DB, nonBidiTxID, sourceRouteID)

	unknownTxID := uuid.Must(libCommons.GenerateUUIDv7())

	cases := []struct {
		name       string
		url        string
		wantStatus int
		wantCode   string
	}{
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
			// Only routes whose OperationType is "bidirectional" may be reverted:
			// ErrRouteNotBidirectional is an UnprocessableOperationError (see pkg/errors.go)
			// -> 422. Reached because subject 5's operations carry a route_id pointing at a
			// "source" route.
			name:       "revert of a transaction on a non-bidirectional route is unprocessable",
			url:        v2RevertURL(infra.orgID, infra.ledgerID, nonBidiTxID),
			wantStatus: nethttp.StatusUnprocessableEntity,
			wantCode:   cn.ErrRouteNotBidirectional.Error(),
		},
		{
			// KNOWN DEFECT (tracked externally) — this expectation records the
			// CURRENT behavior, not the intended contract. A missing entity must render 404
			// (error-handling standard E3/E5), and
			// commit/cancel do exactly that for the same input class (404/0007, section 15).
			// Revert diverges because its read path FindWithOperations
			// (adapters/postgres/transaction/transaction.postgresql.go) has no sql.ErrNoRows
			// arm — unlike FindByParentID in the same file — so zero join rows return the
			// zero-value &Transaction{} with a nil error. The empty status "" then trips the
			// non-APPROVED gate, and the client sees 409/0099 instead of 404/0007.
			//
			// If this starts returning 404/0007 the not-found gap was fixed — update this
			// expectation, do NOT revert the fix.
			//
			// Fixing the read path is OUT OF SCOPE here, and whoever does it must land the
			// caller guard in the SAME change: revertTransaction dereferences the result
			// (tran.ParentTransactionID) immediately after the error check, so a fix that
			// returns (nil, nil) — or any nil transaction with a nil error — converts this
			// wrong status into a nil-pointer panic.
			name:       "revert of an unknown transaction id currently conflicts (0099) instead of 404 — KNOWN DEFECT",
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
			assertProblemCode(t, postTransaction(t, v2App, tc.url, "", ""), tc.wantStatus, tc.wantCode)
		})
	}

	v2ReplayResponse := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, originTxID), "", "")
	assert.Equal(t, "true", v2ReplayResponse.Header.Get("X-Idempotency-Replayed"))
	v2Replay := decodeTxResponse(t, v2ReplayResponse, nethttp.StatusCreated)
	assert.Equal(t, revertTxID.String(), v2Replay["id"], "same-origin v2 retry must return the durable reverse")
	v1ReplayResponse := postTransaction(t, v1App, v1RevertURL(infra.orgID, infra.ledgerID, originTxID), "", "")
	assert.Equal(t, "true", v1ReplayResponse.Header.Get("X-Idempotency-Replayed"))
	v1Replay := decodeTxResponse(t, v1ReplayResponse, nethttp.StatusCreated)
	assert.Equal(t, revertTxID.String(), v1Replay["id"], "same-origin v1 retry must return the durable reverse")

	// v1↔v2 ineligibility parity, sampled on the two gates whose contract is easiest to get
	// wrong: the same unknown subject rejected through the EXISTING v1 revert endpoint returns the
	// byte-identical problem envelope, so the v2 surface adds no divergent error behavior.
	for _, tc := range []struct {
		name string
		txID uuid.UUID
	}{
		{"unknown transaction id", unknownTxID},
	} {
		v1Resp := postTransaction(t, v1App, v1RevertURL(infra.orgID, infra.ledgerID, tc.txID), "", "")
		v1Body := drainBody(t, v1Resp)

		v2Resp := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, tc.txID), "", "")
		v2Body := drainBody(t, v2Resp)

		assert.Equal(t, v1Resp.StatusCode, v2Resp.StatusCode, "%s: v1 and v2 revert must reject with the same status", tc.name)
		assert.JSONEq(t, string(v1Body), string(v2Body), "%s: v1 and v2 revert must reject with the same problem envelope", tc.name)
	}

	// Every rejected or replayed revert left its subject exactly as it was and persisted nothing:
	// the five created transactions plus the seeded degenerate one, and no extra reverse.
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originTxID), "the origin must stay APPROVED after the rejected reverts")
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, revertTxID), "the reverse must stay APPROVED after the rejected reverts")
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, pendingTxID), "the PENDING subject must stay PENDING")
	assert.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, canceledTxID), "the CANCELED subject must stay CANCELED")
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, emptyReversalTxID), "the degenerate subject must not acquire a reverse")
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, nonBidiTxID), "the non-bidirectional subject must not acquire a reverse")
	assert.Equal(t, 6, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "rejected reverts must not persist new transactions")
}

// =============================================================================
// 19. CONCURRENT REVERT OF ONE ORIGIN — EXACTLY ONE MUTATION (money path):
//     concurrent requests share the PostgreSQL claim and origin Redis barrier.
//     Callers racing after persistence may receive the same successful replay;
//     only one reverse transaction and one balance mutation may exist.
//
//     N racers revert one APPROVED origin from a common start barrier. The invariant asserted
//     is "exactly one 201, every other answer a 4xx" rather than one specific sentinel: the
//     loser's code depends on the interleaving (409/0084 when it loses the SetNX before the
//     winner's response is cached, 409/0087 once the winner's child is visible to the gate).
//     The observed codes are logged for the record.
//
//     CONCURRENCY NOTE: only the REQUESTS race. The app is built once, before the barrier —
//     the file header's sequential rule is about libProblem.Install() and the app builders,
//     not about serving; fiber's App.Test guards startupProcess with app.mutex and each call
//     serves its own connection.
// =============================================================================

// concurrentRevertRacers is the number of goroutines that revert the single origin at once.
// Small enough to stay fast, large enough that at least one racer loses the claim rather than
// the gate.
const concurrentRevertRacers = 8

// revertRaceResult is one racer's outcome, captured WITHOUT touching *testing.T: require/assert
// call t.FailNow, which is illegal outside the test goroutine, so every racer records its raw
// outcome and the assertions all run on the test goroutine after the WaitGroup drains.
type revertRaceResult struct {
	transportErr error
	status       int
	replayed     string
	txID         string
	problemCode  string
	body         string
}

// fireRevert issues one revert request and decodes whichever envelope came back (a transaction
// on success, an RFC 9457 problem on rejection). Goroutine-safe by construction: it reports
// failures as data instead of failing the test from a non-test goroutine.
func fireRevert(app *fiber.App, url string) revertRaceResult {
	req := httptest.NewRequest(nethttp.MethodPost, url, nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	if err != nil {
		return revertRaceResult{transportErr: err}
	}

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return revertRaceResult{transportErr: err, status: resp.StatusCode}
	}

	out := revertRaceResult{
		status:   resp.StatusCode,
		replayed: resp.Header.Get("X-Idempotency-Replayed"),
		body:     string(raw),
	}

	var envelope struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}

	if err := json.Unmarshal(raw, &envelope); err != nil {
		return out
	}

	out.txID = envelope.ID
	out.problemCode = envelope.Code

	return out
}

func TestIntegration_TransactionV2Revert_ConcurrentSingleWinner(t *testing.T) {
	// NOT parallel: process-global huma state (see file header). The requests inside race; the
	// test itself does not run alongside other tests.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// BOTH sides start funded, and that is load-bearing. A reverse DEBITS the destination, so
	// an empty destination (seedTransfer's default) can only afford ONE reversal: the atomic
	// balance commit would then reject every duplicate with 0018/insufficient-funds and the
	// "exactly one" assertion below would hold even with the idempotency claim removed — the
	// test would be measuring the destination's balance, not the claim. Funding the destination
	// with headroom makes a SECOND reversal affordable, so if the claim ever stops serializing,
	// the duplicate reverse really does commit and this test fails (verified by temporarily
	// forcing a per-call-unique idempotency preimage on the revert path).
	srcID, dstID := seedFundedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000, 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	originResp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, equivalentV2Body, ""), nethttp.StatusCreated)
	originID := uuid.MustParse(originResp["id"].(string))
	require.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originID), "the origin transfer should be APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available after the origin transfer")
	requireDecimalEqual(t, decimal.NewFromInt(1100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination available after the origin transfer")

	// Race: every goroutine is parked on the same closed-channel barrier so they enter the
	// revert core together rather than in the order they were spawned.
	revertURL := v2RevertURL(infra.orgID, infra.ledgerID, originID)
	results := make([]revertRaceResult, concurrentRevertRacers)
	start := make(chan struct{})

	var wg sync.WaitGroup

	for i := range results {
		wg.Add(1)

		go func(slot int) {
			defer wg.Done()

			<-start

			results[slot] = fireRevert(v2App, revertURL)
		}(i)
	}

	close(start)
	wg.Wait()

	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// The create core has THREE terminal outcomes here, not two, so status alone cannot
	// classify a racer:
	//
	//   winner  — 201 + X-Idempotency-Replayed: false. Created the one reverse, moved money.
	//   replay  — 201 + X-Idempotency-Replayed: true. Lost the claim, then read the winner's
	//             persisted reverse from PostgreSQL primary and echoed it. Creates nothing, moves
	//             nothing, and MUST NOT be counted against the exactly-one-create invariant.
	//   loser   — 4xx while the claim is active, or 503/0505 after Redis has
	//             committed but before PostgreSQL persistence is visible.
	//
	// Which of the two losing shapes a racer gets is a pure timing coin-flip against the
	// winner's async cache write, so folding replays into winners would make the
	// single-winner assertion flaky AND invert its message: it would report "reverted more
	// than once" for a request that reverted nothing.
	var (
		winners    []revertRaceResult
		replays    []revertRaceResult
		loserCodes []string
	)

	// Losing reverts may only be rejected for LOSING THE CLAIM. 0018/insufficient-funds in
	// particular must never appear: that would mean the destination ran out of headroom and
	// the balance engine — not the idempotency claim — is what kept the count at one, which is
	// exactly the accidental backstop seedFundedTransfer above exists to remove.
	allowedLoserCodes := []string{
		cn.ErrIdempotencyKey.Error(),
		cn.ErrTransactionIDHasAlreadyParentTransaction.Error(),
		cn.ErrRevertReconciliationRequired.Error(),
	}

	for i, res := range results {
		require.NoErrorf(t, res.transportErr, "racer %d failed at the transport layer", i)

		if res.status == nethttp.StatusCreated {
			if res.replayed == "true" {
				replays = append(replays, res)
				continue
			}

			winners = append(winners, res)

			continue
		}

		assert.GreaterOrEqualf(t, res.status, 400, "racer %d: a losing revert must be fenced; body: %s", i, res.body)
		if res.problemCode == cn.ErrRevertReconciliationRequired.Error() {
			assert.Equalf(t, nethttp.StatusServiceUnavailable, res.status,
				"racer %d: post-movement reconciliation must surface as 503; body: %s", i, res.body)
		} else {
			assert.Lessf(t, res.status, 500,
				"racer %d: an active pre-movement claim is a client conflict; body: %s", i, res.body)
		}
		assert.Containsf(t, allowedLoserCodes, res.problemCode,
			"racer %d: a losing revert may only be rejected for losing the idempotency claim (%v); body: %s",
			i, allowedLoserCodes, res.body)

		loserCodes = append(loserCodes, res.problemCode)
	}

	t.Logf("concurrent revert of one origin: %d/%d created, %d replayed, loser problem codes %v",
		len(winners), concurrentRevertRacers, len(replays), loserCodes)

	require.Lenf(t, winners, 1,
		"exactly ONE concurrent revert of a single origin may CREATE a reverse; %d did, which means the shared idempotency claim did NOT serialize the read-then-act race and the origin was reverted more than once",
		len(winners))

	assert.Equal(t, "false", winners[0].replayed,
		"the winning revert is a fresh create, so it must advertise X-Idempotency-Replayed: false explicitly — an absent header is indistinguishable from a transport that forgot to project the flag")

	// One reverse persisted, linked to the origin — and the origin itself never acquired a parent.
	require.NotEmpty(t, winners[0].txID, "the winning revert must return the reverse transaction id")
	reverseID := uuid.MustParse(winners[0].txID)

	// A replay must hand back the WINNER's reverse. Nothing else in the suite proves this: if
	// the slot ever answered with some other cached value, the replaying caller would receive a
	// 201 naming a transaction that is not the reversal of their origin.
	for i, res := range replays {
		assert.Equalf(t, winners[0].txID, res.txID,
			"replay %d must echo the winning reverse (%s), not any other cached transaction; body: %s",
			i, winners[0].txID, res.body)
	}

	reverseParent := postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, reverseID)
	require.NotNil(t, reverseParent, "the persisted reverse must carry a parent")
	assert.Equal(t, originID, *reverseParent, "the persisted reverse must point at the raced origin")
	assert.Nil(t, postgrestestutil.GetTransactionParentID(t, infra.pgContainer.DB, originID), "the origin must not acquire a parent")

	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"the ledger must hold exactly the origin and ONE reverse; a third transaction is a double revert")

	// Balances reverted exactly ONCE: back to the seeded state, not past it. Both sides have the
	// headroom to absorb a second reversal, so these two numbers are the money-path proof — a
	// double revert reads 1100 / 900.
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available restored exactly once (a double revert would read 1100)")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination drained back exactly once (a double revert would read 900)")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID), "source on-hold after the raced revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, dstID), "destination on-hold after the raced revert")
}
