// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const logTransactionSpanName = "command.transaction.log_transaction"

// auditPublish is what the mocked producer observed from the audit goroutine.
type auditPublish struct {
	ctxErr   error
	span     trace.SpanContext
	tenantID string
}

// The pending-transition audit runs on a goroutine the commit starts before returning.
// The fixture projects the transition synchronously (TransactionWriteBehindAsync is false,
// so dispatched is false, and the completer confirms APPROVED, so projected is true),
// which satisfies shouldEmitEngineWriteBehindAudit. The mocked producer blocks until the
// test has cancelled the request context; the waits are bounded by channels, not sleeps.
func TestPendingTransitionAudit_StaysInTheRequestTrace(t *testing.T) {
	setAuditTestRoutingEnv(t)
	t.Setenv("AUDIT_LOG_ENABLED", "true")

	recorder := tracetest.NewSpanRecorder()
	endSignal := &spanEndSignal{name: logTransactionSpanName, ended: make(chan struct{})}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder), sdktrace.WithSpanProcessor(endSignal))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	release := make(chan struct{})
	published := make(chan auditPublish, 1)

	var releaseOnce sync.Once

	releasePublish := func() { releaseOnce.Do(func() { close(release) }) }

	// Unblocks the mocked producer on every exit path, so a failed assertion before the
	// explicit release below cannot leave the audit goroutine blocked.
	t.Cleanup(releasePublish)

	uc, _, _, _, in := newTransitionEngineUseCase(t, constant.APPROVED)

	producer := rabbitmq.NewMockProducerRepository(gomock.NewController(t))
	producer.EXPECT().
		ProducerDefault(gomock.Any(), auditTestExchange, auditTestKey, gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, _ []byte) (*string, error) {
			<-release

			published <- auditPublish{
				ctxErr:   ctx.Err(),
				span:     trace.SpanContextFromContext(ctx),
				tenantID: tmcore.GetTenantIDContext(ctx),
			}

			return nil, nil
		}).
		Times(1)

	uc.RabbitMQRepo = producer

	tracer := tp.Tracer("command-test")
	requestCtx, cancel := context.WithCancel(libObservability.ContextWithTracer(context.Background(), tracer))
	requestCtx = tmcore.ContextWithTenantID(requestCtx, "tenant-transition")

	requestCtx, requestSpan := tracer.Start(requestCtx, "http.request")

	got, err := uc.CommitTransactionV1(requestCtx, in)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, constant.APPROVED, got.Status.Code)

	// The client goes away right after the response.
	cancel()
	requestSpan.End()
	releasePublish()

	var publish auditPublish

	select {
	case publish = <-published:
	case <-time.After(5 * time.Second):
		t.Fatal("the pending-transition audit was not published after the request was cancelled")
	}

	select {
	case <-endSignal.ended:
	case <-time.After(5 * time.Second):
		t.Fatalf("span %q never ended", logTransactionSpanName)
	}

	require.NoError(t, publish.ctxErr, "the audit must not inherit the request cancellation")
	assert.Equal(t, "tenant-transition", publish.tenantID)

	auditSpan := spantest.FindSpan(t, recorder, logTransactionSpanName)
	commandSpan := spantest.FindSpan(t, recorder, "command.transition_pending_transaction")

	assert.Equal(t, requestSpan.SpanContext().TraceID(), auditSpan.SpanContext().TraceID(),
		"the audit is in the request trace")
	assert.Equal(t, commandSpan.SpanContext().SpanID(), auditSpan.Parent().SpanID(),
		"the audit is a child of the pending-transition command span")
	assert.Equal(t, auditSpan.SpanContext().SpanID(), publish.span.SpanID(),
		"the producer runs under the audit span")
}
