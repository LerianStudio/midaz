// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	nethttp "net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// This file is the end-to-end lock on the /v2 response rename: every one of the seven v2
// transaction ops (direct, hold, block, unblock create; commit, cancel, revert lifecycle) must
// answer with `debit`/`credit` and never with the v1 `source`/`destination` keys. Unlike the
// sibling parity files, this file does not compare against v1 — it asserts the /v2 shape
// directly, in one table over all seven ops, against a real committed transaction for each.

// v2AllOpsLeg builds one v2 leg object for the given alias, scoped to v2ScopeJSON.
func v2AllOpsLeg(alias string) string {
	return `{"alias":"` + alias + `",` + v2ScopeJSON + `,"amount":"100"}`
}

// v2AllOpsBody builds a minimal single-leg-per-side v2 create body.
func v2AllOpsBody(description, src, dst string) string {
	return `{"description":"` + description + `","asset":"USD","amount":"100",` +
		`"debits":[` + v2AllOpsLeg(src) + `],"credits":[` + v2AllOpsLeg(dst) + `]}`
}

func TestIntegration_TransactionV2AllOps_ResponseUsesDebitCreditKeys(t *testing.T) {
	// NOT parallel: process-global huma state (see the create/hold file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// create seeds a fresh, independent account pair (so the seven ops never contend over
	// shared funds), posts a v2 create action, and drains the resulting balance-sync effect.
	create := func(action, src, dst, description string) map[string]any {
		t.Helper()

		seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, src, dst, 1000)

		resp := postV2Create(t, v2App, action, infra.orgID, infra.ledgerID, v2AllOpsBody(description, src, dst), "")
		result := decodeTxResponse(t, resp, nethttp.StatusCreated)
		drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

		return result
	}

	responses := make(map[string]map[string]any, 7)

	responses["direct"] = create("direct", "@d1src", "@d1dst", "direct op")
	responses["block"] = create("block", "@bsrc", "@bdst", "block op")
	responses["unblock"] = create("unblock", "@usrc", "@udst", "unblock op")
	responses["hold"] = create("hold", "@hsrc", "@hdst", "hold op")

	// commit lane: a fresh hold, settled through the v2 commit op.
	commitHold := create("hold", "@csrc", "@cdst", "commit lane hold")
	commitTxID := uuid.MustParse(commitHold["id"].(string))
	commitResp := postTransaction(t, v2App, v2CommitURL(infra.orgID, infra.ledgerID, commitTxID), "", "")
	responses["commit"] = decodeTxResponse(t, commitResp, nethttp.StatusCreated)
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// cancel lane: a fresh hold, released through the v2 cancel op.
	cancelHold := create("hold", "@nsrc", "@ndst", "cancel lane hold")
	cancelTxID := uuid.MustParse(cancelHold["id"].(string))
	cancelResp := postTransaction(t, v2App, v2CancelURL(infra.orgID, infra.ledgerID, cancelTxID), "", "")
	responses["cancel"] = decodeTxResponse(t, cancelResp, nethttp.StatusCreated)
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// revert lane: a fresh settled direct, reverted through the v2 revert op.
	revertOrigin := create("direct", "@rsrc", "@rdst", "revert lane origin")
	revertOriginID := uuid.MustParse(revertOrigin["id"].(string))
	revertResp := postTransaction(t, v2App, v2RevertURL(infra.orgID, infra.ledgerID, revertOriginID), "", "")
	responses["revert"] = decodeTxResponse(t, revertResp, nethttp.StatusCreated)

	require.Len(t, responses, 7, "all seven v2 ops must be exercised")

	for op, resp := range responses {
		t.Run(op, func(t *testing.T) {
			assert.Containsf(t, resp, "debit", "%s response must carry the debit key", op)
			assert.Containsf(t, resp, "credit", "%s response must carry the credit key", op)
			assert.NotContainsf(t, resp, "source", "%s response must not carry the v1 source key", op)
			assert.NotContainsf(t, resp, "destination", "%s response must not carry the v1 destination key", op)
			assert.Equalf(t, "USD", resp["assetCode"], "%s response assetCode must be preserved", op)
			assert.Containsf(t, resp, "status", "%s response must carry status", op)
			assert.Containsf(t, resp, "operations", "%s response must carry operations", op)
		})
	}

	// Content-equality proof beyond key presence: the direct response's non-renamed fields carry
	// exactly the values the request declared, proving the rename touches ONLY the two leg keys
	// and nothing else in the payload.
	direct := responses["direct"]
	assert.Equal(t, "direct op", direct["description"])
	assert.Equal(t, "USD", direct["assetCode"])
	assert.Equal(t, infra.ledgerID.String(), direct["ledgerId"])
	assert.Equal(t, infra.orgID.String(), direct["organizationId"])
	assert.Equal(t, []any{"@d1src"}, direct["debit"])
	assert.Equal(t, []any{"@d1dst"}, direct["credit"])

	// The immediate create response reflects the funnel's InitialStatus() for a non-pending
	// create (CREATED); the transaction settles to APPROVED afterward, which is what the
	// sibling parity tests assert against the DB post-drain, not against this response.
	status, ok := direct["status"].(map[string]any)
	require.True(t, ok, "status must decode as an object")
	assert.Equal(t, cn.CREATED, status["code"])
}
