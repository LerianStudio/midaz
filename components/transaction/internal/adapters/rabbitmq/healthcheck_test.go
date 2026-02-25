// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.
package rabbitmq

import (
	"context"
	"errors"
	"maps"
	"runtime"
	"sync"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v3/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
)

// TestMain verifies no goroutine leaks across all tests in this package.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// =============================================================================
// RabbitMQ Health Check Function Tests
// =============================================================================

func TestNewRabbitMQHealthCheckFunc_ReturnsFunction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := NewMockRabbitMQHealthChecker(ctrl)

	healthCheckFn := NewRabbitMQHealthCheckFunc(mockConn)

	// Verify it returns a function
	assert.NotNil(t, healthCheckFn)
}

func TestRabbitMQHealthCheckFunc_ReturnsErrorWhenUnhealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := NewMockRabbitMQHealthChecker(ctrl)
	mockConn.EXPECT().HealthCheck().Return(false)

	healthCheckFn := NewRabbitMQHealthCheckFunc(mockConn)

	ctx := context.Background()
	err := healthCheckFn(ctx)

	// Should return specific error when broker is unavailable
	assert.ErrorIs(t, err, ErrRabbitMQUnhealthy)
}

func TestRabbitMQHealthCheckFunc_ReturnsNilWhenHealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := NewMockRabbitMQHealthChecker(ctrl)
	mockConn.EXPECT().HealthCheck().Return(true)
	mockConn.EXPECT().EnsureChannelWithContext(gomock.Any()).Return(nil)

	healthCheckFn := NewRabbitMQHealthCheckFunc(mockConn)

	ctx := context.Background()
	err := healthCheckFn(ctx)

	// Should return nil when both health check and channel are healthy
	assert.NoError(t, err)
}

func TestRabbitMQHealthCheckFunc_ReturnsErrorWhenChannelUnavailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := NewMockRabbitMQHealthChecker(ctrl)
	mockConn.EXPECT().HealthCheck().Return(true)
	mockConn.EXPECT().EnsureChannelWithContext(gomock.Any()).Return(errors.New("channel closed"))

	healthCheckFn := NewRabbitMQHealthCheckFunc(mockConn)

	ctx := context.Background()
	err := healthCheckFn(ctx)

	// Should return channel unavailable error
	assert.ErrorIs(t, err, ErrRabbitMQChannelUnavailable)
}

func TestRabbitMQHealthCheckFunc_RespectsContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := NewMockRabbitMQHealthChecker(ctrl)
	// No expectation on HealthCheck since context is cancelled before it's called

	healthCheckFn := NewRabbitMQHealthCheckFunc(mockConn)

	// Create already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := healthCheckFn(ctx)

	// Should return context.Canceled error
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRabbitMQHealthCheckFunc_HandlesNilConnection(t *testing.T) {
	healthCheckFn := NewRabbitMQHealthCheckFunc(nil)

	ctx := context.Background()
	err := healthCheckFn(ctx)

	// Should return ErrRabbitMQUnhealthy for nil connection
	assert.ErrorIs(t, err, ErrRabbitMQUnhealthy)
}

func TestRabbitMQHealthCheckFunc_RespectsContextDeadlineExceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := NewMockRabbitMQHealthChecker(ctrl)
	// No expectation on HealthCheck since context deadline is exceeded before it's called

	healthCheckFn := NewRabbitMQHealthCheckFunc(mockConn)

	// Create context with very short deadline that's already expired
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond) // Let deadline expire

	err := healthCheckFn(ctx)

	// Should return context.DeadlineExceeded error
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// =============================================================================
// StateAwareHealthChecker Tests
// =============================================================================

// mockHealthChecker implements libCircuitBreaker.HealthChecker for testing.
type mockHealthChecker struct {
	mu              sync.Mutex
	started         bool
	stopped         bool
	startCount      int
	stopCount       int
	registered      map[string]libCircuitBreaker.HealthCheckFunc
	stateChanges    []stateChangeRecord
	healthStatusMap map[string]string
}

