package rabbitmq

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

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
	mockConn.EXPECT().EnsureChannel().Return(nil)

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
	mockConn.EXPECT().EnsureChannel().Return(errors.New("channel closed"))

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
	return m.healthStatusMap
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

func TestStateAwareHealthChecker_StartsWhenCircuitOpens(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
	require.NoError(t, err)

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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
	require.NoError(t, err)

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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
	require.NoError(t, err)

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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
	require.NoError(t, err)

	status := stateAware.GetHealthStatus()

	assert.Equal(t, "open", status["test-service"])
}

func TestStateAwareHealthChecker_StopWhenRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHC := newMockHealthChecker()
	logger := setupTestLogger(ctrl)

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
	require.NoError(t, err)

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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
	require.NoError(t, err)

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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
	require.NoError(t, err)

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

	stateAware, err := NewStateAwareHealthChecker(nil, logger)

	assert.Nil(t, stateAware)
	assert.ErrorIs(t, err, ErrNilHealthChecker)
}

func TestNewStateAwareHealthChecker_ReturnsErrorOnNilLogger(t *testing.T) {
	mockHC := newMockHealthChecker()

	stateAware, err := NewStateAwareHealthChecker(mockHC, nil)

	assert.Nil(t, stateAware)
	assert.ErrorIs(t, err, ErrNilHealthCheckerLogger)
}

func TestNewStateAwareHealthChecker_ReturnsErrorOnBothNil(t *testing.T) {
	stateAware, err := NewStateAwareHealthChecker(nil, nil)

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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
	require.NoError(t, err)

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

	stateAware, err := NewStateAwareHealthChecker(mockHC, logger)
	require.NoError(t, err)

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
