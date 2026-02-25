// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

type indexedBatchFailure struct {
	failed []int
}

func (e indexedBatchFailure) Error() string {
	return "batch failure"
}

func (e indexedBatchFailure) FailedRecordIndexes() []int {
	return e.failed
}

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

func TestConsumerRoutes_SetDBRateLimiter_NilReceiverDoesNotPanic(t *testing.T) {
	var routes *ConsumerRoutes

	assert.NotPanics(t, func() {
		routes.SetDBRateLimiter(100, 100)
	})
}

func TestConsumerRoutes_SetDBRateLimiter_DisableLimiter(t *testing.T) {
	routes := &ConsumerRoutes{}
	routes.SetDBRateLimiter(100, 100)
	require.NotNil(t, routes.dbLimiter)

	routes.SetDBRateLimiter(0, 0)
	assert.Nil(t, routes.dbLimiter)
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

func TestResolveBatchHandler(t *testing.T) {
	routes := &ConsumerRoutes{
		batchRoutes: map[string]BatchQueueHandlerFunc{
			"ledger.balance.operations": func(_ context.Context, _ [][]byte) error { return nil },
		},
	}

	_, ok := routes.resolveBatchHandler("ledger.balance.operations")
	assert.True(t, ok)

	_, ok = routes.resolveBatchHandler("ledger.balance.operations.retry")
	assert.True(t, ok)

	_, ok = routes.resolveBatchHandler("ledger.balance.unknown")
	assert.False(t, ok)
}

func TestConsumerRoutes_SetBatchConfigAndWorkersPerPartition(t *testing.T) {
	routes := NewConsumerRoutesWithSecurity(
		[]string{"127.0.0.1:9092"},
		"group",
		16,
		0,
		libZap.InitializeLogger(),
		nil,
		ClientSecurityConfig{},
		defaultMaxRetryAttempts,
	)

	routes.SetBatchConfig(true, 64, 20*time.Millisecond, 200*time.Millisecond)
	assert.True(t, routes.batchEnabled)
	assert.Equal(t, 64, routes.batchSize)
	assert.Equal(t, 20, int(routes.batchWindow/time.Millisecond))
	assert.Equal(t, 200, int(routes.idleFlush/time.Millisecond))
	assert.True(t, routes.batchImmediateCommit)

	routes.SetBatchImmediateCommit(false)
	assert.False(t, routes.batchImmediateCommit)

	routes.SetPartitionWorkerHint(8)
	assert.Equal(t, 8, routes.partitionHint)

	routes.SetPartitionWorkerHint(0)
	assert.Equal(t, 8, routes.partitionHint) // 0 is ignored, retains previous value
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

func TestStartWorker_FlushesBatchOnBatchSize(t *testing.T) {
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
	routes.SetBatchConfig(true, 2, time.Second, time.Second)

	workCh := make(chan queuedRecord, 2)

	var (
		mu         sync.Mutex
		batchSizes []int
	)

	batchHandler := func(_ context.Context, bodies [][]byte) error {
		mu.Lock()
		batchSizes = append(batchSizes, len(bodies))
		mu.Unlock()

		return nil
	}

	done := make(chan struct{})
	stopCh := make(chan struct{})
	go func() {
		routes.startWorker(1, workCh, stopCh)
		close(done)
	}()

	workCh <- queuedRecord{batchHandler: batchHandler, record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 1, Value: []byte("a")}}
	workCh <- queuedRecord{batchHandler: batchHandler, record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 2, Value: []byte("b")}}
	close(workCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{2}, batchSizes)
}

