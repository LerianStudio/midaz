// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newTestManager(t *testing.T, logger libLog.Logger) libCircuitBreaker.Manager {
	t.Helper()

	return newTestManagerWithConfig(t, logger, CircuitBreakerConfig{
		ConsecutiveFailures: 1,
		FailureRatio:        0.5,
		Interval:            time.Minute,
		MaxRequests:         1,
		MinRequests:         1,
		Timeout:             2 * time.Second,
	})
}

func newTestManagerWithConfig(t *testing.T, logger libLog.Logger, cfg CircuitBreakerConfig) libCircuitBreaker.Manager {
	t.Helper()

	manager := libCircuitBreaker.NewManager(logger)
	manager.GetOrCreate(CircuitBreakerServiceName, ProducerCircuitBreakerConfig(cfg))

	return manager
}

func TestNewCircuitBreakerProducer_ValidatesInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libZap.InitializeLogger()
	manager := newTestManager(t, logger)
	underlying := NewMockProducerRepository(ctrl)

	producer, err := NewCircuitBreakerProducer(nil, manager, logger, time.Second)
	assert.Nil(t, producer)
	assert.ErrorIs(t, err, ErrNilUnderlying)

	producer, err = NewCircuitBreakerProducer(underlying, nil, logger, time.Second)
	assert.Nil(t, producer)
	assert.ErrorIs(t, err, ErrNilCBManager)

	producer, err = NewCircuitBreakerProducer(underlying, manager, nil, time.Second)
	assert.Nil(t, producer)
	assert.ErrorIs(t, err, ErrNilCBLogger)
}

func TestCircuitBreakerProducer_DefaultTimeoutAndDelegation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libZap.InitializeLogger()
	manager := newTestManager(t, logger)
	underlying := NewMockProducerRepository(ctrl)

	producer, err := NewCircuitBreakerProducer(underlying, manager, logger, 0)
	require.NoError(t, err)
	assert.Equal(t, DefaultOperationTimeout, producer.operationTimeout)

	underlying.EXPECT().ProducerDefault(gomock.Any(), "topic", "key", []byte("payload")).Return(nil, nil).Times(1)

	_, err = producer.ProducerDefault(context.Background(), "topic", "key", []byte("payload"))
	assert.NoError(t, err)
}

func TestNewCircuitBreakerProducer_ClampsMaxTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libZap.InitializeLogger()
	manager := newTestManager(t, logger)
	underlying := NewMockProducerRepository(ctrl)

	producer, err := NewCircuitBreakerProducer(underlying, manager, logger, MaxOperationTimeout+time.Second)
	require.NoError(t, err)
	assert.Equal(t, MaxOperationTimeout, producer.operationTimeout)
}

func TestCircuitBreakerProducer_ReturnsServiceUnavailableWhenOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libZap.InitializeLogger()
	manager := newTestManager(t, logger)
	underlying := NewMockProducerRepository(ctrl)

	producer, err := NewCircuitBreakerProducer(underlying, manager, logger, time.Second)
	require.NoError(t, err)

	underlying.EXPECT().ProducerDefault(gomock.Any(), "topic", "key", []byte("payload")).Return(nil, errors.New("broker down")).AnyTimes()

	_, firstErr := producer.ProducerDefault(context.Background(), "topic", "key", []byte("payload"))
	assert.Error(t, firstErr)
	_, err = producer.ProducerDefault(context.Background(), "topic", "key", []byte("payload"))

	assert.ErrorIs(t, err, ErrServiceUnavailable)
	assert.Equal(t, libCircuitBreaker.StateOpen, producer.GetCircuitState())
}

func TestCircuitBreakerProducer_ProducerDefaultWithContext_UsesScopedTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libZap.InitializeLogger()
	manager := newTestManager(t, logger)
	underlying := NewMockProducerRepository(ctrl)

	producer, err := NewCircuitBreakerProducer(underlying, manager, logger, 100*time.Millisecond)
	require.NoError(t, err)

	underlying.EXPECT().ProducerDefaultWithContext(gomock.Any(), "topic", "key", []byte("payload")).DoAndReturn(
		func(ctx context.Context, _, _ string, _ []byte) (*string, error) {
			deadline, hasDeadline := ctx.Deadline()
			assert.True(t, hasDeadline)
			assert.LessOrEqual(t, time.Until(deadline), 200*time.Millisecond)

			return nil, nil
		},
	)

	_, err = producer.ProducerDefaultWithContext(context.Background(), "topic", "key", []byte("payload"))
	assert.NoError(t, err)
}

func TestCircuitBreakerProducer_ProducerDefaultWithContext_ServiceUnavailableWhenOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libZap.InitializeLogger()
	manager := newTestManager(t, logger)
	underlying := NewMockProducerRepository(ctrl)

	producer, err := NewCircuitBreakerProducer(underlying, manager, logger, time.Second)
	require.NoError(t, err)

	underlying.EXPECT().ProducerDefaultWithContext(gomock.Any(), "topic", "key", []byte("payload")).Return(nil, errors.New("broker down")).AnyTimes()

	_, firstErr := producer.ProducerDefaultWithContext(context.Background(), "topic", "key", []byte("payload"))
	assert.Error(t, firstErr)
	_, err = producer.ProducerDefaultWithContext(context.Background(), "topic", "key", []byte("payload"))

	assert.ErrorIs(t, err, ErrServiceUnavailable)
	assert.Equal(t, libCircuitBreaker.StateOpen, producer.GetCircuitState())
}

