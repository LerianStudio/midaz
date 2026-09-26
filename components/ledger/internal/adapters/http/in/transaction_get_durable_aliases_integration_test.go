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

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

// A GET by id answers from one of two places: the engine view while the
// write-behind index says the execution is still pending persistence, and the
// PostgreSQL primary once the index is durable or gone. Clients read "who paid
// whom" from the leg alias lists, so both sources must answer the same
// debit/credit (v2) and source/destination (v1) the create answered. Every read
// here asserts X-Cache-Hit, because a read served by the engine view would pass
// the value assertions without ever reaching the primary.
//
// NOT parallel: the Huma test builders install process-global hooks.
func TestIntegration_TransactionGetByID_LegAliasesMatchCreate(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	v1App := buildHumaTransactionApp(t, fixture.infra.handler, true)
	v2App := buildHumaV2MirrorApp(t, fixture.infra.handler)
	repository, ok := fixture.infra.redisRepo.(*redistransaction.RedisConsumerRepository)
	require.True(t, ok, "Redis repository must expose the engine write-behind index")

	t.Run("durable_then_absent_index_reads_from_primary", func(t *testing.T) {
		ctx := context.Background()
		organizationID := fixture.infra.orgID
		ledgerID := fixture.newLedger(t)
		seedTransfer(t, fixture.infra.pgContainer.DB, organizationID, ledgerID, "@src", "@dst", 100)

		transactionID := createDirectTransfer(t, fixture, ledgerID, "durable-aliases")

		pending := engineIndexPending(t, fixture, repository, ledgerID, transactionID)
		require.False(t, pending, "the acknowledged create must leave the engine index durable; "+
			"a pending index is served by the engine view and cannot prove the primary read")

		v2URL := v2TxByIDURL(organizationID, ledgerID, transactionID)
		v1URL := v1TxByIDURL(organizationID, ledgerID, transactionID)

		v2Read := readTransaction(t, v2App, v2URL, "false")
		assertLegAliases(t, v2Read, "debit", "credit", "v2 GET, durable index")
		assert.Len(t, v2OperationsOf(t, v2Read, "v2 GET, durable index"), 2, "the primary read must carry both operations")

		v1Read := readTransaction(t, v1App, v1URL, "false")
		assertLegAliases(t, v1Read, "source", "destination", "v1 GET, durable index")

		patched := decodeTxResponse(t, patchV2(t, v2App, v2URL, `{"description":"edited after persistence"}`), nethttp.StatusOK)
		require.Equal(t, "edited after persistence", patched["description"])

		v2AfterPatch := readTransaction(t, v2App, v2URL, "false")
		assert.Equal(t, "edited after persistence", v2AfterPatch["description"], "the GET must answer the edit, not the frozen engine view")
		assertLegAliases(t, v2AfterPatch, "debit", "credit", "v2 GET after PATCH")

		indexKey := "engine:" + cachepolicy.HashTag + ":transaction-index:" + organizationID.String() + ":" + ledgerID.String()
		removed, err := fixture.infra.redisContainer.Client.HDel(ctx, indexKey, transactionID.String()).Result()
		require.NoError(t, err)
		require.Equal(t, int64(1), removed, "the transaction's index entry must exist under %s before its removal", indexKey)
		_, err = repository.GetEngineTransactionIndex(ctx, organizationID, ledgerID, transactionID)
		require.ErrorIs(t, err, redistransaction.ErrEngineWriteBehindNotFound, "the index entry must be absent")

		v2Absent := readTransaction(t, v2App, v2URL, "false")
		assertLegAliases(t, v2Absent, "debit", "credit", "v2 GET, absent index")

		v1Absent := readTransaction(t, v1App, v1URL, "false")
		assertLegAliases(t, v1Absent, "source", "destination", "v1 GET, absent index")
	})

	t.Run("pending_index_reads_from_engine_view", func(t *testing.T) {
		acknowledger := fixture.infra.handler.Command.EngineRecoveryAcknowledger
		fixture.infra.handler.Command.EngineRecoveryAcknowledger = nil
		t.Cleanup(func() { fixture.infra.handler.Command.EngineRecoveryAcknowledger = acknowledger })

		organizationID := fixture.infra.orgID
		ledgerID := fixture.newLedger(t)
		seedTransfer(t, fixture.infra.pgContainer.DB, organizationID, ledgerID, "@src", "@dst", 100)

		transactionID := createDirectTransfer(t, fixture, ledgerID, "pending-aliases")

		pending := engineIndexPending(t, fixture, repository, ledgerID, transactionID)
		require.True(t, pending, "an unacknowledged create must leave the engine index pending")

		v2Read := readTransaction(t, v2App, v2TxByIDURL(organizationID, ledgerID, transactionID), "true")
		assertLegAliases(t, v2Read, "debit", "credit", "v2 GET, pending index")
	})
}

// createDirectTransfer posts a 100 USD @src -> @dst direct create and asserts
// the leg aliases it answers, the anchor every later read is compared with.
func createDirectTransfer(t *testing.T, fixture *atomicBatchHTTPIntegrationFixture, ledgerID uuid.UUID, idempotencyKey string) uuid.UUID {
	t.Helper()

	raw, err := json.Marshal(atomicBatchTransfer(fixture.infra.orgID, ledgerID, "leg aliases on read", "@src", "@dst", 100))
	require.NoError(t, err)

	created := decodeTxResponse(t, postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), idempotencyKey), nethttp.StatusCreated)
	assertLegAliases(t, created, "debit", "credit", "v2 direct create")

	return uuid.MustParse(created["id"].(string))
}

// engineIndexPending decodes the transaction's engine index with the codec the
// read path uses, reporting whether the execution is still pending persistence.
func engineIndexPending(
	t *testing.T,
	fixture *atomicBatchHTTPIntegrationFixture,
	repository *redistransaction.RedisConsumerRepository,
	ledgerID, transactionID uuid.UUID,
) bool {
	t.Helper()

	ctx := context.Background()
	raw, err := repository.GetEngineTransactionIndex(ctx, fixture.infra.orgID, ledgerID, transactionID)
	require.NoError(t, err, "the create must index the transaction")

	_, pending, err := fixture.infra.handler.Query.EngineWriteBehindCodec.DecodeEngineTransactionIndex(
		ctx, raw, fixture.infra.orgID, ledgerID, transactionID,
	)
	require.NoError(t, err)

	return pending
}

// readTransaction GETs a transaction by id, asserting 200 and which source
// answered it through X-Cache-Hit: "true" is the engine view, "false" the primary.
func readTransaction(t *testing.T, app *fiber.App, url, wantCacheHit string) map[string]any {
	t.Helper()

	response := getV2(t, app, url)
	body := decodeTxResponse(t, response, nethttp.StatusOK)
	require.Equal(t, wantCacheHit, response.Header.Get("X-Cache-Hit"), "GET %s answered from an unexpected source", url)

	return body
}

// assertLegAliases asserts the leg alias lists by value: a key present with a
// null value is exactly the regression this guards.
func assertLegAliases(t *testing.T, tx map[string]any, debitKey, creditKey, label string) {
	t.Helper()

	assert.Equalf(t, []any{"@src"}, tx[debitKey], "%s: %s", label, debitKey)
	assert.Equalf(t, []any{"@dst"}, tx[creditKey], "%s: %s", label, creditKey)
}

func v1TxByIDURL(organizationID, ledgerID, transactionID uuid.UUID) string {
	return "/v1/organizations/" + organizationID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + transactionID.String()
}