type stateChangeRecord struct {
	serviceName string
	from        libCircuitBreaker.State
	to          libCircuitBreaker.State
}

func newMockHealthChecker() *mockHealthChecker {
	return &mockHealthChecker{
		registered:      make(map[string]libCircuitBreaker.HealthCheckFunc),
		healthStatusMap: make(map[string]string),
	}
}

func (m *mockHealthChecker) Register(serviceName string, healthCheckFn libCircuitBreaker.HealthCheckFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registered[serviceName] = healthCheckFn
}

func (m *mockHealthChecker) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	m.stopped = false
	m.startCount++
}

func (m *mockHealthChecker) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	m.started = false
	m.stopCount++
}

func (m *mockHealthChecker) GetHealthStatus() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy to prevent data races if caller modifies the map
	result := make(map[string]string, len(m.healthStatusMap))
	maps.Copy(result, m.healthStatusMap)
	return result
}

func (m *mockHealthChecker) OnStateChange(serviceName string, from, to libCircuitBreaker.State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateChanges = append(m.stateChanges, stateChangeRecord{
		serviceName: serviceName,
		from:        from,
		to:          to,
	})
}

func (m *mockHealthChecker) isStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

func (m *mockHealthChecker) isStopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

func (m *mockHealthChecker) getStartCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startCount
}

func (m *mockHealthChecker) getStopCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopCount
}

func (m *mockHealthChecker) getStateChanges() []stateChangeRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]stateChangeRecord, len(m.stateChanges))
	copy(result, m.stateChanges)
	return result
}

func setupTestLogger(ctrl *gomock.Controller) *libLog.MockLogger {
	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Warnf(gomock.Any(), gomock.Any()).AnyTimes()
	return logger
}

// mockCircuitStateChecker implements CircuitStateChecker for testing
type mockCircuitStateChecker struct {
	mu            sync.Mutex
	healthyStatus map[string]bool
}

func newMockCircuitStateChecker() *mockCircuitStateChecker {
	return &mockCircuitStateChecker{
		healthyStatus: make(map[string]bool),
	}
}

func (m *mockCircuitStateChecker) IsHealthy(serviceName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	healthy, exists := m.healthyStatus[serviceName]
	if !exists {
		return true // default to healthy
	}
	return healthy
}

func (m *mockCircuitStateChecker) setHealthy(serviceName string, healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthyStatus[serviceName] = healthy
}

func TestStateAwareHealthChecker_StartsWhenCircuitOpens(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	// Initial Start() is a no-op
	stateAware.Start()
	assert.False(t, mockHC.isStarted(), "underlying should not start on initial Start()")
	assert.False(t, stateAware.IsRunning())

	// Simulate circuit opening: CLOSED -> OPEN
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)

	// Health checker should now be running
	assert.True(t, mockHC.isStarted(), "underlying should start when circuit opens")
	assert.True(t, stateAware.IsRunning())
	assert.Equal(t, 1, mockHC.getStartCount())
}

func TestStateAwareHealthChecker_StopsWhenAllCircuitsClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	// Open the circuit first
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning())

	// Close the circuit: OPEN -> CLOSED
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateOpen, libCircuitBreaker.StateClosed)

	// Health checker should now be stopped
	assert.True(t, mockHC.isStopped(), "underlying should stop when all circuits close")
	assert.False(t, stateAware.IsRunning())
	assert.Equal(t, 1, mockHC.getStopCount())
}

func TestStateAwareHealthChecker_KeepsRunningDuringHalfOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	// Open the circuit
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning())
	startCountAfterOpen := mockHC.getStartCount()

	// Transition to half-open: OPEN -> HALF-OPEN
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateOpen, libCircuitBreaker.StateHalfOpen)

	// Should keep running (no additional start/stop)
	assert.True(t, stateAware.IsRunning(), "should keep running during half-open")
	assert.Equal(t, startCountAfterOpen, mockHC.getStartCount(), "should not start again")
	assert.Equal(t, 0, mockHC.getStopCount(), "should not stop during half-open")
}

