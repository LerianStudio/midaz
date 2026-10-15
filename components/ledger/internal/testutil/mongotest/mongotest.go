// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package mongotest provides MongoDB fixtures for ledger unit tests that exercise a
// repository's failure path without a running server.
//
// It is a test-only package: import it only from _test.go files. Production code
// MUST NOT import it.
package mongotest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// unreachableURI points at a port nothing listens on, so a connection is refused at once.
const unreachableURI = "mongodb://127.0.0.1:1"

// failFast bounds server selection so every driver call fails within the test budget.
const failFast = 50 * time.Millisecond

// UnreachableClient returns a client whose server refuses connections: creating it makes
// no network call, and every driver call through it fails fast with a server-selection
// error. The client is disconnected when the test ends, which stops its monitor goroutines.
func UnreachableClient(tb testing.TB) *mongo.Client {
	tb.Helper()

	client, err := mongo.Connect(options.Client().
		ApplyURI(unreachableURI).
		SetServerSelectionTimeout(failFast).
		SetConnectTimeout(failFast))
	require.NoError(tb, err)

	tb.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	return client
}

// UnreachableDatabase returns a database handle on UnreachableClient.
func UnreachableDatabase(tb testing.TB) *mongo.Database {
	tb.Helper()

	return UnreachableClient(tb).Database("mongotest")
}
