// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-observability/zap"
	libMongo "github.com/LerianStudio/reporter/pkg/mongodb"
	"github.com/stretchr/testify/assert"
)

func newTestLogger() *zap.Logger {
	logger, err := zap.New(zap.Config{Environment: zap.EnvironmentLocal, OTelLibraryName: "reporter"})
	if err != nil {
		return &zap.Logger{}
	}

	return logger
}

func TestNewTemplateMongoDBRepository_NilConnection(t *testing.T) {
	t.Parallel()

	// Passing nil connection should panic or fail gracefully.
	// NewTemplateMongoDBRepository dereferences mc.Database, so nil input panics.
	assert.Panics(t, func() {
		_, _ = NewTemplateMongoDBRepository(nil)
	}, "Expected panic when creating repository with nil connection")
}

func TestNewTemplateMongoDBRepositoryLazy_DoesNotDialMongo(t *testing.T) {
	t.Parallel()

	repo, err := NewTemplateMongoDBRepositoryLazy(&libMongo.MongoConnection{
		ConnectionStringSource: "mongodb://invalid:invalid@localhost:0",
		Database:               "tenant_templates",
		Logger:                 newTestLogger(),
	})

	assert.NoError(t, err)
	assert.NotNil(t, repo)
	assert.Equal(t, "tenant_templates", repo.Database)
}

func TestTemplateMongoDBRepository_EnsureIndexes_RequiresConnection(t *testing.T) {
	t.Parallel()

	// EnsureIndexes and DropIndexes require a real MongoDB connection.
	// This test verifies the struct fields are correctly set.
	repo := &TemplateMongoDBRepository{
		Database: "test_db",
	}

	assert.Equal(t, "test_db", repo.Database)
}

func TestEnsureIndexes_NilConnection(t *testing.T) {
	t.Parallel()

	// When the connection field is nil, EnsureIndexes panics on nil pointer dereference.
	repo := &TemplateMongoDBRepository{
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
	repo := &TemplateMongoDBRepository{
		connection: nil,
		Database:   "testdb",
	}

	assert.Panics(t, func() {
		_ = repo.DropIndexes(context.Background())
	}, "Expected panic when connection is nil")
}