func TestStateAwareHealthChecker_StopsWhenHalfOpenCloses(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	// Open then half-open
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateOpen, libCircuitBreaker.StateHalfOpen)
	assert.True(t, stateAware.IsRunning())

	// Close from half-open: HALF-OPEN -> CLOSED
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateHalfOpen, libCircuitBreaker.StateClosed)

	// Health checker should stop
	assert.False(t, stateAware.IsRunning())
	assert.Equal(t, 1, mockHC.getStopCount())
}

func TestStateAwareHealthChecker_MultipleServices(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	// Open first service
	stateAware.OnStateChange("service-1", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning())
	assert.Equal(t, 1, mockHC.getStartCount())

	// Open second service (should not start again)
	stateAware.OnStateChange("service-2", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning())
	assert.Equal(t, 1, mockHC.getStartCount(), "should not start again when already running")

	// Close first service (still has service-2 open)
	stateAware.OnStateChange("service-1", libCircuitBreaker.StateOpen, libCircuitBreaker.StateClosed)
	assert.True(t, stateAware.IsRunning(), "should keep running while service-2 is open")
	assert.Equal(t, 0, mockHC.getStopCount())

	// Close second service (now all closed)
	stateAware.OnStateChange("service-2", libCircuitBreaker.StateOpen, libCircuitBreaker.StateClosed)
	assert.False(t, stateAware.IsRunning(), "should stop when all services close")
	assert.Equal(t, 1, mockHC.getStopCount())
}

func TestStateAwareHealthChecker_ForwardsStateChanges(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	// Trigger state change
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)

	// Verify it was forwarded to underlying
	changes := mockHC.getStateChanges()
	assert.Len(t, changes, 1)
	assert.Equal(t, "test-service", changes[0].serviceName)
	assert.Equal(t, libCircuitBreaker.StateClosed, changes[0].from)
	assert.Equal(t, libCircuitBreaker.StateOpen, changes[0].to)
}

func TestStateAwareHealthChecker_DelegatesRegister(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	healthFn := func(ctx context.Context) error { return nil }
	stateAware.Register("test-service", healthFn)

	assert.Contains(t, mockHC.registered, "test-service")
}

func TestStateAwareHealthChecker_DelegatesGetHealthStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	mockHC.healthStatusMap["test-service"] = "open"
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	status := stateAware.GetHealthStatus()

	assert.Equal(t, "open", status["test-service"])
}

func TestStateAwareHealthChecker_StopWhenRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	// Start by opening a circuit
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning())

	// Explicit stop
	stateAware.Stop()

	assert.False(t, stateAware.IsRunning())
	assert.Equal(t, 1, mockHC.getStopCount())
}

func TestStateAwareHealthChecker_StopWhenNotRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	// Stop when not running - should be a no-op
	stateAware.Stop()

	assert.False(t, stateAware.IsRunning())
	assert.Equal(t, 0, mockHC.getStopCount(), "should not call Stop() on underlying when not running")
}

