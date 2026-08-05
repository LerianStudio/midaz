// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// v2 idempotency is keyed by the v2 body AS SUBMITTED (pre-translation). The v1 create
// funnel hashes the CANONICAL built transaction
// (StructToJSONString(transactionInput)); the v2 surface must instead hash the raw
// flat v2 request bytes so two identical v2 `direct` submissions dedup by the body the
// client actually sent — while the funnel still persists the canonical transaction.
//
// These tests probe the SAME first-repo touch the v1 wiring suite uses
// (TransactionRedisRepo.SetNX, whose internalKey is
// utils.IdempotencyInternalKey(org, ledger, <key>) and — with no caller X-Idempotency —
// <key> falls back to the computed hash). So the SetNX internalKey embeds the hash
// SOURCE by construction, observed without Redis.
//
// Not parallel: buildHumaV2DirectApp / buildHumaTransactionApp mutate process-global
// huma state (see their headers).

const (
	// v2DirectBody is a minimal valid flat v2 `direct` body: amount 100 > 0 clears the
	// funnel's non-positive guard and reaches the idempotency claim; the debit and credit
	// legs name different accounts, so nothing downstream flags the request as ambiguous.
	v2DirectBody = `{"description":"v2 direct","asset":"BRL","amount":"100",` +
		`"debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],` +
		`"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`

	// v1JSONBody is the v1 /json analogue whose CANONICAL built form differs from its raw
	// bytes — so a hash over the canonical transaction can never collide with a hash over
	// these raw bytes.
	v1JSONBody = `{"send":{"asset":"BRL","value":"100","source":{"from":[{"accountAlias":"@src","amount":{"asset":"BRL","value":"100"}}]},"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"BRL","value":"100"}}]}}}`
)

// captureSetNXKey wires a TransactionHandler whose Command is a real use case backed by a
// mocked Redis repo. SetNX captures the internalKey the funnel computed (a losing claim,
// so Get drives the replay short-circuit to 201 before the nil Query is touched). getValue
// is the JSON the losing-claim Get returns (the cached canonical replay).
func captureSetNXKey(t *testing.T, ctrl *gomock.Controller, gotKey *string, getValue string) *TransactionHandler {
	t.Helper()

	redisMock := txRedis.NewMockRedisRepository(ctrl)

	redisMock.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), "", gomock.Any()).
		DoAndReturn(func(_ context.Context, key, _ string, _ time.Duration) (bool, error) {
			*gotKey = key

			return false, nil // losing claim -> Get() -> replay branch
		}).Times(1)

	redisMock.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(getValue, nil).
		Times(1)

	return &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisMock}}
}

