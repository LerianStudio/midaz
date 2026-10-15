// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package rabbitmq

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v7/commons/circuitbreaker"
	libRabbitmq "github.com/LerianStudio/lib-commons/v7/commons/rabbitmq"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libZap "github.com/LerianStudio/lib-observability/v4/zap"

	rmqtestutil "github.com/LerianStudio/midaz/v4/tests/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	confirmMissingExchange = "confirm-missing-exchange"
	confirmRejectingKey    = "confirm.rejecting"
	confirmRejectingQueue  = "confirm-rejecting-queue"
)

// declareRejectingQueue binds a queue that refuses every message, so the broker
// answers each publish routed to it with a basic.nack.
func declareRejectingQueue(t *testing.T, ch *amqp.Channel, exchange string) {
	t.Helper()

	_, err := ch.QueueDeclare(confirmRejectingQueue, true, false, false, false, amqp.Table{
		"x-max-length": int32(0),
		"x-overflow":   "reject-publish",
	})
	require.NoError(t, err)
	require.NoError(t, ch.QueueBind(confirmRejectingQueue, confirmRejectingKey, exchange, false, nil))
}

func TestIntegration_RabbitMQ_ProducerDefaultConfirmsPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	declareRejectingQueue(t, infra.rmqContainer.Channel, infra.exchange)

	t.Run("returns after the broker holds the message", func(t *testing.T) {
		_, err := infra.producer.ProducerDefault(context.Background(), infra.exchange, infra.routingKey, []byte(`{"confirmed":true}`))
		require.NoError(t, err)

		assert.Equal(t, 1, rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue),
			"a confirmed persistent publish is already enqueued when ProducerDefault returns")
	})

	t.Run("fails when the broker nacks the message", func(t *testing.T) {
		_, err := infra.producer.ProducerDefault(context.Background(), infra.exchange, confirmRejectingKey, []byte(`{"nacked":true}`))
		require.ErrorIs(t, err, ErrPublishNacked)
	})

	t.Run("concurrent publishers on the shared channel each get their own outcome", func(t *testing.T) {
		const publishers = 20

		before := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)
		errs := make([]error, publishers)

		var wg sync.WaitGroup

		for i := range publishers {
			wg.Add(1)

			go func() {
				defer wg.Done()

				key := infra.routingKey
				if i%2 == 1 {
					key = confirmRejectingKey
				}

				_, errs[i] = infra.producer.ProducerDefault(context.Background(), infra.exchange, key, []byte(`{"concurrent":true}`))
			}()
		}

		wg.Wait()

		for i, err := range errs {
			if i%2 == 1 {
				assert.ErrorIs(t, err, ErrPublishNacked, "publisher %d targeted the rejecting queue", i)
			} else {
				assert.NoError(t, err, "publisher %d targeted the accepting queue", i)
			}
		}

		assert.Equal(t, before+publishers/2, rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue))
	})

	t.Run("fails when the broker closes the channel for a missing exchange", func(t *testing.T) {
		before := rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue)

		_, err := infra.producer.ProducerDefault(context.Background(), confirmMissingExchange, infra.routingKey, []byte(`{"lost":true}`))
		require.ErrorIs(t, err, ErrPublishUnconfirmed)

		_, err = infra.producer.ProducerDefault(context.Background(), infra.exchange, infra.routingKey, []byte(`{"after":"close"}`))
		require.NoError(t, err, "the next publish must reopen the channel and confirm again")
		assert.Equal(t, before+1, rmqtestutil.GetQueueMessageCount(t, infra.rmqContainer.Channel, infra.queue))
	})
}

func TestIntegration_RabbitMQ_ProducerDefaultBoundsConfirmationWait(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rmqContainer := rmqtestutil.SetupContainer(t)

	const (
		exchange   = "confirm-wait-exchange"
		routingKey = "confirm.wait"
		queue      = "confirm-wait-queue"
	)

	rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
	rmqtestutil.SetupQueue(t, rmqContainer.Channel, queue, exchange, routingKey)

	defaultProducer := newConfirmWaitProducer(t, rmqContainer)
	shortProducer := newConfirmWaitProducer(t, rmqContainer, WithPublishConfirmTimeout(400*time.Millisecond))

	// Each producer puts its channel in confirm mode before the broker stops
	// reading from publishing connections.
	for _, producer := range []*ProducerRabbitMQRepository{defaultProducer, shortProducer} {
		_, err := producer.ProducerDefault(context.Background(), exchange, routingKey, []byte(`{"warm":true}`))
		require.NoError(t, err)
	}

	// A memory alarm makes the broker stop reading from publishing connections,
	// so a publish is written but never confirmed.
	setMemoryWatermark(t, rmqContainer, "0")
	t.Cleanup(func() { setMemoryWatermark(t, rmqContainer, "0.4") })
	waitForMemoryAlarm(t, rmqContainer)

	t.Run("the caller deadline ends the wait", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		started := time.Now()
		_, err := defaultProducer.ProducerDefault(ctx, exchange, routingKey, []byte(`{"blocked":"deadline"}`))

		require.ErrorIs(t, err, ErrPublishUnconfirmed)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, time.Since(started), 3*time.Second)
	})

	t.Run("the configured ceiling ends the wait without a caller deadline", func(t *testing.T) {
		started := time.Now()
		_, err := shortProducer.ProducerDefault(context.Background(), exchange, routingKey, []byte(`{"blocked":"configured"}`))

		require.ErrorIs(t, err, ErrPublishUnconfirmed)
		elapsed := time.Since(started)
		assert.GreaterOrEqual(t, elapsed, 350*time.Millisecond)
		assert.Less(t, elapsed, 3*time.Second)
	})

	t.Run("the default ceiling ends the wait without a caller deadline", func(t *testing.T) {
		started := time.Now()
		_, err := defaultProducer.ProducerDefault(context.Background(), exchange, routingKey, []byte(`{"blocked":"ceiling"}`))

		require.ErrorIs(t, err, ErrPublishUnconfirmed)
		elapsed := time.Since(started)
		assert.GreaterOrEqual(t, elapsed, DefaultPublishConfirmTimeout-100*time.Millisecond)
		assert.Less(t, elapsed, DefaultPublishConfirmTimeout+3*time.Second)
	})
}