func TestStateAwareHealthChecker_GetUnhealthyServices(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	// Initially empty
	assert.Empty(t, stateAware.GetUnhealthyServices())

	// Open a circuit
	stateAware.OnStateChange("service-1", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	unhealthy := stateAware.GetUnhealthyServices()
	assert.Len(t, unhealthy, 1)
	assert.Equal(t, libCircuitBreaker.StateOpen, unhealthy["service-1"])

	// Half-open another
	stateAware.OnStateChange("service-2", libCircuitBreaker.StateClosed, libCircuitBreaker.StateHalfOpen)
	unhealthy = stateAware.GetUnhealthyServices()
	assert.Len(t, unhealthy, 2)
	assert.Equal(t, libCircuitBreaker.StateHalfOpen, unhealthy["service-2"])

	// Close first service
	stateAware.OnStateChange("service-1", libCircuitBreaker.StateOpen, libCircuitBreaker.StateClosed)
	unhealthy = stateAware.GetUnhealthyServices()
	assert.Len(t, unhealthy, 1)
	assert.NotContains(t, unhealthy, "service-1")
}

func TestStateAwareHealthChecker_HalfOpenToOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	// Open -> Half-open -> Open (failure in half-open)
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning())

	stateAware.OnStateChange("test-service", libCircuitBreaker.StateOpen, libCircuitBreaker.StateHalfOpen)
	assert.True(t, stateAware.IsRunning())

	// Back to open (failure during half-open test)
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateHalfOpen, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning(), "should keep running when going back to open")

	unhealthy := stateAware.GetUnhealthyServices()
	assert.Equal(t, libCircuitBreaker.StateOpen, unhealthy["test-service"])
}

func TestStateAwareHealthChecker_IdempotentStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	// Multiple opens of same service should only start once
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateOpen, libCircuitBreaker.StateOpen) // Unusual but possible

	assert.Equal(t, 1, mockHC.getStartCount())
}

// =============================================================================
// NewStateAwareHealthChecker Nil Validation Tests
// =============================================================================

func TestNewStateAwareHealthChecker_ReturnsErrorOnNilUnderlying(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := setupTestLogger(ctrl)
	mockStateChecker := newMockCircuitStateChecker()

	stateAware, err := NewStateAwareHealthChecker(nil, mockStateChecker, logger)

	assert.Nil(t, stateAware)
	assert.ErrorIs(t, err, ErrNilHealthChecker)
}

func TestNewStateAwareHealthChecker_ReturnsErrorOnNilStateChecker(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, nil, logger)

	assert.Nil(t, stateAware)
	assert.ErrorIs(t, err, ErrNilCircuitBreakerManager)
}

func TestNewStateAwareHealthChecker_ReturnsErrorOnNilLogger(t *testing.T) {
	mockHC := newMockHealthChecker()
	mockStateChecker := newMockCircuitStateChecker()

	stateAware, err := NewStateAwareHealthChecker(mockHC, mockStateChecker, nil)

	assert.Nil(t, stateAware)
	assert.ErrorIs(t, err, ErrNilHealthCheckerLogger)
}

func TestNewStateAwareHealthChecker_ReturnsErrorOnAllNil(t *testing.T) {
	stateAware, err := NewStateAwareHealthChecker(nil, nil, nil)

	assert.Nil(t, stateAware)
	// Should return error for underlying first (order of validation)
	assert.ErrorIs(t, err, ErrNilHealthChecker)
}

// =============================================================================
// StateAwareHealthChecker Concurrent Access Tests
// =============================================================================

