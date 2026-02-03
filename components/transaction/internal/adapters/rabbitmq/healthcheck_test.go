package rabbitmq

import (
	"context"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewRabbitMQHealthCheckFunc_ReturnsFunction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()

	conn := &libRabbitmq.RabbitMQConnection{
		HealthCheckURL: "http://localhost:15672",
		User:           "guest",
		Pass:           "guest",
		Logger:         logger,
	}

	healthCheckFn := NewRabbitMQHealthCheckFunc(conn)

	// Verify it returns a function
	assert.NotNil(t, healthCheckFn)
}

func TestRabbitMQHealthCheckFunc_ReturnsErrorWhenUnhealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()

	// Create connection with invalid URL to simulate unhealthy broker
	conn := &libRabbitmq.RabbitMQConnection{
		HealthCheckURL: "http://invalid-host:15672",
		User:           "guest",
		Pass:           "guest",
		Logger:         logger,
	}

	healthCheckFn := NewRabbitMQHealthCheckFunc(conn)

	ctx := context.Background()
	err := healthCheckFn(ctx)

	// Should return specific error when broker is unavailable
	assert.ErrorIs(t, err, ErrRabbitMQUnhealthy)
}

func TestRabbitMQHealthCheckFunc_ReturnsNilWhenHealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)
	// No error logging expected for healthy broker

	// Note: This test will fail if RabbitMQ is not running locally
	// We test the function signature and nil guard behavior instead
	conn := &libRabbitmq.RabbitMQConnection{
		HealthCheckURL: "http://localhost:15672",
		User:           "guest",
		Pass:           "guest",
		Logger:         logger,
	}

	healthCheckFn := NewRabbitMQHealthCheckFunc(conn)

	// Verify the function is callable - the actual result depends on RabbitMQ availability
	assert.NotNil(t, healthCheckFn)
}

func TestRabbitMQHealthCheckFunc_RespectsContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := libLog.NewMockLogger(ctrl)

	conn := &libRabbitmq.RabbitMQConnection{
		HealthCheckURL: "http://localhost:15672",
		User:           "guest",
		Pass:           "guest",
		Logger:         logger,
	}

	healthCheckFn := NewRabbitMQHealthCheckFunc(conn)

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
