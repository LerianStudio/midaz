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
	"sync"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
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
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// Revert replay visibility. A revert carries no caller idempotency key, so its slot is derived
// inside the create core from the origin-scoped preimage. When that slot already holds a
// serialized reverse the core returns the FIRST reverse instead of creating a second one — a
// 201 that is economically different from a fresh revert. Both revert transports must project
// the core's `replayed` flag onto X-Idempotency-Replayed (the same projection the CREATE
// transports do), and the core must record a Warn so a replayed revert is visible to operators
// as well as to the caller.

// logRecord is one captured call to the GoMock libLog.Logger.
type logRecord struct {
	level  libLog.Level
	msg    string
	fields []libLog.Field
}

// recordingLogger returns the lib-observability GoMock logger wired to append every Log call
// into the returned accumulator. Recording rather than EXPECTing one specific call keeps the
// level/message/field contract assertable in the test body and stays immune to whichever other
// lines the exercised path emits.
func recordingLogger(t *testing.T, ctrl *gomock.Controller) (*libLog.MockLogger, *[]logRecord) {
	t.Helper()

	var (
		mu      sync.Mutex
		records []logRecord
	)

	logger := libLog.NewMockLogger(ctrl)

	capture := func(_ context.Context, level libLog.Level, msg string, fields ...libLog.Field) {
		mu.Lock()
		defer mu.Unlock()

		records = append(records, logRecord{level: level, msg: msg, fields: fields})
	}

	// Two arities: the exercised path emits both field-less lines (the idempotency cache-hit
	// Debug) and lines carrying fields, and a gomock variadic matcher list must match the
	// actual call arity.
	logger.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any()).Do(capture).AnyTimes()
	logger.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(capture).AnyTimes()
	logger.EXPECT().With(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithGroup(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Enabled(gomock.Any()).Return(true).AnyTimes()
	logger.EXPECT().Sync(gomock.Any()).Return(nil).AnyTimes()

	return logger, &records
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
// touching balances. The Redis expectations pin the slot to the origin-scoped preimage, so the
// arrangement is only reachable if the revert core still keys on revertIdempotencyHashSource.
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

	// The claimed slot: keyed by the ORIGIN-scoped preimage, with the pre-migration Fiber TTL.
	internalKey := utils.IdempotencyInternalKey(subjects.orgID, subjects.ledgerID,
		libCommons.HashSHA256(revertIdempotencyHashSource(subjects.originID)))

	redisRepo.EXPECT().
		SetNX(gomock.Any(), internalKey, "", pkgHTTP.ParseIdempotencyTTL("")).
		Return(false, nil).
		Times(1)

	redisRepo.EXPECT().
		Get(gomock.Any(), internalKey).
		Return(string(cachedValue), nil).
		Times(1)

	handler := &TransactionHandler{
		Query: &query.UseCase{
			TransactionRepo:         transactionRepo,
			TransactionMetadataRepo: metadataRepo,
		},
		Command: &command.UseCase{
			TransactionRepo:      transactionRepo,
			TransactionRedisRepo: redisRepo,
		},
	}

	return handler, subjects
}

// revertViaFiber drives the v1 Fiber revert wrapper and returns the replayed header value plus
// the decoded response body.
func revertViaFiber(t *testing.T, ctx context.Context, handler *TransactionHandler, s revertReplaySubjects) (string, map[string]any) {
	t.Helper()

	app := fiber.New()
	app.Post("/revert", func(c fiber.Ctx) error {
		c.Locals("organization_id", s.orgID)
		c.Locals("ledger_id", s.ledgerID)
		c.Locals("transaction_id", s.originID)
		c.SetContext(ctx)

		return c.Next()
	}, handler.RevertTransaction)

	resp, err := app.Test(httptest.NewRequest(nethttp.MethodPost, "/revert", nil), fiber.TestConfig{Timeout: 0})
	require.NoError(t, err, "the Fiber revert request should not fail at the transport layer")

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read the Fiber revert response body")
	require.Equal(t, nethttp.StatusCreated, resp.StatusCode, "a replayed revert still answers 201; body: %s", string(raw))

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body), "the Fiber revert response should be valid JSON; body: %s", string(raw))

	return resp.Header.Get(libConstants.IdempotencyReplayed), body
}

// revertViaHuma drives the Huma revert shell (the SAME shell both the v1 and v2 routes mount)
// and returns the replayed header field off the typed output plus the returned transaction.
func revertViaHuma(t *testing.T, ctx context.Context, handler *TransactionHandler, s revertReplaySubjects) (string, map[string]any) {
	t.Helper()

	out, err := handler.RevertTransactionHuma(ctx, &StateTransactionInputHuma{
		OrganizationID: s.orgID.String(),
		LedgerID:       s.ledgerID.String(),
		TransactionID:  s.originID.String(),
	})
	require.NoError(t, err, "the Huma revert shell should return the replayed transaction")
	require.NotNil(t, out, "the Huma revert shell must return an output envelope")
	require.NotNil(t, out.Body, "the Huma revert shell must return the replayed transaction")
	require.Equal(t, nethttp.StatusCreated, out.Status, "a replayed revert still answers 201")

	return out.IdempotencyReplayed, map[string]any{"id": out.Body.ID}
}

// TestRevertTransaction_ReplayedIdempotency_SurfacesOnBothTransports proves a replayed revert
// is distinguishable from a fresh one on BOTH revert transports: each projects the create
// core's replayed flag onto X-Idempotency-Replayed and returns the FIRST reverse, and the core
// records a Warn naming the origin so the replay is visible to operators too.
func TestRevertTransaction_ReplayedIdempotency_SurfacesOnBothTransports(t *testing.T) {
	t.Parallel()

	transports := []struct {
		name   string
		invoke func(t *testing.T, ctx context.Context, handler *TransactionHandler, s revertReplaySubjects) (string, map[string]any)
	}{
		{name: "v1 Fiber wrapper", invoke: revertViaFiber},
		{name: "Huma shell shared by the v1 and v2 routes", invoke: revertViaHuma},
	}

	for _, tc := range transports {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			logger, records := recordingLogger(t, ctrl)
			handler, subjects := arrangeReplayedRevert(t, ctrl)

			replayed, body := tc.invoke(t, libObservability.ContextWithLogger(context.Background(), logger), handler, subjects)

			assert.Equal(t, "true", replayed,
				"a replayed revert must advertise X-Idempotency-Replayed: true — otherwise a replay is indistinguishable from a fresh revert")
			assert.Equal(t, subjects.reverseID.String(), body["id"],
				"a replayed revert must return the FIRST reverse transaction")

			warn := findLogRecord(*records, libLog.LevelWarn, revertIdempotencyReplayedLogMessage)
			require.NotNil(t, warn,
				"a replayed revert must record a Warn (%q) so the replay is visible to operators, not only to the caller",
				revertIdempotencyReplayedLogMessage)
			assert.Equal(t, []libLog.Field{libLog.String("transaction_id", subjects.originID.String())}, warn.fields,
				"the replay Warn must name the origin transaction id and carry nothing else (no balances, amounts or payloads)")
		})
	}
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
