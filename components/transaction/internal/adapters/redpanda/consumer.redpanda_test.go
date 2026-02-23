// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"
	"math"
	"testing"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestNewConsumerRoutes_UsesDefaults(t *testing.T) {
	routes := NewConsumerRoutes([]string{"127.0.0.1:9092"}, "", 0, 0, nil, nil)

	require.NotNil(t, routes)
	assert.Equal(t, defaultConsumerGroup, routes.consumerGroup)
	assert.Equal(t, defaultConsumerWorkers, routes.NumbersOfWorker)
	assert.Equal(t, defaultMaxRetryAttempts, routes.maxRetryAttempts)
	assert.NotNil(t, routes.routes)
	assert.NotNil(t, routes.cancel)
}

func TestConsumerRoutes_Stop_NilAndIdempotent(t *testing.T) {
	var nilRoutes *ConsumerRoutes
	nilRoutes.Stop()

	routes := NewConsumerRoutesWithSecurity(
		[]string{"127.0.0.1:9092"},
		"group",
		1,
		0,
		libZap.InitializeLogger(),
		nil,
		ClientSecurityConfig{},
		defaultMaxRetryAttempts,
	)

	routes.Stop()
	routes.Stop()
}

func TestConsumerRoutes_Register(t *testing.T) {
	routes := &ConsumerRoutes{
		routes: make(map[string]QueueHandlerFunc),
		Logger: libZap.InitializeLogger(),
	}

	handler := func(_ context.Context, _ []byte) error { return nil }
	routes.Register("ledger.balance.operations", handler)

	assert.Len(t, routes.routes, 1)
	assert.NotNil(t, routes.routes["ledger.balance.operations"])
}

func TestConsumerRoutes_RunConsumers_NoRoutes(t *testing.T) {
	routes := &ConsumerRoutes{
		routes: make(map[string]QueueHandlerFunc),
		Logger: libZap.InitializeLogger(),
	}

	err := routes.RunConsumers()
	require.NoError(t, err)
}

func TestConsumerRoutes_RunConsumers_NilReceiver(t *testing.T) {
	var routes *ConsumerRoutes

	err := routes.RunConsumers()
	require.Error(t, err)
	assert.ErrorContains(t, err, "consumer routes are nil")
}

func TestConsumerRoutes_RunConsumers_InvalidSecurityConfig(t *testing.T) {
	routes := NewConsumerRoutesWithSecurity(
		[]string{"127.0.0.1:9092"},
		"test-group",
		1,
		int(math.MaxInt32)+1024,
		libZap.InitializeLogger(),
		nil,
		ClientSecurityConfig{SASLEnabled: true},
		defaultMaxRetryAttempts,
	)
	routes.Register("ledger.balance.operations", func(_ context.Context, _ []byte) error { return nil })

	err := routes.RunConsumers()
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid redpanda security configuration")
}

func TestResolveHandler(t *testing.T) {
	routes := &ConsumerRoutes{
		routes: map[string]QueueHandlerFunc{
			"ledger.balance.operations": func(_ context.Context, _ []byte) error { return nil },
		},
	}

	_, ok := routes.resolveHandler("ledger.balance.operations")
	assert.True(t, ok)

	_, ok = routes.resolveHandler("ledger.balance.operations.retry")
	assert.True(t, ok)

	_, ok = routes.resolveHandler("ledger.balance.unknown")
	assert.False(t, ok)
}

func TestParseRetryAttempt(t *testing.T) {
	assert.Equal(t, 0, parseRetryAttempt(nil))

	headers := []kgo.RecordHeader{{Key: retryAttemptHeader, Value: []byte("2")}}
	assert.Equal(t, 2, parseRetryAttempt(headers))

	headers = []kgo.RecordHeader{{Key: retryAttemptHeader, Value: []byte("abc")}}
	assert.Equal(t, 0, parseRetryAttempt(headers))

	headers = []kgo.RecordHeader{{Key: retryAttemptHeader, Value: []byte("-1")}}
	assert.Equal(t, 0, parseRetryAttempt(headers))
}