// canonicalV1IdempotencyHash reproduces the funnel's v1 hash source EXACTLY: decode the
// raw body into CreateTransactionInput (the same DecodeAndValidate the handler runs),
// BuildTransaction(), apply the default balance keys to both legs (the two
// ApplyDefaultBalanceKeys calls that precede the hash in executeCreateTransaction), then
// StructToJSONString + HashSHA256. If the v1 hash SOURCE ever drifts, this lock breaks.
func canonicalV1IdempotencyHash(t *testing.T, rawBody string) string {
	t.Helper()

	payload := new(mtransaction.CreateTransactionInput)
	_, err := pkgHTTP.DecodeAndValidate([]byte(rawBody), payload)
	require.NoError(t, err, "v1 body must decode for the canonical-hash reconstruction")

	tx := payload.BuildTransaction()
	mtransaction.ApplyDefaultBalanceKeys(tx.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(tx.Send.Distribute.To)

	ts, err := libCommons.StructToJSONString(*tx)
	require.NoError(t, err)

	return libCommons.HashSHA256(ts)
}

// TestHuma_CreateTransactionDirectV2_IdempotencyKeyedByRawV2Body proves the v2 direct
// surface keys idempotency off the RAW v2 body as submitted, not off the
// canonical translated transaction. THE TOOTH: on the header-only v2 path the funnel
// hashes StructToJSONString(canonicalTransaction), so the captured internalKey embeds
// that canonical hash — the raw-body-hash assertion fails until the v2 seam supplies the
// raw bytes as the hash source.
func TestHuma_CreateTransactionDirectV2_IdempotencyKeyedByRawV2Body(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	var gotKey string

	handler := captureSetNXKey(t, ctrl, &gotKey, "{}")
	app := buildHumaV2DirectApp(t, handler)

	req := httptest.NewRequest(http.MethodPost, directV2ConcretePath, strings.NewReader(v2DirectBody))
	req.Header.Set("Content-Type", "application/json")
	// No X-Idempotency header on purpose: the key falls back to the computed hash, so the
	// SetNX internalKey embeds the hash SOURCE.

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	rawBodyHash := libCommons.HashSHA256(v2DirectBody)

	assert.Contains(t, gotKey, rawBodyHash,
		"v2 idempotency must be keyed by the raw v2 body as submitted; got internalKey=%q (a different hash here means v2 still hashes the canonical translated transaction)", gotKey)

	// And it must NOT be the canonical translated-transaction hash the v1 funnel uses.
	// (The v2 flat body translates to a full canonical Transaction whose serialized form
	// differs from the raw bytes, so the two hashes are distinct by construction.)
	payload := new(mtransaction.CreateTransactionV2Input)
	_, derr := pkgHTTP.DecodeAndValidate([]byte(v2DirectBody), payload)
	require.NoError(t, derr)

	canonical, _, terr := payload.Translate(false)
	require.NoError(t, terr)

	mtransaction.ApplyDefaultBalanceKeys(canonical.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(canonical.Send.Distribute.To)

	ts, serr := libCommons.StructToJSONString(canonical)
	require.NoError(t, serr)

	assert.NotContains(t, gotKey, libCommons.HashSHA256(ts),
		"v2 must NOT key idempotency off the canonical translated transaction")
}

// TestHuma_CreateTransactionDirectV2_ReplayReturnsCanonicalResult proves the critical
// edge case: the idempotency record stores the CANONICAL persisted transaction, keyed by
// the v2-body hash. A losing claim whose cached value is a canonical transaction replays
// that canonical result (201 + X-Idempotency-Replayed:true) without creating a new one.
func TestHuma_CreateTransactionDirectV2_ReplayReturnsCanonicalResult(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	var gotKey string

	// The cached value is the CANONICAL transaction (a full transaction.Transaction),
	// NOT the raw v2 body — proving hashed-source (v2 body) and persisted-result
	// (canonical) are decoupled.
	canonicalID := uuid.New().String()
	handler := captureSetNXKey(t, ctrl, &gotKey, `{"id":"`+canonicalID+`"}`)
	app := buildHumaV2DirectApp(t, handler)

	req := httptest.NewRequest(http.MethodPost, directV2ConcretePath, strings.NewReader(v2DirectBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	assert.Contains(t, gotKey, libCommons.HashSHA256(v2DirectBody),
		"replay claim must be keyed by the raw v2 body")
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "v2 replay returns 201, body: %s", string(body))
	assert.Equal(t, "true", resp.Header.Get(libConstants.IdempotencyReplayed),
		"a losing v2 claim with a cached canonical value replays -> X-Idempotency-Replayed:true")
	assert.Contains(t, string(body), canonicalID,
		"the replay returns the CANONICAL persisted transaction (keyed by the v2-body hash), not the raw v2 body")
}

// TestHuma_CreateTransactionV1JSON_IdempotencyStillKeyedByCanonicalBody is the v1
// regression lock: the v1 /json funnel must remain byte-identical — hashing the canonical
// built transaction, NEVER the raw request bytes. Guards against the additive v2 seam
// leaking onto the v1 hash source.
func TestHuma_CreateTransactionV1JSON_IdempotencyStillKeyedByCanonicalBody(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.New()
	ledgerID := uuid.New()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	var gotKey string

	handler := captureSetNXKey(t, ctrl, &gotKey, "{}")
	app := buildHumaTransactionApp(t, handler, true)

	url := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/json"
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(v1JSONBody))
	req.Header.Set("Content-Type", "application/json")
	// No X-Idempotency header: v1 key falls back to the canonical-transaction hash.

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Contains(t, gotKey, canonicalV1IdempotencyHash(t, v1JSONBody),
		"v1 idempotency must stay keyed by the canonical built transaction (byte-identical)")
	assert.NotContains(t, gotKey, libCommons.HashSHA256(v1JSONBody),
		"v1 must NOT key idempotency off the raw request bytes; the additive v2 seam must not leak onto v1")
}
