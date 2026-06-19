// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"

	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	"github.com/LerianStudio/lib-observability/zap"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

// newTestProducer creates a ProducerRabbitMQRepository for testing without
// calling NewProducerRabbitMQ (which invokes GetNewConnect that may log.Fatal).
func newTestProducer() *ProducerRabbitMQRepository {
	logger, err := zap.New(zap.Config{Environment: zap.EnvironmentLocal, OTelLibraryName: "reporter"})
	if err != nil {
		logger = &zap.Logger{}
	}

	conn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: "amqp://invalid:invalid@localhost:0",
		Host:                   "localhost",
		Port:                   "0",
		User:                   "invalid",
		Pass:                   "invalid",
		Queue:                  "test-queue",
		Logger:                 logger,
	}

	return &ProducerRabbitMQRepository{conn: conn}
}

// TestProducerDefault_RetryBehavior exercises the retry loop's interaction with
// context cancellation. The retry wait now goes through libBackoff.WaitContext
// (the ctx-timeout seam replacing the old overridable sleepFunc), so a cancelled
// or deadline-exceeded context aborts the loop and surfaces the context error
// instead of blocking through the remaining backoff windows.
func TestProducerDefault_RetryBehavior(t *testing.T) {
	t.Parallel()

	t.Run("Error - RetryExhaustionWithoutBlocking", func(t *testing.T) {
		t.Parallel()

		// An already-cancelled context makes WaitContext return immediately on the
		// first retry, so the loop terminates fast without real backoff sleeps. The
		// publish still fails (no broker), so an error is returned.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		producer := newTestProducer()

		msg := model.ReportMessage{
			ReportID:     uuid.New(),
			TemplateID:   uuid.New(),
			OutputFormat: "pdf",
		}

		_, err := producer.ProducerDefault(ctx, "test-exchange", "test-key", msg)

		require.Error(t, err)
	})

	t.Run("Error - DeadlineInterruptsRetryWait", func(t *testing.T) {
		t.Parallel()

		// A short deadline lets the first attempt fail and the first retry wait be
		// interrupted by WaitContext, so the call returns well before the full
		// ProducerMaxRetries backoff schedule (which would otherwise span seconds)
		// elapses.
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		producer := newTestProducer()

		msg := model.ReportMessage{
			ReportID:     uuid.New(),
			TemplateID:   uuid.New(),
			OutputFormat: "pdf",
		}

		start := time.Now()
		_, err := producer.ProducerDefault(ctx, "test-exchange", "test-key", msg)
		elapsed := time.Since(start)

		require.Error(t, err)
		// Without ctx-aware waits the loop would block through the full backoff
		// schedule (>> 1s). With WaitContext it returns near the deadline.
		assert.Less(t, elapsed, time.Second, "deadline must interrupt the retry wait quickly")
	})
}

func TestProducerDefault_RetryConstants(t *testing.T) {
	t.Parallel()

	// Verify the constants match the midaz pattern
	assert.Equal(t, 5, constant.ProducerMaxRetries)
	assert.Equal(t, 500*time.Millisecond, constant.ProducerInitialBackoff)
	assert.Equal(t, 10*time.Second, constant.ProducerMaxBackoff)
}

func TestProducerRabbitMQRepository_StructFields(t *testing.T) {
	t.Parallel()

	logger, err := zap.New(zap.Config{Environment: zap.EnvironmentLocal, OTelLibraryName: "reporter"})
	require.NoError(t, err)

	conn := &libRabbitmq.RabbitMQConnection{
		Logger: logger,
	}

	producer := &ProducerRabbitMQRepository{conn: conn}

	assert.NotNil(t, producer.conn)
	assert.Equal(t, conn, producer.conn)
}

func TestQueueMessageTelemetryAttributes_RedactsPayloadDetails(t *testing.T) {
	t.Parallel()

	msg := model.ReportMessage{
		ReportID:     uuid.New(),
		TemplateID:   uuid.New(),
		OutputFormat: "pdf",
		Filters: map[string]map[string]map[string]model.FilterCondition{
			"customers": {
				"accounts": {
					"document": {Equals: []any{"12345678900"}},
				},
			},
		},
		MappedFields: map[string]map[string][]string{
			"customers": {"accounts": {"document"}},
		},
	}

	attrs := queueMessageTelemetryAttributes(msg)

	assert.Contains(t, attrs, attribute.String("app.request.template_id", msg.TemplateID.String()))
	assert.Contains(t, attrs, attribute.String("app.request.report_id", msg.ReportID.String()))
	assert.Contains(t, attrs, attribute.Int("app.request.filter_datasource_count", 1))
	assert.NotContains(t, attrs, attribute.String("app.request.rabbitmq.message", "12345678900"))
}