func TestResolveHeader(t *testing.T) {
	headers := []kgo.RecordHeader{
		{Key: "a", Value: []byte("1")},
		{Key: "b", Value: []byte("")},
		{Key: "c", Value: []byte("3")},
	}

	assert.Equal(t, "1", resolveHeader(headers, "a"))
	assert.Equal(t, "", resolveHeader(headers, "b"))
	assert.Equal(t, "", resolveHeader(headers, "missing"))
}

func TestUpsertHeader(t *testing.T) {
	headers := []kgo.RecordHeader{{Key: "a", Value: []byte("1")}}
	headers = upsertHeader(headers, "a", []byte("2"))
	assert.Len(t, headers, 1)
	assert.Equal(t, "2", string(headers[0].Value))

	headers = upsertHeader(headers, "b", []byte("3"))
	assert.Len(t, headers, 2)
}

func TestCloneHeaders(t *testing.T) {
	original := []kgo.RecordHeader{{Key: "a", Value: []byte("1")}}
	cloned := cloneHeaders(original)
	cloned = append(cloned, kgo.RecordHeader{Key: "b", Value: []byte("2")})

	assert.Len(t, original, 1)
	assert.Len(t, cloned, 2)
}

func TestResolveFailedRecordTargetTopic(t *testing.T) {
	assert.Equal(t,
		"ledger.balance.operations.retry",
		resolveFailedRecordTargetTopic("ledger.balance.operations", 1, 3),
	)

	assert.Equal(t,
		"ledger.balance.operations.retry",
		resolveFailedRecordTargetTopic("ledger.balance.operations.retry", 2, 3),
	)

	assert.Equal(t,
		"ledger.balance.operations.dlt",
		resolveFailedRecordTargetTopic("ledger.balance.operations.retry", 4, 3),
	)
}

func TestNewConsumerRoutesWithSecurity_DefaultsNilLogger(t *testing.T) {
	routes := NewConsumerRoutesWithSecurity(
		[]string{"127.0.0.1:9092"},
		"",
		0,
		0,
		nil,
		nil,
		ClientSecurityConfig{},
		0,
	)

	assert.NotNil(t, routes)
	assert.NotNil(t, routes.Logger)
	assert.Equal(t, defaultConsumerGroup, routes.consumerGroup)
	assert.Equal(t, defaultConsumerWorkers, routes.NumbersOfWorker)
	assert.Equal(t, defaultMaxRetryAttempts, routes.maxRetryAttempts)
}

func TestRouteFailedRecord_NilRecord(t *testing.T) {
	routes := &ConsumerRoutes{maxRetryAttempts: defaultMaxRetryAttempts}

	err := routes.routeFailedRecord(context.Background(), nil, errors.New("handler failed"), libZap.InitializeLogger())
	require.Error(t, err)
	assert.ErrorContains(t, err, "record is nil")
}

func TestRouteFailedRecord_PublishFailure(t *testing.T) {
	client, err := kgo.NewClient(kgo.SeedBrokers("127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(client.Close)

	routes := &ConsumerRoutes{
		client:           client,
		maxRetryAttempts: 1,
	}

	record := &kgo.Record{
		Topic: "ledger.balance.operations",
		Key:   []byte("k"),
		Value: []byte("payload"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = routes.routeFailedRecord(ctx, record, errors.New("handler failed"), libZap.InitializeLogger())
	require.Error(t, err)
	assert.ErrorContains(t, err, "publish failed message")
}

func TestRouteFailedRecordWithRetry_StopsWhenConsumerContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	routes := &ConsumerRoutes{ctx: ctx}

	err := routes.routeFailedRecordWithRetry(context.Background(), nil, errors.New("handler failed"), libZap.InitializeLogger())
	require.Error(t, err)
	assert.ErrorContains(t, err, "consumer shutting down while rerouting failed message")
}