func TestStartWorker_FlushesBatchOnTopicChange(t *testing.T) {
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
	routes.SetBatchConfig(true, 10, time.Second, time.Second)

	workCh := make(chan queuedRecord, 2)

	var (
		mu         sync.Mutex
		batchSizes []int
	)

	batchHandler := func(_ context.Context, bodies [][]byte) error {
		mu.Lock()
		batchSizes = append(batchSizes, len(bodies))
		mu.Unlock()

		return nil
	}

	done := make(chan struct{})
	stopCh := make(chan struct{})
	go func() {
		routes.startWorker(1, workCh, stopCh)
		close(done)
	}()

	workCh <- queuedRecord{batchHandler: batchHandler, record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 1, Value: []byte("a")}}
	workCh <- queuedRecord{batchHandler: batchHandler, record: &kgo.Record{Topic: "ledger.balance.create", Partition: 0, Offset: 2, Value: []byte("b")}}
	close(workCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{1, 1}, batchSizes)
}

func TestStartWorker_FlushesBatchOnIdle(t *testing.T) {
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
	routes.SetBatchConfig(true, 10, time.Second, 20*time.Millisecond)

	workCh := make(chan queuedRecord, 1)

	var (
		mu         sync.Mutex
		batchSizes []int
	)

	batchHandler := func(_ context.Context, bodies [][]byte) error {
		mu.Lock()
		batchSizes = append(batchSizes, len(bodies))
		mu.Unlock()

		return nil
	}

	done := make(chan struct{})
	stopCh := make(chan struct{})
	go func() {
		routes.startWorker(1, workCh, stopCh)
		close(done)
	}()

	workCh <- queuedRecord{batchHandler: batchHandler, record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 1, Value: []byte("a")}}
	time.Sleep(120 * time.Millisecond)
	close(workCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{1}, batchSizes)
}

func TestStartWorker_FlushesBatchOnBatchWindowAge(t *testing.T) {
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
	routes.SetBatchConfig(true, 10, 40*time.Millisecond, time.Second)

	workCh := make(chan queuedRecord, 3)

	var (
		mu         sync.Mutex
		batchSizes []int
	)

	flushCh := make(chan int, 1)
	batchHandler := func(_ context.Context, bodies [][]byte) error {
		mu.Lock()
		batchSizes = append(batchSizes, len(bodies))
		mu.Unlock()

		flushCh <- len(bodies)

		return nil
	}

	done := make(chan struct{})
	stopCh := make(chan struct{})
	go func() {
		routes.startWorker(1, workCh, stopCh)
		close(done)
	}()

	workCh <- queuedRecord{batchHandler: batchHandler, record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 1, Value: []byte("a")}}
	time.Sleep(15 * time.Millisecond)
	workCh <- queuedRecord{batchHandler: batchHandler, record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 2, Value: []byte("b")}}

	select {
	case size := <-flushCh:
		assert.Equal(t, 2, size)
	case <-time.After(800 * time.Millisecond):
		t.Fatal("worker did not flush on batch window age")
	}

	close(workCh)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{2}, batchSizes)
}

func TestStartWorker_FlushesPendingBatchOnRebalanceStop(t *testing.T) {
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
	routes.SetBatchConfig(true, 10, time.Second, time.Second)

	// Unbuffered channel: the send blocks until the worker goroutine reads,
	// guaranteeing the record is in the internal batch before we close stopCh.
	workCh := make(chan queuedRecord)

	var (
		mu         sync.Mutex
		batchSizes []int
	)

	batchHandler := func(_ context.Context, bodies [][]byte) error {
		mu.Lock()
		batchSizes = append(batchSizes, len(bodies))
		mu.Unlock()

		return nil
	}

	done := make(chan struct{})
	stopCh := make(chan struct{})
	go func() {
		routes.startWorker(1, workCh, stopCh)
		close(done)
	}()

	workCh <- queuedRecord{batchHandler: batchHandler, record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 1, Value: []byte("a")}}
	close(stopCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after rebalance stop")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{1}, batchSizes)
}

