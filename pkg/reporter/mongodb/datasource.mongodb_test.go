// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mg "go.mongodb.org/mongo-driver/v2/mongo"
)

// ---------------------------------------------------------------------------
// Ping — lightweight connectivity probe used by the HealthChecker.
// Replaces the previous "lightweight" GetDatabaseSchema-as-ping that actually
// performed full collection-by-collection schema inference. The new Ping
// delegates to *mongo.Client.Ping which issues a single
// db.runCommand({ping:1}) — the canonical lightweight reachability probe.
// ---------------------------------------------------------------------------

func TestExternalDataSource_Ping_NilReceiver_ReturnsError(t *testing.T) {
	t.Parallel()

	var ds *ExternalDataSource

	err := ds.Ping(context.Background())
	require.Error(t, err, "nil receiver must not panic and must return error")
	assert.Contains(t, err.Error(), "mongodb connection not initialized")
}

func TestExternalDataSource_Ping_NilConnection_ReturnsError(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{connection: nil}

	err := ds.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mongodb connection not initialized")
}

// TestExternalDataSource_Ping_DelegatesToMongoClient verifies that Ping
// delegates to the underlying *mongo.Client. With a freshly-constructed
// (un-connected) client, Ping must surface an error rather than silently
// succeed — pinning the contract that the wrapper does not mask
// connectivity failures.
func TestExternalDataSource_Ping_DelegatesToMongoClient(t *testing.T) {
	t.Parallel()

	client, err := mg.Connect()
	require.NoError(t, err)

	ds := &ExternalDataSource{connection: &MongoConnection{DB: client}}

	pingErr := ds.Ping(context.Background())
	require.Error(t, pingErr, "un-connected client must yield error")
}
