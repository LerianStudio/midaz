package rabbitmq

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

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

	healthCheckFn := NewRabbitMQHealthCheckFunc(mockConn)

	ctx := context.Background()
	err := healthCheckFn(ctx)

	// Should return nil when broker is healthy
	assert.NoError(t, err)
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
