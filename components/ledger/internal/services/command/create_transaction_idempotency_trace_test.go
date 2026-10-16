// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

const setIdempotencySpanName = "command.set_value_idempotency_key"

// spanEndSignal closes ended once a span with the watched name ends, so a test can wait
// for the idempotency goroutine's span without sleeping.
type spanEndSignal struct {
	name  string
	once  sync.Once
	ended chan struct{}
}

func (s *spanEndSignal) OnStart(context.Context, sdktrace.ReadWriteSpan) {}

func (s *spanEndSignal) OnEnd(span sdktrace.ReadOnlySpan) {
	if span.Name() == s.name {
		s.once.Do(func() { close(s.ended) })
	}
}

func (s *spanEndSignal) Shutdown(context.Context) error   { return nil }
func (s *spanEndSignal) ForceFlush(context.Context) error { return nil }

// idempotencyWrite is what the mocked Redis Set observed from the background goroutine.
type idempotencyWrite struct {
	keyErr      error
	ctxErr      error
	tenantID    string
	key         string
	tenantedKey string
	ttl         time.Duration
}

// Scenarios: Gravação de idempotência fica no trace da requisição; Cancelamento da
// requisição não cancela a gravação; Tenant preservado na goroutine em multi-tenant.
//
// The idempotency write runs on a goroutine the command starts before returning. The
// mocked Set blocks until the test has cancelled the request context, so the write is
// observed strictly after the cancellation; the waits are bounded by channels, not sleeps.
func TestCreateTransactionIdempotencyWrite_StaysInTheRequestTrace(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	recorder := tracetest.NewSpanRecorder()
	endSignal := &spanEndSignal{name: setIdempotencySpanName, ended: make(chan struct{})}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder), sdktrace.WithSpanProcessor(endSignal))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	const tenantID = "tenant-a"

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	release := make(chan struct{})
	written := make(chan idempotencyWrite, 1)

	var releaseOnce sync.Once

	releaseWrite := func() { releaseOnce.Do(func() { close(release) }) }

	// Unblocks the mocked Set on every exit path, so a failed assertion before the
	// explicit release below cannot leave the command's goroutine blocked.
	t.Cleanup(releaseWrite)

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
	redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
		func(ctx context.Context, key, _ string, ttl time.Duration) error {
			<-release

			// Runs on the command's goroutine: no t.FailNow here, the test asserts later.
			tenantedKey, keyErr := tmvalkey.GetKeyContext(ctx, key)

			written <- idempotencyWrite{
				keyErr:      keyErr,
				ctxErr:      ctx.Err(),
				tenantID:    tmcore.GetTenantIDContext(ctx),
				key:         key,
				tenantedKey: tenantedKey,
				ttl:         ttl,
			}

			return nil
		},
	)

	source := translationBalance(organizationID, ledgerID, "33333333-3333-4333-8333-333333333333", "@source", constant.DefaultBalanceKey)
	target := translationBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@target", constant.DefaultBalanceKey)
	uc := &UseCase{
		TransactionRedisRepo:        redisRepo,
		TransactionReader:           &createEngineReader{balances: []*mmodel.Balance{source, target}},
		Engine:                      &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}},
		AppliedTransactionCompleter: &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}},
		EngineRecoveryAcknowledger:  &recordingEngineRecoveryAcknowledger{},
	}

	tracer := tp.Tracer("command-test")
	requestCtx, cancel := context.WithCancel(libObservability.ContextWithTracer(context.Background(), tracer))
	requestCtx = tmcore.ContextWithTenantID(requestCtx, tenantID)

	requestCtx, requestSpan := tracer.Start(requestCtx, "http.request")

	got, _, err := uc.CreateTransactionV1(requestCtx, CreateTransactionV1Input{
		OrganizationID: organizationID, LedgerID: ledgerID,
		Transaction:       createEngineTransaction(time.Date(2026, time.September, 8, 12, 30, 0, 0, time.UTC)),
		TransactionStatus: constant.CREATED, IdempotencyTTL: time.Minute,
	})
	require.NoError(t, err)
	require.NotNil(t, got)

	// The client goes away right after the response: the request is cancelled and its
	// span ends before the background write runs.
	cancel()
	requestSpan.End()
	releaseWrite()

	var write idempotencyWrite

	select {
	case write = <-written:
	case <-time.After(5 * time.Second):
		t.Fatal("the idempotency value was not written after the request was cancelled")
	}

	select {
	case <-endSignal.ended:
	case <-time.After(5 * time.Second):
		t.Fatalf("span %q never ended", setIdempotencySpanName)
	}

	// Scenario: Cancelamento da requisição não cancela a gravação.
	require.NoError(t, write.ctxErr, "the write must not inherit the request cancellation")
	assert.Equal(t, time.Minute, write.ttl, "the configured TTL is kept")

	// Scenario: Tenant preservado na goroutine em multi-tenant.
	assert.Equal(t, tenantID, write.tenantID)
	require.NoError(t, write.keyErr)
	assert.Equal(t, "tenant:"+tenantID+":"+write.key, write.tenantedKey,
		"the Redis adapter namespaces the key with the request's tenant")
	assert.True(t, strings.HasPrefix(write.key, "idempotency:{"+organizationID.String()+":"+ledgerID.String()),
		"the key is the transaction idempotency key of the request's ledger, got %q", write.key)

	// Scenario: Gravação de idempotência fica no trace da requisição.
	commandSpan := spantest.FindSpan(t, recorder, "command.create_transaction_v1")
	writeSpan := spantest.FindSpan(t, recorder, setIdempotencySpanName)

	assert.Equal(t, requestSpan.SpanContext().TraceID(), writeSpan.SpanContext().TraceID(),
		"the background write is in the request trace")
	assert.Equal(t, commandSpan.SpanContext().SpanID(), writeSpan.Parent().SpanID(),
		"the background write is a child of the command span")
}
