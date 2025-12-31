package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/stretchr/testify/assert"
)

func TestNewBalanceSyncWorker_PanicsOnNilRedisConn(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockUseCase := &command.UseCase{}

	assert.Panics(t, func() {
		NewBalanceSyncWorker(nil, mockLogger, mockUseCase, 5)
	}, "Expected panic on nil Redis connection")
}

func TestNewBalanceSyncWorker_PanicsOnNilLogger(t *testing.T) {
	mockConn := &libRedis.RedisConnection{}
	mockUseCase := &command.UseCase{}

	assert.Panics(t, func() {
		NewBalanceSyncWorker(mockConn, nil, mockUseCase, 5)
	}, "Expected panic on nil Logger")
}

func TestNewBalanceSyncWorker_PanicsOnNilUseCase(t *testing.T) {
	mockConn := &libRedis.RedisConnection{}
	mockLogger := &libLog.NoneLogger{}

	assert.Panics(t, func() {
		NewBalanceSyncWorker(mockConn, mockLogger, nil, 5)
	}, "Expected panic on nil UseCase")
}

func TestNewBalanceSyncWorker_SucceedsWithValidDependencies(t *testing.T) {
	mockConn := &libRedis.RedisConnection{}
	mockLogger := &libLog.NoneLogger{}
	mockUseCase := &command.UseCase{}

	assert.NotPanics(t, func() {
		worker := NewBalanceSyncWorker(mockConn, mockLogger, mockUseCase, 5)
		assert.NotNil(t, worker)
		assert.Equal(t, 5, worker.maxWorkers)
	})
}

func TestNewBalanceSyncWorker_DefaultsMaxWorkersWhenZero(t *testing.T) {
	mockConn := &libRedis.RedisConnection{}
	mockLogger := &libLog.NoneLogger{}
	mockUseCase := &command.UseCase{}

	worker := NewBalanceSyncWorker(mockConn, mockLogger, mockUseCase, 0)
	assert.Equal(t, 5, worker.maxWorkers)
}

func TestNewBalanceSyncWorker_DefaultsMaxWorkersWhenNegative(t *testing.T) {
	mockConn := &libRedis.RedisConnection{}
	mockLogger := &libLog.NoneLogger{}
	mockUseCase := &command.UseCase{}

	worker := NewBalanceSyncWorker(mockConn, mockLogger, mockUseCase, -1)
	assert.Equal(t, 5, worker.maxWorkers)
}
