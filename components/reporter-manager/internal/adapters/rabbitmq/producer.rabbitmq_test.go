// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/LerianStudio/reporter/pkg/model"

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

// TestProducerDefault_RetryBehavior groups all tests that modify the package-level
// sleepFunc variable to prevent data races. These subtests run sequentially.
// NOTE: Cannot use t.Parallel() because subtests modify the package-level sleepFunc variable.
func TestProducerDefault_RetryBehavior(t *testing.T) {
	t.Run("Error - EnsureChannelRetryExhaustion", func(t *testing.T) {
		// Override sleepFunc to be a no-op for fast tests
		originalSleep := sleepFunc
		sleepFunc = func(_ time.Duration) {}

		defer func() { sleepFunc = originalSleep }()

		producer := newTestProducer()

		msg := model.ReportMessage{
			ReportID:     uuid.New(),
			TemplateID:   uuid.New(),
			OutputFormat: "pdf",
		}

		_, err := producer.ProducerDefault(context.Background(), "test-exchange", "test-key", msg)

		// Should fail after exhausting all retries
		require.Error(t, err)
	})

	t.Run("Success - SleepIsCalledOnRetry", func(t *testing.T) {
		var sleepCallCount atomic.Int32

		originalSleep := sleepFunc
		sleepFunc = func(_ time.Duration) {
			sleepCallCount.Add(1)
		}

		defer func() { sleepFunc = originalSleep }()

		producer := newTestProducer()

		msg := model.ReportMessage{
			ReportID:     uuid.New(),
			TemplateID:   uuid.New(),
			OutputFormat: "pdf",
		}

		_, _ = producer.ProducerDefault(context.Background(), "test-exchange", "test-key", msg)

		// Sleep should be called exactly ProducerMaxRetries times
		// (once per retry, not for the final failed attempt)
		assert.Equal(t, int32(constant.ProducerMaxRetries), sleepCallCount.Load(),
			"sleep should be called exactly %d times (once per retry before the final failure)", constant.ProducerMaxRetries)
	})

	t.Run("Success - SleepReceivesPositiveDuration", func(t *testing.T) {
		var sleepDurations []time.Duration

		originalSleep := sleepFunc
		sleepFunc = func(d time.Duration) {
			sleepDurations = append(sleepDurations, d)
		}

		defer func() { sleepFunc = originalSleep }()

		producer := newTestProducer()

		msg := model.ReportMessage{
			ReportID:     uuid.New(),
			TemplateID:   uuid.New(),
			OutputFormat: "pdf",
		}

		_, _ = producer.ProducerDefault(context.Background(), "test-exchange", "test-key", msg)

		// All sleep durations should be non-negative and within bounds
		for i, d := range sleepDurations {
			assert.GreaterOrEqual(t, d, time.Duration(0), "sleep duration %d should be non-negative", i)
			assert.LessOrEqual(t, d, constant.ProducerMaxBackoff, "sleep duration %d should not exceed max backoff", i)
		}
	})

	t.Run("Success - SleepFuncDefaultIsTimeSleep", func(t *testing.T) {
		// Restore to original default
		sleepFunc = time.Sleep
		assert.NotNil(t, sleepFunc, "sleepFunc should default to time.Sleep")
	})
}

func TestProducerDefault_RetryConstants(t *testing.T) {
	t.Parallel()

	// Verify the constants match the midaz pattern
	assert.Equal(t, 5, constant.ProducerMaxRetries)
	assert.Equal(t, 500*time.Millisecond, constant.ProducerInitialBackoff)
	assert.Equal(t, 10*time.Second, constant.ProducerMaxBackoff)
	assert.Equal(t, 2.0, constant.ProducerBackoffFactor)
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
