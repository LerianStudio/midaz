package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
	"github.com/stretchr/testify/assert"
)

// Mock implementations for testing
// These embed the interface to satisfy nil checks without behavior.
type mockOutboxRepo struct {
	outbox.Repository
}

type mockMetadataRepo struct {
	mongodb.Repository
}

func TestNewMetadataOutboxWorker_PanicsOnNilLogger(t *testing.T) {
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(nil, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 7)
	}, "Expected panic on nil Logger")
}

func TestNewMetadataOutboxWorker_PanicsOnNilOutboxRepo(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(mockLogger, nil, mockMetadata, mockPostgres, mockMongo, 5, 7)
	}, "Expected panic on nil OutboxRepository")
}

func TestNewMetadataOutboxWorker_PanicsOnNilMetadataRepo(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(mockLogger, mockOutbox, nil, mockPostgres, mockMongo, 5, 7)
	}, "Expected panic on nil MetadataRepository")
}

func TestNewMetadataOutboxWorker_PanicsOnNilPostgresConn(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockMongo := &libMongo.MongoConnection{}

	assert.Panics(t, func() {
		NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, nil, mockMongo, 5, 7)
	}, "Expected panic on nil PostgresConnection")
}

func TestNewMetadataOutboxWorker_SucceedsWithValidDependencies(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	assert.NotPanics(t, func() {
		worker := NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 7)
		assert.NotNil(t, worker)
		assert.Equal(t, 5, worker.maxWorkers)
		assert.Equal(t, 7, worker.retentionDays)
	})
}

func TestNewMetadataOutboxWorker_DefaultsMaxWorkersWhenZero(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	worker := NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 0, 7)
	assert.Equal(t, 5, worker.maxWorkers)
}

func TestNewMetadataOutboxWorker_DefaultsRetentionDaysWhenZero(t *testing.T) {
	mockLogger := &libLog.NoneLogger{}
	mockOutbox := &mockOutboxRepo{}
	mockMetadata := &mockMetadataRepo{}
	mockPostgres := &libPostgres.PostgresConnection{}
	mockMongo := &libMongo.MongoConnection{}

	worker := NewMetadataOutboxWorker(mockLogger, mockOutbox, mockMetadata, mockPostgres, mockMongo, 5, 0)
	assert.Equal(t, 7, worker.retentionDays)
}