func TestProcessBatchRecords_BatchFailureFallsBackPerRecord(t *testing.T) {
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
	routes.SetBatchConfig(true, 10, time.Second, time.Second)

	var singleCalls int

	batch := []queuedRecord{
		{
			handler: func(_ context.Context, _ []byte) error {
				singleCalls++
				return nil
			},
			batchHandler: func(_ context.Context, _ [][]byte) error {
				return errors.New("batch failed")
			},
			record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 1, Value: []byte("a")},
		},
		{
			handler: func(_ context.Context, _ []byte) error {
				singleCalls++
				return nil
			},
			batchHandler: func(_ context.Context, _ [][]byte) error {
				return errors.New("batch failed")
			},
			record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 2, Value: []byte("b")},
		},
	}

	processed := routes.processBatchRecords(1, batch)

	require.True(t, processed)
	assert.Equal(t, 2, singleCalls)
}

func TestProcessBatchRecords_BatchFailureFallsBackOnlyFailedRecordsWhenIndexed(t *testing.T) {
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
	routes.SetBatchConfig(true, 10, time.Second, time.Second)

	var singleCalls int

	batch := []queuedRecord{
		{
			handler: func(_ context.Context, _ []byte) error {
				singleCalls++
				return nil
			},
			batchHandler: func(_ context.Context, _ [][]byte) error {
				return indexedBatchFailure{failed: []int{1}}
			},
			record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 1, Value: []byte("a")},
		},
		{
			handler: func(_ context.Context, _ []byte) error {
				singleCalls++
				return nil
			},
			batchHandler: func(_ context.Context, _ [][]byte) error {
				return indexedBatchFailure{failed: []int{1}}
			},
			record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 2, Value: []byte("b")},
		},
	}

	processed := routes.processBatchRecords(1, batch)

	require.True(t, processed)
	assert.Equal(t, 1, singleCalls)
}

func TestClosePartitionPoolsForAssignments_StopsRevokedPools(t *testing.T) {
	routes := NewConsumerRoutesWithSecurity(
		[]string{"127.0.0.1:9092"},
		"group",
		4,
		0,
		libZap.InitializeLogger(),
		nil,
		ClientSecurityConfig{},
		defaultMaxRetryAttempts,
	)

	p0 := routes.getOrCreatePartitionPool(0)
	p1 := routes.getOrCreatePartitionPool(1)

	routes.closePartitionPoolsForAssignments(map[string][]int32{"topic": {1}})

	select {
	case <-p1.done:
	case <-time.After(time.Second):
		t.Fatal("expected partition 1 pool to be stopped")
	}

	select {
	case <-p0.done:
		t.Fatal("partition 0 pool should remain active")
	default:
	}

	routes.closeAllPartitionPools()
}

func TestProcessBatchRecords_EmptyBatchReturnsTrue(t *testing.T) {
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

	assert.True(t, routes.processBatchRecords(1, nil))
	assert.True(t, routes.processBatchRecords(1, []queuedRecord{}))
}

func TestResolveWorkerTickerInterval_DerivesFromBatchConfig(t *testing.T) {
	routes := &ConsumerRoutes{}
	assert.Equal(t, defaultWorkerTicker, routes.resolveWorkerTickerInterval())

	routes.batchWindow = 2 * time.Millisecond
	routes.idleFlush = 8 * time.Millisecond
	assert.Equal(t, 2*time.Millisecond, routes.resolveWorkerTickerInterval())

	routes.batchWindow = 500 * time.Microsecond
	assert.Equal(t, time.Millisecond, routes.resolveWorkerTickerInterval())
}

func TestDrainStoppedPartitionQueue_ProcessesBufferedRecords(t *testing.T) {
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

	workCh := make(chan queuedRecord, 1)
	processed := 0
	workCh <- queuedRecord{
		handler: func(_ context.Context, _ []byte) error {
			processed++
			return nil
		},
		record: &kgo.Record{Topic: "ledger.balance.operations", Partition: 0, Offset: 1, Value: []byte("a")},
	}

	require.True(t, routes.drainStoppedPartitionQueue(1, workCh))
	assert.Equal(t, 1, processed)
}
