// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"testing"
	"time"

	mmongoDB "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mg "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestMongoReadinessChecker_NilReceiver verifies the defensive nil-receiver
// guard returns an explanatory error rather than panicking. This codifies
// behavior for miswired bootstrap paths.
func TestMongoReadinessChecker_NilReceiver(t *testing.T) {
	t.Parallel()

	var m *MongoReadinessChecker

	err := m.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection not configured")
}

// TestMongoReadinessChecker_NilConnection verifies the defensive nil-connection
// guard returns an explanatory error rather than panicking. This protects
// against a checker constructed without wiring the underlying MongoConnection.
func TestMongoReadinessChecker_NilConnection(t *testing.T) {
	t.Parallel()

	checker := &MongoReadinessChecker{connection: nil}

	err := checker.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection not configured")
}

// TestPing_RealRoundTrip_DownstreamUnreachable confirms that Ping issues a
// real round-trip after retrieving the cached client. With the legacy
// implementation (GetDB only), an unreachable Mongo would still return success
// because the client pointer is cached. With the enhanced implementation,
// client.Ping fires a request to the primary and surfaces network failures.
//
// Strategy: inject a *mg.Client that points at an unreachable host with very
// short server-selection and connect timeouts. GetDB returns it instantly
// (DB-injection bypass); the subsequent client.Ping must fail.
func TestPing_RealRoundTrip_DownstreamUnreachable(t *testing.T) {
	t.Parallel()

	// Tight timeouts so the test stays fast even when ServerSelection runs.
	clientOpts := options.Client().
		ApplyURI("mongodb://127.0.0.1:1/?connectTimeoutMS=200&serverSelectionTimeoutMS=200").
		SetServerSelectionTimeout(200 * time.Millisecond).
		SetConnectTimeout(200 * time.Millisecond)

	client, err := mg.Connect(clientOpts)
	require.NoError(t, err)

	conn := &mmongoDB.MongoConnection{
		ConnectionStringSource: "mongodb://127.0.0.1:1/",
		Database:               "testdb",
		MaxPoolSize:            1,
		DB:                     client,
	}

	checker := NewMongoReadinessChecker(conn)

	// Bound the test on the outside so a regressed implementation cannot
	// hang indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pingErr := checker.Ping(ctx)
	require.Error(t, pingErr,
		"Ping must surface a real round-trip error when MongoDB is unreachable; "+
			"a legacy GetDB-only implementation would silently succeed here")
}

// TestPing_NilClient_SurfacesError ensures that when the Mongo driver returns a
// nil client (possible if a future refactor leaks bad state), Ping reports an
// error rather than panicking on the dereference.
func TestPing_NilClient_SurfacesError(t *testing.T) {
	t.Parallel()

	conn := &mmongoDB.MongoConnection{
		// Unset DB — falls through to lazy init against an unreachable URI.
		ConnectionStringSource: "mongodb://127.0.0.1:1/?connectTimeoutMS=200&serverSelectionTimeoutMS=200",
		Database:               "testdb",
		MaxPoolSize:            1,
	}

	checker := NewMongoReadinessChecker(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := checker.Ping(ctx)
	require.Error(t, err)
}
