// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

// This file proves, over the real Redis and PostgreSQL behind the production Huma
// seams, that an idempotency slot replays only the request that created it. A
// request reusing the key with another action, another body, another /v1 mode or
// the other API version answers 409 0084 and posts nothing; a retry of the same
// request — even re-serialized — still replays; a slot written before the slot
// carried a request fingerprint keeps replaying whatever reuses its key.

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// fingerprintV2Body is a flat v2 body every singular v2 action accepts; the reason
// metadata is what block and unblock require.
const fingerprintV2Body = `{"description":"reused key","asset":"USD","amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}],"metadata":{"reason":"idempotency-check"}}`

// fingerprintV1Body is a /v1 body the json, annotation, block and unblock modes all
// accept unchanged.
const fingerprintV1Body = `{"description":"reused key","metadata":{"reason":"idempotency-check"},"send":{"asset":"USD","value":"100","source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]},"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}}}`

type fingerprintFixture struct {
	infra *testInfra
	v1App *fiber.App
	v2App *fiber.App
}

func setupFingerprintFixture(t *testing.T) *fingerprintFixture {
	t.Helper()

	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 10000)

	return &fingerprintFixture{
		infra: infra,
		v1App: buildHumaTransactionApp(t, infra.handler, true),
		v2App: buildHumaV2DirectApp(t, infra.handler),
	}
}

func (f *fingerprintFixture) v1URL(mode string) string {
	return "/v1/organizations/" + f.infra.orgID.String() + "/ledgers/" + f.infra.ledgerID.String() + "/transactions/" + mode
}

func (f *fingerprintFixture) postV2(t *testing.T, action, body, key string) *nethttp.Response {
	t.Helper()

	return postV2Create(t, f.v2App, action, f.infra.orgID, f.infra.ledgerID, body, key)
}

func (f *fingerprintFixture) postV1(t *testing.T, mode, body, key string) *nethttp.Response {
	t.Helper()

	return postTransaction(t, f.v1App, f.v1URL(mode), body, key)
}

// createAndSettle posts the first request under key and waits until its outcome is
// stored in the slot, so a later conflict comes from the stored fingerprint and not
// from the in-flight placeholder, which answers 0084 for every request.
func (f *fingerprintFixture) createAndSettle(t *testing.T, resp *nethttp.Response, key string) string {
	t.Helper()

	result := decodeTxResponse(t, resp, nethttp.StatusCreated)
	require.Equal(t, "false", resp.Header.Get("X-Idempotency-Replayed"), "the first request must not be a replay")

	waitForIdempotencyStored(t, context.Background(), f.infra.redisRepo, f.infra.orgID, f.infra.ledgerID, key)

	return result["id"].(string)
}

func (f *fingerprintFixture) transactionCount(t *testing.T) int {
	t.Helper()

	return countTransactionsInLedger(t, f.infra.pgContainer.DB, f.infra.ledgerID)
}

func requireIdempotencyConflict(t *testing.T, resp *nethttp.Response) {
	t.Helper()

	body := drainBody(t, resp)
	require.Equal(t, nethttp.StatusConflict, resp.StatusCode, "a reused key with another request must conflict; body: %s", string(body))
	requireProblemCode(t, body, "0084")
}

func requireReplayOf(t *testing.T, resp *nethttp.Response, transactionID string) {
	t.Helper()

	result := decodeTxResponse(t, resp, nethttp.StatusCreated)
	assert.Equal(t, "true", resp.Header.Get("X-Idempotency-Replayed"), "a retry of the same request must be a replay")
	assert.Equal(t, transactionID, result["id"], "the replay must answer the transaction the slot holds")
}

func TestIntegration_TransactionV2_ReusedKeyOnAnotherActionConflicts(t *testing.T) {
	// NOT parallel: process-global huma state (see transaction_handler_v2_test.go).
	f := setupFingerprintFixture(t)

	cases := []struct {
		first  string
		reused []string
	}{
		{first: "direct", reused: []string{"hold", "block", "unblock"}},
		{first: "hold", reused: []string{"direct"}},
		{first: "block", reused: []string{"unblock"}},
	}

	for _, tc := range cases {
		key := "reused-action-" + uuid.NewString()
		firstID := f.createAndSettle(t, f.postV2(t, tc.first, fingerprintV2Body, key), key)
		countAfterFirst := f.transactionCount(t)

		for _, action := range tc.reused {
			requireIdempotencyConflict(t, f.postV2(t, action, fingerprintV2Body, key))
			assert.Equal(t, countAfterFirst, f.transactionCount(t), "%s reusing the %s key must post nothing", action, tc.first)
		}

		// The conflicts neither released nor overwrote the slot.
		requireReplayOf(t, f.postV2(t, tc.first, fingerprintV2Body, key), firstID)
	}
}

