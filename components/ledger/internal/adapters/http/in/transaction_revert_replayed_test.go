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

	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// Revert replay visibility. A completed durable claim resolves the exact reverse
// from PostgreSQL primary and returns it without another balance mutation.
//
// There is ONE live revert terminal, RevertTransactionHuma: both the v1 and v2 routes mount it
// (the Fiber wrapper RevertTransaction has no production registration). It must project the
// core's `replayed` flag onto X-Idempotency-Replayed — as the typed output field AND as a
// header a client can actually read — and the core must record a Warn so a replayed revert is
// visible to operators as well as to the caller.

// logRecord is one captured call to the GoMock libLog.Logger.
type logRecord struct {
	level  libLog.Level
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

	capture := func(_ context.Context, level libLog.Level, msg string, fields ...libLog.Field) {
		recorder.add(logRecord{level: level, msg: msg, fields: fields})
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

// arrangeReplayedRevert builds a handler whose PostgreSQL primary already has
// the persisted child and durable exact-ID claim. The core returns that reverse
// as replayed without entering the Redis or balance paths.
func arrangeReplayedRevert(t *testing.T, ctrl *gomock.Controller) (*TransactionHandler, revertReplaySubjects) {
	t.Helper()

	subjects := revertReplaySubjects{
		orgID:     uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb501"),
		ledgerID:  uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb502"),
		originID:  uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb503"),
		reverseID: uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb504"),
	}

	amount := decimal.NewFromInt(1000)

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
		Operations:          []*operation.Operation{{ID: uuid.NewString()}},
	}

	transactionRepo := transaction.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)

	transactionRepo.EXPECT().
		FindByParentID(gomock.Any(), subjects.orgID, subjects.ledgerID, subjects.originID).
		Return(cachedReverse, nil).
		Times(1)
	transactionRepo.EXPECT().
		FindWithOperations(gomock.Any(), subjects.orgID, subjects.ledgerID, subjects.reverseID).
		Return(cachedReverse, nil).
		Times(1)

	metadataRepo.EXPECT().
		FindByEntity(gomock.Any(), cn.EntityTransaction, subjects.reverseID.String()).
		Return(nil, nil).
		Times(2)

	claim := &revertclaim.Claim{
		OrganizationID:       subjects.orgID,
		LedgerID:             subjects.ledgerID,
		OriginTransactionID:  subjects.originID,
		ReverseTransactionID: subjects.reverseID,
		State:                revertclaim.StateCompleted,
	}
	claimRepo.EXPECT().
		Claim(gomock.Any(), subjects.orgID, subjects.ledgerID, subjects.originID, subjects.reverseID, nil, nil).
		Return(claim, false, nil).
		Times(1)
	claimRepo.EXPECT().
		Transition(gomock.Any(), subjects.orgID, subjects.ledgerID, subjects.originID, subjects.reverseID,
			revertclaim.StateCompleted, nil).
		Return(nil).
		Times(1)
	redisRepo.EXPECT().
		CompleteOwnedKey(gomock.Any(), originRevertIdempotencyKey(claim), subjects.reverseID.String(),
			gomock.Any(), gomock.Any()).
		Return(true, nil).
		Times(1)
	redisRepo.EXPECT().
		ReadMessageFromQueue(gomock.Any(),
			utils.TransactionInternalKey(subjects.orgID, subjects.ledgerID, subjects.reverseID.String())).
		Return(nil, redislib.Nil).
		Times(1)
	redisRepo.EXPECT().
		FinalizeTransactionPersistence(gomock.Any(), subjects.orgID, subjects.ledgerID, subjects.reverseID,
			mmodel.BalanceExecutionAttempt{
				ExecutionKey: utils.TransactionBalanceExecutionKey(subjects.orgID, subjects.ledgerID, subjects.reverseID),
				OutcomeKey:   utils.TransactionBalanceOutcomeKey(subjects.orgID, subjects.ledgerID, subjects.reverseID),
				Owner:        subjects.reverseID.String(),
				Outcome:      mmodel.TransactionOutcomeCommitted,
				Identity:     subjects.reverseID,
			}, gomock.Any()).
		Return(nil).
		Times(1)

	handler := &TransactionHandler{
		RevertIdempotencyMode: revertIdempotencyModeFinal,
		RevertUpdateFreeze:    &revertUpdateFreezeStub{ready: true},
		Query: &query.UseCase{
			TransactionRepo:         transactionRepo,
			TransactionMetadataRepo: metadataRepo,
		},
		Command: &command.UseCase{
			TransactionRepo:      transactionRepo,
			RevertClaimRepo:      claimRepo,
			TransactionRedisRepo: redisRepo,
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

	out, err := handler.RevertTransactionHuma(
		libObservability.ContextWithLogger(context.Background(), logger),
		&StateTransactionInputHuma{
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

	warn := findLogRecord(recorder.snapshot(), libLog.LevelWarn, revertIdempotencyReplayedLogMessage)
	require.NotNil(t, warn,
		"a replayed revert must record a Warn (%q) so the replay is visible to operators, not only to the caller",
		revertIdempotencyReplayedLogMessage)
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

	warn := findLogRecord(recorder.snapshot(), libLog.LevelWarn, revertIdempotencyReplayedLogMessage)
	assert.NotNil(t, warn, "the replay Warn must also fire on the wired path")
}

// findLogRecord returns the first captured record with the given level and message, or nil.
func findLogRecord(records []logRecord, level libLog.Level, msg string) *logRecord {
	for i := range records {
		if records[i].level == level && records[i].msg == msg {
			return &records[i]
		}
	}

	return nil
}
