//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	redpandatestutil "github.com/LerianStudio/midaz/v3/tests/utils/redpanda"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

func skipIfRedpandaITDisabled(t *testing.T) {
	t.Helper()

	if os.Getenv("RUN_REDPANDA_IT") != "1" {
		t.Skip("set RUN_REDPANDA_IT=1 to run Redpanda integration tests")
	}
}

func newIntegrationConsumerClient(t *testing.T, brokers []string, topics ...string) *kgo.Client {
	t.Helper()

	group := "it-redpanda-group-" + libCommons.GenerateUUIDv7().String()

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
	)
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return client
}

func waitForTopicRecords(t *testing.T, client *kgo.Client, topic string, expected int, timeout time.Duration) []*kgo.Record {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	records := make([]*kgo.Record, 0, expected)

	for len(records) < expected {
		fetches := client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			require.FailNowf(t, "consumer closed", "client closed while waiting for %d records on %s", expected, topic)
		}

		if err := fetches.Err0(); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				break
			}

			continue
		}

		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()
			if record.Topic != topic {
				continue
			}

			records = append(records, record)
			if len(records) >= expected {
				break
			}
		}
	}

	require.Lenf(t, records, expected, "expected %d records on topic %s", expected, topic)

	return records
}

func TestProducerRedpanda_BasicPublishRoundTrip_Integration(t *testing.T) {
	skipIfRedpandaITDisabled(t)

	container := redpandatestutil.SetupContainer(t)
	topic := "it.producer.basic." + libCommons.GenerateUUIDv7().String()
	redpandatestutil.SetupTopics(t, container, topic)

	consumer := newIntegrationConsumerClient(t, container.Brokers, topic)

	producer, err := NewProducerRedpanda(container.Brokers, 0, 0, true)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, producer.Close())
	})

	reqID := "req-" + libCommons.GenerateUUIDv7().String()
	ctx := libCommons.ContextWithHeaderID(context.Background(), reqID)
	key := "key-1"
	payload := []byte(`{"event":"created"}`)

	_, err = producer.ProducerDefaultWithContext(ctx, topic, key, payload)
	require.NoError(t, err)

	records := waitForTopicRecords(t, consumer, topic, 1, 20*time.Second)
	record := records[0]

	assert.Equal(t, payload, record.Value)
	assert.Equal(t, []byte(key), record.Key)
	assert.Equal(t, reqID, resolveHeader(record.Headers, libConstants.HeaderID))
}

func TestProducerRedpanda_ConcurrentPublish_Integration(t *testing.T) {
	skipIfRedpandaITDisabled(t)

	container := redpandatestutil.SetupContainer(t)
	topic := "it.producer.concurrent." + libCommons.GenerateUUIDv7().String()
	redpandatestutil.SetupTopics(t, container, topic)

	consumer := newIntegrationConsumerClient(t, container.Brokers, topic)

	producer, err := NewProducerRedpanda(container.Brokers, 0, 0, true)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, producer.Close())
	})

	const totalMessages = 24

	errCh := make(chan error, totalMessages)
	var wg sync.WaitGroup
	expectedByKey := make(map[string]string, totalMessages)

	for i := range totalMessages {
		key := fmt.Sprintf("k-%d", i)
		payload := fmt.Sprintf("payload-%d", i)
		expectedByKey[key] = payload

		wg.Add(1)
		go func(messageKey, messagePayload string) {
			defer wg.Done()

			payload := []byte(messagePayload)

			_, publishErr := producer.ProducerDefaultWithContext(context.Background(), topic, messageKey, payload)
			errCh <- publishErr
		}(key, payload)
	}

	wg.Wait()
	close(errCh)

	for publishErr := range errCh {
		require.NoError(t, publishErr)
	}

	records := waitForTopicRecords(t, consumer, topic, totalMessages, 30*time.Second)
	assert.Len(t, records, totalMessages)

	actualCounts := make(map[string]int, totalMessages)
	actualPayloadByKey := make(map[string]string, totalMessages)

	for _, record := range records {
		key := string(record.Key)
		actualCounts[key]++
		actualPayloadByKey[key] = string(record.Value)
	}

	assert.Len(t, actualCounts, totalMessages)
	for key, payload := range expectedByKey {
		assert.Equal(t, 1, actualCounts[key], "unexpected duplicate/missing key %s", key)
		assert.Equal(t, payload, actualPayloadByKey[key], "payload mismatch for key %s", key)
	}
}

func TestConsumerRoutes_RoutesFailedMessagesToDLT_Integration(t *testing.T) {
	skipIfRedpandaITDisabled(t)

	container := redpandatestutil.SetupContainer(t)

	baseTopic := "it.consumer.dlt." + libCommons.GenerateUUIDv7().String()
	retryTopic := baseTopic + retryTopicSuffix
	dltTopic := baseTopic + dltTopicSuffix

	redpandatestutil.SetupTopics(t, container, baseTopic, retryTopic, dltTopic)

	routes := NewConsumerRoutesWithSecurity(
		container.Brokers,
		"it-dlt-group-"+libCommons.GenerateUUIDv7().String(),
		1,
		0,
		libZap.InitializeLogger(),
		&libOpentelemetry.Telemetry{},
		ClientSecurityConfig{},
		1,
	)
	t.Cleanup(routes.Stop)

	var attempts atomic.Int32
	routes.Register(baseTopic, func(_ context.Context, _ []byte) error {
		attempts.Add(1)
		return errors.New("forced handler error")
	})

	require.NoError(t, routes.RunConsumers())

	dltConsumer := newIntegrationConsumerClient(t, container.Brokers, dltTopic)

	producer, err := NewProducerRedpanda(container.Brokers, 0, 0, true)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, producer.Close())
	})

	key := "dlt-key"
	payload := []byte(`{"event":"force-dlt"}`)

	_, err = producer.ProducerDefaultWithContext(context.Background(), baseTopic, key, payload)
	require.NoError(t, err)

	records := waitForTopicRecords(t, dltConsumer, dltTopic, 1, 40*time.Second)
	record := records[0]

	assert.Equal(t, []byte(key), record.Key)
	assert.Equal(t, payload, record.Value)
	assert.Equal(t, "2", resolveHeader(record.Headers, retryAttemptHeader))
	assert.Contains(t, resolveHeader(record.Headers, "x-midaz-handler-error"), "forced handler error")
	assert.GreaterOrEqual(t, attempts.Load(), int32(2))
}