func TestIntegration_MultiTenantProducer_ConfirmsPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	const tenantID = "tenant-confirm"

	infra := setupMultiTenantInfra(t, []string{tenantID})
	tenant := infra.tenants[tenantID]
	declareRejectingQueue(t, tenant.channel, infra.exchange)

	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)

	t.Run("returns after the broker holds the message", func(t *testing.T) {
		_, err := infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, []byte(`{"confirmed":true}`))
		require.NoError(t, err)

		assert.Equal(t, 1, rmqtestutil.GetQueueMessageCount(t, tenant.channel, infra.queue))
	})

	t.Run("fails when the broker nacks the message", func(t *testing.T) {
		_, err := infra.producer.ProducerDefault(ctx, infra.exchange, confirmRejectingKey, []byte(`{"nacked":true}`))
		require.ErrorIs(t, err, ErrPublishNacked)
	})

	t.Run("fails when the broker closes the channel for a missing exchange", func(t *testing.T) {
		_, err := infra.producer.ProducerDefault(ctx, confirmMissingExchange, infra.routingKey, []byte(`{"lost":true}`))
		require.ErrorIs(t, err, ErrPublishUnconfirmed)

		_, err = infra.producer.ProducerDefault(ctx, infra.exchange, infra.routingKey, []byte(`{"after":"close"}`))
		require.NoError(t, err)
		assert.Equal(t, 2, rmqtestutil.GetQueueMessageCount(t, tenant.channel, infra.queue))
	})
}

func TestIntegration_CircuitBreaker_CountsUnconfirmedPublishAsFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupCircuitBreakerTestInfra(t, defaultCircuitBreakerConfig())
	declareRejectingQueue(t, infra.rmqContainer.Channel, infra.exchange)

	_, err := infra.cbProducer.ProducerDefault(context.Background(), infra.exchange, confirmRejectingKey, []byte(`{"nacked":true}`))
	require.Error(t, err)

	counts := infra.cbProducer.GetCounts()
	assert.Equal(t, uint32(1), counts.TotalFailures)
	assert.Equal(t, uint32(0), counts.TotalSuccesses)
	assert.Equal(t, libCircuitBreaker.StateClosed, infra.cbProducer.GetCircuitState())
}

func newConfirmWaitProducer(t *testing.T, rmq *rmqtestutil.ContainerResult, opts ...ProducerOption) *ProducerRabbitMQRepository {
	t.Helper()

	logger, err := libZap.New(libZap.Config{Environment: libZap.EnvironmentDevelopment, OTelLibraryName: "midaz-tests"})
	require.NoError(t, err)

	conn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource:   rmq.URI,
		HealthCheckURL:           "http://" + rmq.Host + ":" + rmq.MgmtPort,
		AllowInsecureHealthCheck: true,
		Host:                     rmq.Host,
		Port:                     rmq.AMQPPort,
		User:                     rmqtestutil.DefaultUser,
		Pass:                     rmqtestutil.DefaultPassword,
		Logger:                   logger,
	}

	producer, err := NewProducerRabbitMQ(conn, opts...)
	require.NoError(t, err)

	t.Cleanup(func() { _ = producer.Close() })

	return producer
}

func setMemoryWatermark(t *testing.T, rmq *rmqtestutil.ContainerResult, value string) {
	t.Helper()

	code, _, err := rmq.Container.Exec(context.Background(), []string{"rabbitmqctl", "set_vm_memory_high_watermark", value})
	require.NoError(t, err)
	require.Equal(t, 0, code, "rabbitmqctl set_vm_memory_high_watermark %s", value)
}

func waitForMemoryAlarm(t *testing.T, rmq *rmqtestutil.ContainerResult) {
	t.Helper()

	url := "http://" + rmq.Host + ":" + rmq.MgmtPort + "/api/health/checks/alarms"
	client := &http.Client{Timeout: 2 * time.Second}

	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			return false
		}

		req.SetBasicAuth(rmqtestutil.DefaultUser, rmqtestutil.DefaultPassword)

		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		return resp.StatusCode == http.StatusServiceUnavailable
	}, 15*time.Second, 100*time.Millisecond, "memory alarm never raised")
}
