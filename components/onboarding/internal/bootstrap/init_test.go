// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitServers_RedisFailure ensures that InitServers propagates a redis
// connection error without panicking, covering the non-unified branch.
func TestInitServers_RedisFailure(t *testing.T) {
	// Cannot t.Parallel() because we mutate env vars.
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("REDIS_HOST", "localhost:9999")

	require.NotPanics(t, func() {
		svc, err := InitServers()
		assert.Nil(t, svc)
		assert.Error(t, err)
	})
}

// TestInitServersWithOptions_NilOptions_RedisFailure drives the nil-options
// branch where the function must still reach the redis dial step.
func TestInitServersWithOptions_NilOptions_RedisFailure(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("REDIS_HOST", "localhost:9999")

	require.NotPanics(t, func() {
		svc, err := InitServersWithOptions(nil)
		assert.Nil(t, svc)
		assert.Error(t, err)
	})
}

// TestInitServersWithOptions_CustomLoggerRedisFailure exercises the
// opts.Logger branch in resolveLogger plus the redis failure path.
func TestInitServersWithOptions_CustomLoggerRedisFailure(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("REDIS_HOST", "localhost:9999")

	require.NotPanics(t, func() {
		svc, err := InitServersWithOptions(&Options{Logger: noopLogger{}})
		assert.Nil(t, svc)
		assert.Error(t, err)
	})
}