func TestIntegration_TransactionV2_ReusedKeyReplaysOnlyTheSameBody(t *testing.T) {
	// NOT parallel: process-global huma state (see transaction_handler_v2_test.go).
	f := setupFingerprintFixture(t)

	key := "reused-body-" + uuid.NewString()
	firstID := f.createAndSettle(t, f.postV2(t, "direct", fingerprintV2Body, key), key)
	countAfterFirst := f.transactionCount(t)

	changed := strings.Replace(fingerprintV2Body, `"description":"reused key"`, `"description":"another transfer"`, 1)
	requireIdempotencyConflict(t, f.postV2(t, "direct", changed, key))

	reserialized := `{
		"metadata": {"reason": "idempotency-check"},
		"credits": [{"amount": "100", ` + v2ScopeJSON + `, "alias": "@dst"}],
		"debits": [{"amount": "100", ` + v2ScopeJSON + `, "alias": "@src"}],
		"amount": "100",
		"asset": "USD",
		"description": "reused key"
	}`
	requireReplayOf(t, f.postV2(t, "direct", reserialized, key), firstID)
	requireReplayOf(t, f.postV2(t, "direct", fingerprintV2Body, key), firstID)

	assert.Equal(t, countAfterFirst, f.transactionCount(t), "neither the conflict nor the replays may post")
}

func TestIntegration_TransactionV1_ReusedKeyOnAnotherModeConflicts(t *testing.T) {
	// NOT parallel: process-global huma state (see transaction_handler_test.go).
	f := setupFingerprintFixture(t)

	key := "reused-mode-" + uuid.NewString()
	firstID := f.createAndSettle(t, f.postV1(t, "json", fingerprintV1Body, key), key)
	countAfterFirst := f.transactionCount(t)

	for _, mode := range []string{"annotation", "block", "unblock"} {
		requireIdempotencyConflict(t, f.postV1(t, mode, fingerprintV1Body, key))
	}

	inflowBody := `{"description":"reused key","send":{"asset":"USD","value":"100","distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}}}`
	requireIdempotencyConflict(t, f.postV1(t, "inflow", inflowBody, key))

	outflowBody := `{"description":"reused key","send":{"asset":"USD","value":"100","source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]}}}`
	requireIdempotencyConflict(t, f.postV1(t, "outflow", outflowBody, key))

	reordered := `{"send":{"distribute":{"to":[{"amount":{"value":"100","asset":"USD"},"accountAlias":"@dst"}]},"source":{"from":[{"amount":{"value":"100","asset":"USD"},"accountAlias":"@src"}]},"value":"100","asset":"USD"},"metadata":{"reason":"idempotency-check"},"description":"reused key"}`
	requireReplayOf(t, f.postV1(t, "json", reordered, key), firstID)

	assert.Equal(t, countAfterFirst, f.transactionCount(t), "neither the conflicts nor the replay may post")
}

func TestIntegration_Transaction_ReusedKeyAcrossVersionsConflicts(t *testing.T) {
	// NOT parallel: process-global huma state (see transaction_handler_v2_test.go).
	f := setupFingerprintFixture(t)

	v1First := "v1-then-v2-" + uuid.NewString()
	f.createAndSettle(t, f.postV1(t, "json", fingerprintV1Body, v1First), v1First)
	requireIdempotencyConflict(t, f.postV2(t, "direct", fingerprintV2Body, v1First))

	v2First := "v2-then-v1-" + uuid.NewString()
	f.createAndSettle(t, f.postV2(t, "direct", fingerprintV2Body, v2First), v2First)
	requireIdempotencyConflict(t, f.postV1(t, "json", fingerprintV1Body, v2First))

	assert.Equal(t, 2, f.transactionCount(t), "only the two first requests post")
}

func TestIntegration_Transaction_SlotWithoutFingerprintKeepsReplaying(t *testing.T) {
	// NOT parallel: process-global huma state (see transaction_handler_v2_test.go).
	f := setupFingerprintFixture(t)
	ctx := context.Background()

	// A slot as pods that predate the fingerprint wrote it: a real transaction's
	// stored value with the fingerprint field removed.
	sourceKey := "fingerprinted-" + uuid.NewString()
	storedID := f.createAndSettle(t, f.postV2(t, "direct", fingerprintV2Body, sourceKey), sourceKey)

	stored, err := f.infra.redisRepo.Get(ctx, utils.IdempotencyInternalKey(f.infra.orgID, f.infra.ledgerID, sourceKey))
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stored), &fields))
	require.Contains(t, fields, "idempotencyFingerprint", "the new writer must store the fingerprint")
	delete(fields, "idempotencyFingerprint")

	legacyValue, err := json.Marshal(fields)
	require.NoError(t, err)

	legacyKey := "legacy-" + uuid.NewString()
	require.NoError(t, f.infra.redisRepo.Set(ctx, utils.IdempotencyInternalKey(f.infra.orgID, f.infra.ledgerID, legacyKey), string(legacyValue), time.Minute))

	countBefore := f.transactionCount(t)

	changed := strings.Replace(fingerprintV2Body, `"description":"reused key"`, `"description":"another transfer"`, 1)
	requireReplayOf(t, f.postV2(t, "hold", changed, legacyKey), storedID)

	assert.Equal(t, countBefore, f.transactionCount(t), "a legacy slot replays and posts nothing")
}