func TestStateAwareHealthChecker_ConcurrentStateChanges(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	// Launch multiple goroutines to trigger concurrent state changes
	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			serviceName := "test-service"
			for j := 0; j < numOperations; j++ {
				// Alternate between opening and closing circuits
				if j%2 == 0 {
					stateAware.OnStateChange(serviceName, libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
				} else {
					stateAware.OnStateChange(serviceName, libCircuitBreaker.StateOpen, libCircuitBreaker.StateClosed)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify no data races occurred and state is consistent
	// After all operations complete, we should be able to read state without panic
	_ = stateAware.IsRunning()
	_ = stateAware.GetUnhealthyServices()
	_ = stateAware.GetHealthStatus()
}

func TestStateAwareHealthChecker_ConcurrentMultipleServices(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	// Launch goroutines for different services
	services := []string{"service-1", "service-2", "service-3", "service-4", "service-5"}

	var wg sync.WaitGroup
	wg.Add(len(services))

	for _, svc := range services {
		go func(serviceName string) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				// Open circuit
				stateAware.OnStateChange(serviceName, libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
				// Transition to half-open
				stateAware.OnStateChange(serviceName, libCircuitBreaker.StateOpen, libCircuitBreaker.StateHalfOpen)
				// Close circuit
				stateAware.OnStateChange(serviceName, libCircuitBreaker.StateHalfOpen, libCircuitBreaker.StateClosed)
			}
		}(svc)
	}

	wg.Wait()

	// All services should be closed after all operations
	unhealthy := stateAware.GetUnhealthyServices()
	assert.Empty(t, unhealthy, "all services should be healthy after closing")
}

// =============================================================================
// Recovery Monitor Tests
// =============================================================================

func TestStateAwareHealthChecker_RecoveryMonitorDetectsReset(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	mockStateChecker := newMockCircuitStateChecker()
	logger := setupTestLogger(ctrl)

	// Service starts as unhealthy (circuit open)
	mockStateChecker.setHealthy("test-service", false)

	stateAware, err := NewStateAwareHealthChecker(mockHC, mockStateChecker, logger)
	require.NoError(t, err)

	// Open the circuit
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning())
	assert.Len(t, stateAware.GetUnhealthyServices(), 1)

	// Simulate external reset by making service healthy in mock
	// (This simulates lib-commons Reset() which doesn't trigger listeners)
	mockStateChecker.setHealthy("test-service", true)

	// Manually trigger the recovery check (normally called by ticker)
	stateAware.checkForRecoveredServices()

	// Should have detected recovery and stopped
	assert.False(t, stateAware.IsRunning(), "should stop after recovery detected")
	assert.Empty(t, stateAware.GetUnhealthyServices(), "service should be removed from unhealthy")
}

func TestStateAwareHealthChecker_RecoveryMonitorPartialRecovery(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	mockStateChecker := newMockCircuitStateChecker()
	logger := setupTestLogger(ctrl)

	// Both services start as unhealthy
	mockStateChecker.setHealthy("service-1", false)
	mockStateChecker.setHealthy("service-2", false)

	stateAware, err := NewStateAwareHealthChecker(mockHC, mockStateChecker, logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	// Open both circuits
	stateAware.OnStateChange("service-1", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	stateAware.OnStateChange("service-2", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning())
	assert.Len(t, stateAware.GetUnhealthyServices(), 2)

	// Only service-1 recovers externally
	mockStateChecker.setHealthy("service-1", true)

	// Trigger recovery check
	stateAware.checkForRecoveredServices()

	// Should still be running because service-2 is still unhealthy
	assert.True(t, stateAware.IsRunning(), "should keep running while service-2 is unhealthy")
	assert.Len(t, stateAware.GetUnhealthyServices(), 1)
	assert.NotContains(t, stateAware.GetUnhealthyServices(), "service-1")
	assert.Contains(t, stateAware.GetUnhealthyServices(), "service-2")
}

// =============================================================================
// Goroutine Cleanup Tests
// =============================================================================

func TestStateAwareHealthChecker_GoroutineCleanupOnStop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	// Get goroutine count before starting monitor
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	beforeStart := runtime.NumGoroutine()

	// Start the monitor by opening a circuit
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning(), "health checker should be running")
	time.Sleep(50 * time.Millisecond) // Let goroutine start

	// Stop
	stateAware.Stop()
	assert.False(t, stateAware.IsRunning(), "health checker should be stopped")
	time.Sleep(50 * time.Millisecond) // Let goroutine clean up

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	afterStop := runtime.NumGoroutine()

	// After stop, goroutine count should not significantly exceed what it was before start
	// Allow small variance (2) for test infrastructure
	assert.LessOrEqual(t, afterStop, beforeStart+2, "goroutine should be cleaned up after stop")
}

func TestStateAwareHealthChecker_GoroutineCleanupOnAllCircuitsClose(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	// Start by opening a circuit
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	time.Sleep(50 * time.Millisecond)

	// Close the circuit (this should stop the monitor)
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateOpen, libCircuitBreaker.StateClosed)
	time.Sleep(50 * time.Millisecond)

	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	afterClose := runtime.NumGoroutine()

	assert.LessOrEqual(t, afterClose, baseline+2, "goroutine should be cleaned up after all circuits close")
}

