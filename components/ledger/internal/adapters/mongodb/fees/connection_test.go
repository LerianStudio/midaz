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
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestMongoConnection_GetDB_InjectedClientBypassesLazyInit verifies that when DB
// is set directly (test injection), GetDB returns it immediately without going
// through the lazy init path — even when ConnectionStringSource points to an
// unreachable host that would cause a real connection attempt to fail.
func TestMongoConnection_GetDB_InjectedClientBypassesLazyInit(t *testing.T) {
	t.Parallel()

	// Create a *mg.Client without connecting (mongo.NewClient does not dial).
	injected, err := mg.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:19999"))
	require.NoError(t, err, "mongo.NewClient must not fail — it does not dial")

	conn := &MongoConnection{
		// Unreachable URI — if lazy init runs instead of bypass, GetDB would fail.
		ConnectionStringSource: "mongodb://127.0.0.1:19999/?connectTimeoutMS=200&serverSelectionTimeoutMS=200",
		Database:               "testdb",
		MaxPoolSize:            1,
		DB:                     injected,
	}

	ctx := context.Background()

	got, err := conn.GetDB(ctx)
	require.NoError(t, err, "GetDB must succeed when DB is injected directly")
	assert.Same(t, injected, got, "GetDB must return the injected client without going through lazy init")
}

// TestMongoConnection_GetDB_InjectedClientWithTLSCACert verifies that when DB is
// injected directly (test path), GetDB bypasses lazy init even when TLSCACert is set.
func TestMongoConnection_GetDB_InjectedClientWithTLSCACert(t *testing.T) {
	t.Parallel()

	injected, err := mg.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:19999"))
	require.NoError(t, err)

	conn := &MongoConnection{
		ConnectionStringSource: "mongodb://127.0.0.1:19999/?connectTimeoutMS=200&serverSelectionTimeoutMS=200",
		Database:               "testdb",
		MaxPoolSize:            1,
		TLSCACert:              "dGVzdC1jYQ==", // base64 placeholder, not a real cert
		DB:                     injected,
	}

	got, err := conn.GetDB(context.Background())
	require.NoError(t, err, "GetDB must succeed when DB is injected directly, even with TLSCACert set")
	assert.Same(t, injected, got)
}

// TestMongoConnection_GetDB_SyncOncePoisoning demonstrates the sync.Once poisoning
// bug: after a failed first GetDB call, every subsequent call returns the cached error
// even though the underlying connection parameters have not changed and a retry could
// potentially succeed.
//
// This test is RED with the current sync.Once implementation and GREEN after the fix
// that memoizes only successful connections.
//
// The fix is validated at the integration level by
// TestIntegration_Chaos_MongoDB_StartupFailure_DoesNotPoisonConnection in
// tests/chaos/chaos_mongodb_integration_test.go, which uses Toxiproxy to simulate
// MongoDB being unreachable on first call and then recovering.
func TestMongoConnection_GetDB_SyncOncePoisoning(t *testing.T) {
	t.Parallel()

	conn := &MongoConnection{
		ConnectionStringSource: "mongodb://127.0.0.1:19999/?connectTimeoutMS=200&serverSelectionTimeoutMS=200",
		Database:               "testdb",
		MaxPoolSize:            1,
	}

	ctx := context.Background()

	// First call — must fail (MongoDB unreachable).
	_, err1 := conn.GetDB(ctx)
	require.Error(t, err1, "first GetDB must fail when MongoDB is unreachable")

	// With the sync.Once bug, the error is cached in c.err and c.once is marked done.
	// Every subsequent call to GetDB returns c.err directly without attempting a new
	// connection — even if the underlying issue has been resolved.
	//
	// We verify this by checking that both errors are the exact same object (same
	// interface pointer), which is only possible if the second call returned c.err
	// directly instead of calling base.NewClient again.
	_, err2 := conn.GetDB(ctx)
	require.Error(t, err2, "second GetDB must also fail")

	// After the fix: each call produces a fresh error from a new connection attempt,
	// so err1 != err2 (different objects).
	// With the bug: err2 == c.err == err1 (same cached object).
	// Use Go interface identity (==) not reflect.DeepEqual to detect whether the
	// same error object was returned. With the sync.Once bug, err2 is literally c.err
	// (same pointer), so err1 == err2 is true. After the fix, each call creates a new
	// error object from a fresh base.NewClient call, so err1 != err2.
	assert.False(t, err1 == err2,
		"SYNC.ONCE POISONING BUG: both calls returned the exact same cached error object. "+
			"A transient failure is permanently blocking all future connection attempts. "+
			"Fix: do not cache failed initialization; retry on next call.")
}
