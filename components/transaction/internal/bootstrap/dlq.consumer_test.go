package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/stretchr/testify/assert"
)

func TestNewDLQConsumer_PanicsOnNilLogger(t *testing.T) {
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockRedis := &libRedis.RedisConnection{}
	queueNames := []string{"test-queue"}

	assert.Panics(t, func() {
		NewDLQConsumer(nil, mockRabbitMQ, mockPostgres, mockRedis, queueNames)
	}, "Expected panic on nil Logger")
}

func TestNewDLQConsumer_PanicsOnNilRabbitMQConn(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockRedis := &libRedis.RedisConnection{}
	queueNames := []string{"test-queue"}

	assert.Panics(t, func() {
		NewDLQConsumer(mockLogger, nil, mockPostgres, mockRedis, queueNames)
	}, "Expected panic on nil RabbitMQConnection")
}

func TestNewDLQConsumer_PanicsOnNoInfrastructureConnections(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	queueNames := []string{"test-queue"}

	assert.Panics(t, func() {
		NewDLQConsumer(mockLogger, mockRabbitMQ, nil, nil, queueNames)
	}, "Expected panic when no infrastructure connections provided")
}

func TestNewDLQConsumer_SucceedsWithPostgresOnly(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	mockPostgres := &libPostgres.PostgresConnection{}
	queueNames := []string{"test-queue"}

	assert.NotPanics(t, func() {
		consumer := NewDLQConsumer(mockLogger, mockRabbitMQ, mockPostgres, nil, queueNames)
		assert.NotNil(t, consumer)
	})
}

func TestNewDLQConsumer_SucceedsWithRedisOnly(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	mockRedis := &libRedis.RedisConnection{}
	queueNames := []string{"test-queue"}

	assert.NotPanics(t, func() {
		consumer := NewDLQConsumer(mockLogger, mockRabbitMQ, nil, mockRedis, queueNames)
		assert.NotNil(t, consumer)
	})
}

func TestNewDLQConsumer_SucceedsWithBothConnections(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockRedis := &libRedis.RedisConnection{}
	queueNames := []string{"test-queue"}

	assert.NotPanics(t, func() {
		consumer := NewDLQConsumer(mockLogger, mockRabbitMQ, mockPostgres, mockRedis, queueNames)
		assert.NotNil(t, consumer)
		assert.Equal(t, 1, len(consumer.QueueNames))
	})
}

func TestNewDLQConsumer_WarnsOnEmptyQueueNames(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockRabbitMQ := &libRabbitmq.RabbitMQConnection{}
	mockPostgres := &libPostgres.PostgresConnection{}
	queueNames := []string{}

	assert.NotPanics(t, func() {
		consumer := NewDLQConsumer(mockLogger, mockRabbitMQ, mockPostgres, nil, queueNames)
		assert.NotNil(t, consumer)
		assert.Equal(t, 0, len(consumer.QueueNames))
	})
}