// =============================================================================
// Double-Stop Idempotency Tests
// =============================================================================

func TestStateAwareHealthChecker_DoubleStopIsIdempotent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	// Start by opening a circuit
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning())

	// Double stop - should not panic or cause issues
	stateAware.Stop()
	stateAware.Stop()

	assert.False(t, stateAware.IsRunning())
	assert.Equal(t, 1, mockHC.getStopCount(), "underlying Stop should only be called once")
}

func TestStateAwareHealthChecker_MultipleStopsNotRunning(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)

	// Multiple stops when not running - should be no-ops
	stateAware.Stop()
	stateAware.Stop()
	stateAware.Stop()

	assert.Equal(t, 0, mockHC.getStopCount(), "should not call Stop() on underlying when not running")
}

// =============================================================================
// Concurrent Start/Stop Race Tests
// =============================================================================

func TestStateAwareHealthChecker_ConcurrentStartStop(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	var wg sync.WaitGroup
	wg.Add(3)

	// Goroutine 1: Opens circuits
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
		}
	}()

	// Goroutine 2: Closes circuits
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			stateAware.OnStateChange("test-service", libCircuitBreaker.StateOpen, libCircuitBreaker.StateClosed)
		}
	}()

	// Goroutine 3: Calls Stop
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			stateAware.Stop()
		}
	}()

	wg.Wait()

	// Should not panic, state should be consistent
	running := stateAware.IsRunning()
	unhealthy := stateAware.GetUnhealthyServices()

	// Final state: If running, should have unhealthy services; if not running, may or may not have unhealthy
	if running {
		assert.NotEmpty(t, unhealthy, "if running, should have unhealthy services")
	}
	// The main assertion is that we didn't panic - race detector will catch data races
}

func TestStateAwareHealthChecker_ConcurrentOnStateChangeAndStop(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	// Start the health checker
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Rapid state changes
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			stateAware.OnStateChange("service-"+string(rune('a'+i%5)), libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
			stateAware.OnStateChange("service-"+string(rune('a'+i%5)), libCircuitBreaker.StateOpen, libCircuitBreaker.StateClosed)
		}
	}()

	// Goroutine 2: Calls Stop repeatedly
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			stateAware.Stop()
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()

	// Should complete without panic
	_ = stateAware.IsRunning()
}

// =============================================================================
// Error Wrapping Tests
// =============================================================================

func TestRabbitMQHealthCheckFunc_ErrorWrappingPreservesOriginalError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	originalErr := errors.New("channel closed by server")

	mockConn := NewMockRabbitMQHealthChecker(ctrl)
	mockConn.EXPECT().HealthCheck().Return(true)
	mockConn.EXPECT().EnsureChannelWithContext(gomock.Any()).Return(originalErr)

	healthCheckFn := NewRabbitMQHealthCheckFunc(mockConn)

	ctx := context.Background()
	err := healthCheckFn(ctx)

	// Should wrap both errors
	assert.ErrorIs(t, err, ErrRabbitMQChannelUnavailable, "should contain ErrRabbitMQChannelUnavailable")
	assert.ErrorIs(t, err, originalErr, "should preserve original error")
	assert.Contains(t, err.Error(), "channel closed by server", "error message should contain original error text")
}

func TestRabbitMQHealthCheckFunc_ErrorWrappingWithMultipleErrors(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create a wrapped original error
	innerErr := errors.New("connection reset")
	wrappedErr := errors.Join(errors.New("network error"), innerErr)

	mockConn := NewMockRabbitMQHealthChecker(ctrl)
	mockConn.EXPECT().HealthCheck().Return(true)
	mockConn.EXPECT().EnsureChannelWithContext(gomock.Any()).Return(wrappedErr)

	healthCheckFn := NewRabbitMQHealthCheckFunc(mockConn)

	ctx := context.Background()
	err := healthCheckFn(ctx)

	// Should contain all errors in the chain
	assert.ErrorIs(t, err, ErrRabbitMQChannelUnavailable)
	assert.ErrorIs(t, err, innerErr, "should preserve inner error")
}

