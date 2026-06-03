// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"context"
	"errors"
	"fmt"

	mmongoDB "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// MongoReadinessChecker implements ReadinessChecker by issuing a real round-trip
// to MongoDB. This is critical: GetDB returns a CACHED client pointer with no
// network activity after the first successful connect, so a checker that only
// calls GetDB cannot detect runtime failures (network partition, primary loss,
// container OOM after boot). Ping must reach across the wire on every call.
type MongoReadinessChecker struct {
	connection *mmongoDB.MongoConnection
}

// NewMongoReadinessChecker creates a new MongoReadinessChecker.
func NewMongoReadinessChecker(mc *mmongoDB.MongoConnection) *MongoReadinessChecker {
	return &MongoReadinessChecker{connection: mc}
}

// Ping verifies MongoDB connectivity by retrieving the underlying client and
// issuing client.Ping(ctx, readpref.Primary()) — a real round-trip to the
// primary node. Returns:
//   - "connection not configured" when the checker or its connection is nil
//     (defensive against miswired bootstrap paths)
//   - GetDB's error when the connection cannot be established/retrieved
//   - client.Ping's error when the round-trip fails (network partition,
//     unreachable primary, auth failure, etc.)
//   - nil only when the round-trip succeeds
//
// readpref.Primary() is intentional: probing the primary catches replica-set
// failover scenarios where secondaries are reachable but the primary is not.
func (m *MongoReadinessChecker) Ping(ctx context.Context) error {
	if m == nil || m.connection == nil {
		return errors.New("mongodb readiness checker: connection not configured")
	}

	client, err := m.connection.GetDB(ctx)
	if err != nil {
		return fmt.Errorf("mongo: get client: %w", err)
	}

	if client == nil {
		return errors.New("mongodb readiness checker: nil client returned by GetDB")
	}

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("mongo: ping primary: %w", err)
	}

	return nil
}