func TestCircuitBreakerProducer_DelegatesHealthAndClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libZap.InitializeLogger()
	manager := newTestManager(t, logger)
	underlying := NewMockProducerRepository(ctrl)

	producer, err := NewCircuitBreakerProducer(underlying, manager, logger, time.Second)
	require.NoError(t, err)

	underlying.EXPECT().CheckHealth().Return(true)
	assert.True(t, producer.CheckHealth())

	underlying.EXPECT().Close().Return(nil)
	assert.NoError(t, producer.Close())

	_ = producer.IsCircuitHealthy()
	_ = producer.GetCounts()
}

func TestCircuitBreakerProducer_Close_NilReceiver(t *testing.T) {
	var producer *CircuitBreakerProducer
	assert.NoError(t, producer.Close())
}

func TestCircuitBreakerProducer_NilReceiverGuards(t *testing.T) {
	var producer *CircuitBreakerProducer

	_, err := producer.ProducerDefault(context.Background(), "topic", "key", []byte("payload"))
	assert.ErrorIs(t, err, ErrInternalProducerError)

	assert.False(t, producer.CheckHealth())
	assert.False(t, producer.IsCircuitHealthy())
	assert.Equal(t, libCircuitBreaker.StateOpen, producer.GetCircuitState())
	assert.Equal(t, libCircuitBreaker.Counts{}, producer.GetCounts())
}

func TestCircuitBreakerProducer_TransitionsToHalfOpenAndRecovers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libZap.InitializeLogger()
	cfg := CircuitBreakerConfig{
		ConsecutiveFailures: 1,
		FailureRatio:        0,
		Interval:            time.Minute,
		MaxRequests:         1,
		MinRequests:         1,
		Timeout:             50 * time.Millisecond,
	}
	manager := newTestManagerWithConfig(t, logger, cfg)
	underlying := NewMockProducerRepository(ctrl)

	producer, err := NewCircuitBreakerProducer(underlying, manager, logger, time.Second)
	require.NoError(t, err)

	gomock.InOrder(
		underlying.EXPECT().ProducerDefaultWithContext(gomock.Any(), "topic", "key", []byte("payload")).Return(nil, errors.New("broker down")),
		underlying.EXPECT().ProducerDefaultWithContext(gomock.Any(), "topic", "key", []byte("payload")).Return(nil, nil),
	)

	_, err = producer.ProducerDefaultWithContext(context.Background(), "topic", "key", []byte("payload"))
	assert.ErrorIs(t, err, ErrServiceUnavailable)
	require.Eventually(t, func() bool {
		return producer.GetCircuitState() == libCircuitBreaker.StateOpen
	}, time.Second, 10*time.Millisecond)

	time.Sleep(cfg.Timeout + 20*time.Millisecond)

	_, err = producer.ProducerDefaultWithContext(context.Background(), "topic", "key", []byte("payload"))
	assert.NoError(t, err)
	require.Eventually(t, func() bool {
		return producer.GetCircuitState() == libCircuitBreaker.StateClosed
	}, time.Second, 10*time.Millisecond)
}

func TestCircuitBreakerProducer_ProducerDefaultWithContext_RespectsCancelledContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libZap.InitializeLogger()
	manager := newTestManagerWithConfig(t, logger, CircuitBreakerConfig{
		ConsecutiveFailures: 5,
		FailureRatio:        0,
		Interval:            time.Minute,
		MaxRequests:         1,
		MinRequests:         1,
		Timeout:             time.Second,
	})
	underlying := NewMockProducerRepository(ctrl)

	producer, err := NewCircuitBreakerProducer(underlying, manager, logger, time.Second)
	require.NoError(t, err)

	underlying.EXPECT().ProducerDefaultWithContext(gomock.Any(), "topic", "key", []byte("payload")).DoAndReturn(
		func(ctx context.Context, _, _ string, _ []byte) (*string, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	).Times(1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = producer.ProducerDefaultWithContext(ctx, "topic", "key", []byte("payload"))
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCircuitBreakerProducer_ConcurrentCallsRemainSafe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libZap.InitializeLogger()
	manager := newTestManagerWithConfig(t, logger, CircuitBreakerConfig{
		ConsecutiveFailures: 3,
		FailureRatio:        0,
		Interval:            time.Minute,
		MaxRequests:         1,
		MinRequests:         1,
		Timeout:             200 * time.Millisecond,
	})
	underlying := NewMockProducerRepository(ctrl)

	producer, err := NewCircuitBreakerProducer(underlying, manager, logger, time.Second)
	require.NoError(t, err)

	underlying.EXPECT().ProducerDefaultWithContext(gomock.Any(), "topic", "key", []byte("payload")).Return(nil, errors.New("broker down")).AnyTimes()

	const workers = 32
	errCh := make(chan error, workers)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, callErr := producer.ProducerDefaultWithContext(context.Background(), "topic", "key", []byte("payload"))
			errCh <- callErr
		}()
	}

	wg.Wait()
	close(errCh)

	for callErr := range errCh {
		require.Error(t, callErr)
	}
}
