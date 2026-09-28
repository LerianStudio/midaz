// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libConstants "github.com/LerianStudio/lib-commons/v7/commons/constants"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// Revert replay visibility. A revert carries no caller idempotency key, so its slot is derived
// inside the create core from the serialized reversal payload. When that slot already holds a
// serialized reverse the core returns the FIRST reverse instead of creating a second one — a
// 201 that is economically different from a fresh revert, and (because the payload carries no
// origin reference — KNOWN DEFECT) possibly not even a reverse of the
// origin the caller named.
//
// There is ONE live revert terminal, RevertTransaction: both the v1 and v2 routes mount it
// (the Fiber wrapper RevertTransaction has no production registration). It must project the
// core's `replayed` flag onto X-Idempotency-Replayed — as the typed output field AND as a
// header a client can actually read — and the core must record a Warn so a replayed revert is
// visible to operators as well as to the caller.

// logRecord is one captured call to the GoMock libLog.Logger.
type logRecord struct {
	level  int
	msg    string
	fields []libLog.Field
}

// logRecorder accumulates the Log calls made through the mock logger. Reads go through
// snapshot() so the accessor takes the same mutex the writes do — the exercised path may log
// from a goroutine other than the test's.
type logRecorder struct {
	mu      sync.Mutex
	records []logRecord
}

func (r *logRecorder) add(rec logRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.records = append(r.records, rec)
}

// snapshot returns a copy of the records captured so far.
func (r *logRecorder) snapshot() []logRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.records)
}