// =============================================================================
// Stop During Recovery Monitor Tick Tests
// =============================================================================

func TestStateAwareHealthChecker_StopDuringRecoveryMonitorExecution(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	mockStateChecker := newMockCircuitStateChecker()
	logger := setupTestLogger(ctrl)

	// Service starts as unhealthy
	mockStateChecker.setHealthy("test-service", false)

	stateAware, err := NewStateAwareHealthChecker(mockHC, mockStateChecker, logger)
	require.NoError(t, err)

	// Open the circuit to start the monitor
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	assert.True(t, stateAware.IsRunning())

	// Give the monitor time to potentially start a tick
	time.Sleep(10 * time.Millisecond)

	// Stop while monitor may be checking
	done := make(chan struct{})
	go func() {
		stateAware.Stop()
		close(done)
	}()

	// Should complete without deadlock
	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() deadlocked")
	}

	assert.False(t, stateAware.IsRunning())
}

func TestStateAwareHealthChecker_RecoveryCheckDuringStop(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	mockStateChecker := newMockCircuitStateChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, mockStateChecker, logger)
	require.NoError(t, err)

	// Open circuit to start monitor
	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Calls checkForRecoveredServices repeatedly
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			stateAware.checkForRecoveredServices()
		}
	}()

	// Goroutine 2: Calls Stop
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond) // Let some checks run
		stateAware.Stop()
	}()

	wg.Wait()

	// Should complete without panic or deadlock
	assert.False(t, stateAware.IsRunning())
}

// =============================================================================
// Additional Parallel Test Markers
// =============================================================================

func TestNewRabbitMQHealthCheckFunc_ReturnsFunction_Parallel(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := NewMockRabbitMQHealthChecker(ctrl)

	healthCheckFn := NewRabbitMQHealthCheckFunc(mockConn)

	assert.NotNil(t, healthCheckFn)
}

func TestStateAwareHealthChecker_StartsWhenCircuitOpens_Parallel(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	stateAware.Start()
	assert.False(t, mockHC.isStarted())
	assert.False(t, stateAware.IsRunning())

	stateAware.OnStateChange("test-service", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)

	assert.True(t, mockHC.isStarted())
	assert.True(t, stateAware.IsRunning())
	assert.Equal(t, 1, mockHC.getStartCount())
}

// =============================================================================
// Test for Deterministic Final State in Concurrent Tests
// =============================================================================

func TestStateAwareHealthChecker_ConcurrentStateChanges_DeterministicFinalState(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, newMockCircuitStateChecker(), logger)
	require.NoError(t, err)
	t.Cleanup(func() { stateAware.Stop() })

	const numGoroutines = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// All goroutines will eventually close their circuits
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			serviceName := "test-service"
			// Open
			stateAware.OnStateChange(serviceName, libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
			time.Sleep(time.Millisecond)
			// Half-open
			stateAware.OnStateChange(serviceName, libCircuitBreaker.StateOpen, libCircuitBreaker.StateHalfOpen)
			time.Sleep(time.Millisecond)
			// Close
			stateAware.OnStateChange(serviceName, libCircuitBreaker.StateHalfOpen, libCircuitBreaker.StateClosed)
		}(i)
	}

	wg.Wait()

	// Give time for final state to settle
	time.Sleep(50 * time.Millisecond)

	// Final state should be deterministic: all circuits closed, health checker stopped
	running := stateAware.IsRunning()
	unhealthy := stateAware.GetUnhealthyServices()

	assert.False(t, running, "should not be running after all circuits close")
	assert.Empty(t, unhealthy, "should have no unhealthy services after all close")
}
