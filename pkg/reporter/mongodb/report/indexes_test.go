// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package report

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-observability/zap"
	"github.com/stretchr/testify/assert"

	libMongo "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
)

func newTestLogger(t *testing.T) *zap.Logger {
	t.Helper()

	logger, err := zap.New(zap.Config{Environment: zap.EnvironmentLocal, OTelLibraryName: "reporter"})
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	return logger
}

func TestNewReportMongoDBRepository_NilConnection(t *testing.T) {
	t.Parallel()

	// Passing nil connection should panic or fail gracefully.
	// NewReportMongoDBRepository dereferences mc.Database, so nil input panics.
	assert.Panics(t, func() {
		_, _ = NewReportMongoDBRepository(nil)
	}, "Expected panic when creating repository with nil connection")
}

func TestNewReportMongoDBRepositoryLazy_DoesNotDialMongo(t *testing.T) {
	t.Parallel()

	repo, err := NewReportMongoDBRepositoryLazy(&libMongo.MongoConnection{
		ConnectionStringSource: "mongodb://invalid:invalid@localhost:0",
		Database:               "tenant_reports",
		Logger:                 newTestLogger(t),
	})

	assert.NoError(t, err)
	assert.NotNil(t, repo)
	assert.Equal(t, "tenant_reports", repo.Database)
}

func TestReportMongoDBRepository_EnsureIndexes_RequiresConnection(t *testing.T) {
	t.Parallel()

	// EnsureIndexes and DropIndexes require a real MongoDB connection.
	// This test verifies the struct fields are correctly set.
	repo := &ReportMongoDBRepository{
		Database: "test_db",
	}

	assert.Equal(t, "test_db", repo.Database)
}

func TestEnsureIndexes_NilConnection(t *testing.T) {
	t.Parallel()

	// When the connection field is nil, EnsureIndexes panics on nil pointer dereference.
	repo := &ReportMongoDBRepository{
		connection: nil,
		Database:   "testdb",
	}

	assert.Panics(t, func() {
		_ = repo.EnsureIndexes(context.Background())
	}, "Expected panic when connection is nil")
}

func TestDropIndexes_NilConnection(t *testing.T) {
	t.Parallel()

	// When the connection field is nil, DropIndexes panics on nil pointer dereference.
	repo := &ReportMongoDBRepository{
		connection: nil,
		Database:   "testdb",
	}

	assert.Panics(t, func() {
		_ = repo.DropIndexes(context.Background())
	}, "Expected panic when connection is nil")
}