// recordingLogger returns the lib-observability GoMock logger wired to append every Log call
// into the returned recorder. Recording rather than EXPECTing one specific call keeps the
// level/message/field contract assertable in the test body and stays immune to whichever other
// lines the exercised path emits.
func recordingLogger(t *testing.T, ctrl *gomock.Controller) (*libLog.MockLogger, *logRecorder) {
	t.Helper()

	recorder := &logRecorder{}
	logger := libLog.NewMockLogger(ctrl)

	// The closure is invoked by gomock through reflection, so its parameter types must be the
	// EXACT types of the mocked method: since lib-observability v4 those are the universal
	// (int, ...any), not (libLog.Level, ...libLog.Field). A stale signature still compiles and
	// panics only at run time. libLog.Fields re-types the variadic for the assertions below.
	capture := func(_ context.Context, level int, msg string, fields ...any) {
		recorder.add(logRecord{level: level, msg: msg, fields: libLog.Fields(fields...)})
	}

	// FOUR matchers for a three-arg-plus-variadic method, deliberately: with exactly
	// methodType.NumIn() matchers gomock accepts any number of variadic args (including none),
	// so this one expectation covers both the field-less lines the exercised path emits (the
	// idempotency cache-hit Debug) and the lines carrying fields. A three-matcher list would
	// match ONLY the field-less calls.
	logger.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(capture).AnyTimes()
	logger.EXPECT().With(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithGroup(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Enabled(gomock.Any()).Return(true).AnyTimes()
	logger.EXPECT().Sync(gomock.Any()).Return(nil).AnyTimes()

	return logger, recorder
}

// revertReplaySubjects are the fixed ids the replayed-revert arrangement is built around.
type revertReplaySubjects struct {
	orgID     uuid.UUID
	ledgerID  uuid.UUID
	originID  uuid.UUID
	reverseID uuid.UUID
}

// arrangeReplayedRevert builds a handler whose revert eligibility gate passes and whose
// idempotency slot is ALREADY populated with a serialized reverse transaction, so the create
// core short-circuits into its replay branch and returns (cachedReverse, replayed=true) without
// touching balances. The Redis expectations name the exact slot, so they also pin the derivation
// the revert path uses today: no caller key, no override, hence HashSHA256 over the canonical
// serialized reversal payload, at the pre-migration Fiber TTL.
func arrangeReplayedRevert(t *testing.T, ctrl *gomock.Controller) (*TransactionHandler, revertReplaySubjects) {
	t.Helper()

	subjects := revertReplaySubjects{
		orgID:     uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb501"),
		ledgerID:  uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb502"),
		originID:  uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb503"),
		reverseID: uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb504"),
	}

	amount := decimal.NewFromInt(1000)

	origin := &transaction.Transaction{
		ID:                  subjects.originID.String(),
		OrganizationID:      subjects.orgID.String(),
		LedgerID:            subjects.ledgerID.String(),
		ParentTransactionID: nil,
		Description:         "replayed revert subject",
		AssetCode:           "USD",
		Amount:              &amount,
		Status:              transaction.Status{Code: cn.APPROVED},
		Operations: []*operation.Operation{
			{
				Type:         cn.CREDIT,
				AccountAlias: "@receiver",
				AssetCode:    "USD",
				Amount:       operation.Amount{Value: &amount},
			},
		},
	}

	originIDStr := subjects.originID.String()
	cachedReverse := &transaction.Transaction{
		ID:                  subjects.reverseID.String(),
		OrganizationID:      subjects.orgID.String(),
		LedgerID:            subjects.ledgerID.String(),
		ParentTransactionID: &originIDStr,
		Description:         "replayed revert subject",
		AssetCode:           "USD",
		Amount:              &amount,
		Status:              transaction.Status{Code: cn.APPROVED},
	}

	cachedValue, err := json.Marshal(cachedReverse)
	require.NoError(t, err, "serialize the cached reverse transaction")

	transactionRepo := transaction.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	transactionRepo.EXPECT().
		FindByParentID(gomock.Any(), subjects.orgID, subjects.ledgerID, subjects.originID).
		Return(nil, nil).
		Times(1)

	transactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), subjects.orgID, subjects.ledgerID, subjects.originID).
		Return(origin, nil).
		Times(1)

	metadataRepo.EXPECT().
		FindByEntity(gomock.Any(), cn.EntityTransaction, subjects.originID.String()).
		Return(nil, nil).
		Times(1)

	// The claimed slot, derived the way the revert path derives it: the reversal payload the
	// eligibility gate hands to createRevertTransaction, canonically serialized after the same
	// ApplyDefaultBalanceKeys normalization executeCreateTransaction applies before hashing.
	// Spelled out here rather than reusing a production helper because there is no revert-side
	// helper to reuse — the origin plays no part in this key, which is the known origin-scoping
	// defect documented on revertTransaction.
	reversal := origin.TransactionRevert()
	mtransaction.ApplyDefaultBalanceKeys(reversal.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(reversal.Send.Distribute.To)

	hashSource, err := libCommons.StructToJSONString(reversal)
	require.NoError(t, err, "serialize the reversal payload the idempotency hash is computed over")

	internalKey := utils.IdempotencyInternalKey(subjects.orgID, subjects.ledgerID,
		libCommons.HashSHA256(hashSource))

	redisRepo.EXPECT().
		SetNX(gomock.Any(), internalKey, "", pkgHTTP.ParseIdempotencyTTL("")).
		Return(false, nil).
		Times(1)

	redisRepo.EXPECT().
		Get(gomock.Any(), internalKey).
		Return(string(cachedValue), nil).
		Times(1)

	queryUC := &query.UseCase{
		TransactionRepo:         transactionRepo,
		TransactionMetadataRepo: metadataRepo,
	}

	handler := &TransactionHandler{
		Query: queryUC,
		Command: &command.UseCase{
			TransactionRepo:      transactionRepo,
			TransactionRedisRepo: redisRepo,
			TransactionReader:    queryUC,
		},
	}

	return handler, subjects
}

// TestRevertTransaction_ReplayedIdempotency_SurfacesOnTheHumaShell proves the live revert
// terminal reports a replay on its typed output: the replayed flag is projected, the FIRST
// reverse is returned, and the core records a Warn naming the origin so the replay is visible
// to operators too.
func TestRevertTransaction_ReplayedIdempotency_SurfacesOnTheHumaShell(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	logger, recorder := recordingLogger(t, ctrl)
	handler, subjects := arrangeReplayedRevert(t, ctrl)

	out, err := handler.RevertTransaction(
		libObservability.ContextWithLogger(context.Background(), logger),
		&StateTransactionRequest{
			OrganizationID: subjects.orgID.String(),
			LedgerID:       subjects.ledgerID.String(),
			TransactionID:  subjects.originID.String(),
		},
	)
	require.NoError(t, err, "the Huma revert shell should return the replayed transaction")
	require.NotNil(t, out, "the Huma revert shell must return an output envelope")
	require.NotNil(t, out.Body, "the Huma revert shell must return the replayed transaction")
	require.Equal(t, nethttp.StatusCreated, out.Status, "a replayed revert still answers 201")

	assert.Equal(t, "true", out.IdempotencyReplayed,
		"a replayed revert must advertise X-Idempotency-Replayed: true — otherwise a replay is indistinguishable from a fresh revert")
	assert.Equal(t, subjects.reverseID.String(), out.Body.ID,
		"a replayed revert must return the FIRST reverse transaction")

	warn := findLogRecord(recorder.snapshot(), libLog.LevelWarn, command.RevertIdempotencyReplayedLogMessage)
	require.NotNil(t, warn,
		"a replayed revert must record a Warn (%q) so the replay is visible to operators, not only to the caller",
		command.RevertIdempotencyReplayedLogMessage)
	assert.Equal(t, []libLog.Field{libLog.String("transaction_id", subjects.originID.String())}, warn.fields,
		"the replay Warn must name the origin transaction id and carry nothing else (no balances, amounts or payloads)")
}

// TestRevertTransaction_ReplayedIdempotency_SurfacesOnTheWire closes the gap the typed-output
// test cannot: it drives a replayed revert through the real route and asserts the header on the
// RESPONSE, so the `header:"X-Idempotency-Replayed"` tag on the output envelope is proven to
// serialize. Without this, `replayed=true` would be pinned only at the struct field — invisible
// to the client the flag exists for. (The `false` side of the same header is covered on the wire
// by the V2 revert integration tests.)
func TestRevertTransaction_ReplayedIdempotency_SurfacesOnTheWire(t *testing.T) {
	// NOT parallel: process-global huma state (see buildHumaV2DirectApp).
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	logger, recorder := recordingLogger(t, ctrl)
	handler, subjects := arrangeReplayedRevert(t, ctrl)

	// The logger is injected so the Warn assertion below reads what the WIRED path logged.
	app := buildHumaV2DirectApp(t, handler, logger)

	url := "/v2/organizations/" + subjects.orgID.String() +
		"/ledgers/" + subjects.ledgerID.String() +
		"/transactions/" + subjects.originID.String() + "/revert"

	req := httptest.NewRequest(nethttp.MethodPost, url, nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err, "the v2 revert request should not fail at the transport layer")

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read the v2 revert response body")
	require.Equal(t, nethttp.StatusCreated, resp.StatusCode, "a replayed revert still answers 201; body: %s", string(raw))

	assert.Equal(t, "true", resp.Header.Get(libConstants.IdempotencyReplayed),
		"a replayed revert must advertise X-Idempotency-Replayed: true ON THE WIRE; body: %s", string(raw))

	var body struct {
		ID string `json:"id"`
	}

	require.NoError(t, json.Unmarshal(raw, &body), "the v2 revert response should be valid JSON; body: %s", string(raw))
	assert.Equal(t, subjects.reverseID.String(), body.ID, "a replayed revert must return the FIRST reverse transaction")

	warn := findLogRecord(recorder.snapshot(), libLog.LevelWarn, command.RevertIdempotencyReplayedLogMessage)
	assert.NotNil(t, warn, "the replay Warn must also fire on the wired path")
}

// findLogRecord returns the first captured record with the given level and message, or nil.
func findLogRecord(records []logRecord, level int, msg string) *logRecord {
	for i := range records {
		if records[i].level == level && records[i].msg == msg {
			return &records[i]
		}
	}

	return nil
}