func TestIntegration_TransactionV1_NoKeyModeSwitchWithSameBodyConflicts(t *testing.T) {
	// NOT parallel: process-global huma state (see transaction_handler_test.go).
	f := setupFingerprintFixture(t)

	first := decodeTxResponse(t, f.postV1(t, "json", fingerprintV1Body, ""), nethttp.StatusCreated)
	firstID := first["id"].(string)

	// Without a key the slot is named by the body hash, which the test does not
	// recompute: a replay of the same no-key request proves the outcome is stored.
	var replayed bool

	for i := 0; i < 400 && !replayed; i++ {
		resp := f.postV1(t, "json", fingerprintV1Body, "")
		body := drainBody(t, resp)

		if resp.StatusCode == nethttp.StatusCreated {
			require.Equal(t, "true", resp.Header.Get("X-Idempotency-Replayed"), "a no-key retry of the same request must replay; body: %s", string(body))
			assert.Contains(t, string(body), firstID)

			replayed = true

			break
		}

		require.Equal(t, nethttp.StatusConflict, resp.StatusCode, "only the in-flight placeholder may answer before the replay; body: %s", string(body))
		time.Sleep(25 * time.Millisecond)
	}

	require.True(t, replayed, "the first no-key outcome was not stored within the retry budget")

	requireIdempotencyConflict(t, f.postV1(t, "annotation", fingerprintV1Body, ""))
	assert.Equal(t, 1, f.transactionCount(t), "the annotation reusing the json slot must post nothing")
}

func TestIntegration_Transaction_RetryOfTheSameRequestReplaysOnEveryMode(t *testing.T) {
	// NOT parallel: process-global huma state (see transaction_handler_v2_test.go).
	f := setupFingerprintFixture(t)

	// Inflow and outflow post against the ledger's external USD account.
	seedExternalFundingBalances(t, f.infra.pgContainer.DB, f.infra.orgID, f.infra.ledgerID)

	inflowBody := `{"description":"retried inflow","send":{"asset":"USD","value":"100","distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}}}`
	outflowBody := `{"description":"retried outflow","send":{"asset":"USD","value":"100","source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]}}}`

	// Each mode derives its fingerprint on its own path, so each must reproduce it
	// on the retry: a non-deterministic input there would answer 0084 to an honest
	// retry.
	cases := []struct {
		name string
		post func(t *testing.T, key string) *nethttp.Response
	}{
		{name: "v1 json", post: func(t *testing.T, key string) *nethttp.Response { return f.postV1(t, "json", fingerprintV1Body, key) }},
		{name: "v1 annotation", post: func(t *testing.T, key string) *nethttp.Response {
			return f.postV1(t, "annotation", fingerprintV1Body, key)
		}},
		{name: "v1 block", post: func(t *testing.T, key string) *nethttp.Response { return f.postV1(t, "block", fingerprintV1Body, key) }},
		{name: "v1 unblock", post: func(t *testing.T, key string) *nethttp.Response {
			return f.postV1(t, "unblock", fingerprintV1Body, key)
		}},
		{name: "v1 inflow", post: func(t *testing.T, key string) *nethttp.Response { return f.postV1(t, "inflow", inflowBody, key) }},
		{name: "v1 outflow", post: func(t *testing.T, key string) *nethttp.Response { return f.postV1(t, "outflow", outflowBody, key) }},
		{name: "v2 direct", post: func(t *testing.T, key string) *nethttp.Response { return f.postV2(t, "direct", fingerprintV2Body, key) }},
		{name: "v2 hold", post: func(t *testing.T, key string) *nethttp.Response { return f.postV2(t, "hold", fingerprintV2Body, key) }},
		{name: "v2 block", post: func(t *testing.T, key string) *nethttp.Response { return f.postV2(t, "block", fingerprintV2Body, key) }},
		{name: "v2 unblock", post: func(t *testing.T, key string) *nethttp.Response {
			return f.postV2(t, "unblock", fingerprintV2Body, key)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := "retry-" + uuid.NewString()
			firstID := f.createAndSettle(t, tc.post(t, key), key)
			countAfterFirst := f.transactionCount(t)

			requireReplayOf(t, tc.post(t, key), firstID)
			assert.Equal(t, countAfterFirst, f.transactionCount(t), "the retry must not post")
		})
	}
}
