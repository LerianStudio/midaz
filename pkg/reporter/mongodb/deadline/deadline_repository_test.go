// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package deadline

import (
	"testing"

	"github.com/LerianStudio/lib-observability/zap"
	"github.com/stretchr/testify/assert"

	libMongo "github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb"
)

func testLogger() *zap.Logger {
	logger, err := zap.New(zap.Config{Environment: zap.EnvironmentLocal, OTelLibraryName: "reporter"})
	if err != nil {
		return &zap.Logger{}
	}

	return logger
}

func TestNewDeadlineMongoDBRepositoryLazy_DoesNotDialMongo(t *testing.T) {
	t.Parallel()

	repo, err := NewDeadlineMongoDBRepositoryLazy(&libMongo.MongoConnection{
		ConnectionStringSource: "mongodb://invalid:invalid@localhost:0",
		Database:               "tenant_deadlines",
		Logger:                 testLogger(),
	})

	assert.NoError(t, err)
	assert.NotNil(t, repo)
	assert.Equal(t, "tenant_deadlines", repo.Database)
}
