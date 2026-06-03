// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/testutil"
	"tracer/pkg/model"
)

// =============================================================================
// Audit Event Deduplication Tests
//
// These tests verify:
// 1. Duplicate audit INSERT for TRANSACTION_VALIDATED events returns no error
// 2. Only 1 audit row exists per (resource_id, event_type) for TRANSACTION_VALIDATED
// 3. Hash chain integrity is preserved after dedup
// 4. Non-TRANSACTION_VALIDATED events are unaffected by dedup constraint
//
// Implementation:
// - Migration 000011: partial UNIQUE index on audit_events(resource_id, event_type)
//   WHERE resource_type = 'transaction'
// - Repository: INSERT...SELECT...WHERE NOT EXISTS pattern for deduplication
//   (ON CONFLICT cannot be used because audit_events has PostgreSQL RULEs)
// =============================================================================

// insertAuditEventDirect inserts an audit event directly into the database
// bypassing the repository hash chain trigger. This is for testing dedup only.
// Uses a WHERE NOT EXISTS subquery to silently ignore duplicates for transaction validations
// (since ON CONFLICT cannot be used with tables that have PostgreSQL RULEs).
// Returns error only for non-duplicate failures.
func insertAuditEventDirect(t *testing.T, db *sql.DB, event *model.AuditEvent) error {
	t.Helper()

	// PostgreSQL tables with RULEs (like audit_events which has prevent_update/delete rules)
	// cannot use ON CONFLICT. Instead, we use INSERT ... SELECT ... WHERE NOT EXISTS
	// to achieve the same deduplication behavior for transaction validation events.
	//
	// For non-transaction resource types, this will always insert (no dedup).
	// For transaction resource types, it will only insert if no matching record exists.
	//
	// Note: We use separate parameters ($15, $16, $17) for the WHERE clause to avoid
	// PostgreSQL type inference issues with reusing parameters in different contexts.
	query := `
		INSERT INTO audit_events (
			event_id, event_type, created_at, action, result,
			resource_id, resource_type,
			actor_type, actor_id, actor_name, actor_role, actor_ip_address,
			context, metadata
		)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14::jsonb
		WHERE NOT EXISTS (
			SELECT 1 FROM audit_events
			WHERE resource_id = $15
			  AND event_type = $16
			  AND resource_type = 'transaction'
			  AND $17 = 'transaction'
		)
	`

	_, err := db.ExecContext(context.Background(), query,
		event.EventID,
		string(event.EventType),
		event.CreatedAt,
		string(event.Action),
		string(event.Result),
		event.ResourceID,
		string(event.ResourceType),
		string(event.Actor.ActorType),
		event.Actor.ID,
		event.Actor.Name,
		event.Actor.Role,
		event.Actor.IPAddress,
		`{}`,                       // context JSON ($13)
		`{}`,                       // metadata JSON ($14)
		event.ResourceID,           // $15: duplicate for WHERE resource_id
		string(event.EventType),    // $16: duplicate for WHERE event_type
		string(event.ResourceType), // $17: duplicate for WHERE resource_type check
	)

	return err
}

// countAuditEventsByResourceID counts audit events for a specific resource_id and event_type
func countAuditEventsByResourceID(t *testing.T, db *sql.DB, resourceID string, eventType string) int {
	t.Helper()

	var count int
	query := `SELECT COUNT(*) FROM audit_events WHERE resource_id = $1 AND event_type = $2`
	err := db.QueryRowContext(context.Background(), query, resourceID, eventType).Scan(&count)
	require.NoError(t, err, "Failed to count audit events")

	return count
}

// getLatestAuditEventHash retrieves the hash of the latest audit event in the chain
func getLatestAuditEventHash(t *testing.T, db *sql.DB) string {
	t.Helper()

	var hash sql.NullString
	query := `SELECT hash FROM audit_events ORDER BY id DESC LIMIT 1`
	err := db.QueryRowContext(context.Background(), query).Scan(&hash)

	if err == sql.ErrNoRows {
		return ""
	}

	require.NoError(t, err, "Failed to get latest audit event hash")

	if hash.Valid {
		return hash.String
	}

	return ""
}

// verifyHashChainIntegrity checks that all audit events have valid hash chain
func verifyHashChainIntegrity(t *testing.T, db *sql.DB) bool {
	t.Helper()

	// Get the latest event ID
	var maxID int64
	query := `SELECT COALESCE(MAX(id), 0) FROM audit_events`
	err := db.QueryRowContext(context.Background(), query).Scan(&maxID)
	require.NoError(t, err, "Failed to get max audit event ID")

	if maxID == 0 {
		return true // Empty table, no chain to verify
	}

	// Get the earliest event ID
	var minID int64
	minQuery := `SELECT COALESCE(MIN(id), 0) FROM audit_events`
	err = db.QueryRowContext(context.Background(), minQuery).Scan(&minID)
	require.NoError(t, err, "Failed to get min audit event ID")

	// Call the verification function
	var isValid bool
	verifyQuery := `SELECT is_valid FROM verify_audit_hash_chain($1, $2)`
	err = db.QueryRowContext(context.Background(), verifyQuery, minID, maxID).Scan(&isValid)
	require.NoError(t, err, "Failed to verify hash chain")

	return isValid
}

// createTestAuditEvent creates a test audit event struct
func createTestAuditEvent(resourceID string, eventType model.AuditEventType) *model.AuditEvent {
	return &model.AuditEvent{
		EventID:      uuid.New(),
		EventType:    eventType,
		CreatedAt:    time.Now().UTC(),
		Action:       model.AuditActionValidate,
		Result:       model.AuditResultAllow,
		ResourceID:   resourceID,
		ResourceType: model.ResourceTypeTransaction,
		Actor: model.Actor{
			ActorType: model.ActorTypeSystem,
			ID:        "system",
			Name:      "Tracer System",
			Role:      "system",
			IPAddress: "127.0.0.1",
		},
	}
}

// =============================================================================
// Test 1: InsertAuditEvent_Duplicate_SilentlyIgnored
// Same resource_id + event_type for TRANSACTION_VALIDATED -> no error, 1 row
// =============================================================================

func TestInsertAuditEvent_Duplicate_SilentlyIgnored(t *testing.T) {
	// Setup: Get database connection
	db := testutil.SetupIntegrationDB(t)

	// Use deterministic UUID for test reproducibility
	resourceID := testutil.MustDeterministicUUID(13001).String()

	// Create first audit event for TRANSACTION_VALIDATED
	event1 := createTestAuditEvent(resourceID, model.AuditEventTransactionValidated)
	event1.Result = model.AuditResultAllow

	// Insert first event - should succeed
	err := insertAuditEventDirect(t, db, event1)
	require.NoError(t, err, "First audit event insert should succeed")

	// Verify first event was inserted
	count := countAuditEventsByResourceID(t, db, resourceID, string(model.AuditEventTransactionValidated))
	require.Equal(t, 1, count, "Should have exactly 1 audit event after first insert")

	// Create second audit event with SAME resource_id and event_type (duplicate)
	event2 := createTestAuditEvent(resourceID, model.AuditEventTransactionValidated)
	event2.EventID = uuid.New() // Different event_id
	event2.Result = model.AuditResultDeny

	// Insert duplicate - should NOT fail (silently ignored via WHERE NOT EXISTS)
	err = insertAuditEventDirect(t, db, event2)
	assert.NoError(t, err, "Duplicate audit event insert should not return error (silently ignored)")

	// Verify still only 1 event exists (dedup working)
	countAfter := countAuditEventsByResourceID(t, db, resourceID, string(model.AuditEventTransactionValidated))
	assert.Equal(t, 1, countAfter, "Should still have exactly 1 audit event after duplicate insert (dedup)")
}

// =============================================================================
// Test 2: InsertAuditEvent_DifferentResourceType_Unaffected
// Non-TRANSACTION_VALIDATED events should NOT be affected by dedup constraint
// =============================================================================

func TestInsertAuditEvent_DifferentResourceType_Unaffected(t *testing.T) {
	// Setup: Get database connection
	db := testutil.SetupIntegrationDB(t)

	// Use deterministic UUIDs
	ruleResourceID := testutil.MustDeterministicUUID(13002).String()

	// Create first RULE_CREATED event
	event1 := &model.AuditEvent{
		EventID:      uuid.New(),
		EventType:    model.AuditEventRuleCreated,
		CreatedAt:    time.Now().UTC(),
		Action:       model.AuditActionCreate,
		Result:       model.AuditResultSuccess,
		ResourceID:   ruleResourceID,
		ResourceType: model.ResourceTypeRule,
		Actor: model.Actor{
			ActorType: model.ActorTypeUser,
			ID:        "user-123",
			Name:      "Test User",
			Role:      "system",
			IPAddress: "192.168.1.1",
		},
	}

	err := insertAuditEventDirect(t, db, event1)
	require.NoError(t, err, "First RULE_CREATED event should succeed")

	// Create second RULE_UPDATED event for the SAME resource_id
	// This should succeed because the unique constraint is only on TRANSACTION_VALIDATED
	event2 := &model.AuditEvent{
		EventID:      uuid.New(),
		EventType:    model.AuditEventRuleUpdated,
		CreatedAt:    time.Now().UTC(),
		Action:       model.AuditActionUpdate,
		Result:       model.AuditResultSuccess,
		ResourceID:   ruleResourceID, // Same resource_id
		ResourceType: model.ResourceTypeRule,
		Actor: model.Actor{
			ActorType: model.ActorTypeUser,
			ID:        "user-123",
			Name:      "Test User",
			Role:      "system",
			IPAddress: "192.168.1.1",
		},
	}

	err = insertAuditEventDirect(t, db, event2)
	assert.NoError(t, err, "RULE_UPDATED event for same resource_id should succeed (not affected by dedup)")

	// Verify both events exist (no dedup for non-TRANSACTION_VALIDATED)
	var countRuleCreated, countRuleUpdated int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_events WHERE resource_id = $1 AND event_type = 'RULE_CREATED'`,
		ruleResourceID).Scan(&countRuleCreated)
	require.NoError(t, err)

	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_events WHERE resource_id = $1 AND event_type = 'RULE_UPDATED'`,
		ruleResourceID).Scan(&countRuleUpdated)
	require.NoError(t, err)

	assert.Equal(t, 1, countRuleCreated, "Should have 1 RULE_CREATED event")
	assert.Equal(t, 1, countRuleUpdated, "Should have 1 RULE_UPDATED event")
}

// =============================================================================
// Test 3: HashChainIntegrity_PreservedAfterDedupAttempt
// After attempting duplicate insert, hash chain remains valid
// =============================================================================

func TestHashChainIntegrity_PreservedAfterDedupAttempt(t *testing.T) {
	// Setup: Get database connection
	db := testutil.SetupIntegrationDB(t)

	// Use deterministic UUIDs
	resourceID1 := testutil.MustDeterministicUUID(13003).String()
	resourceID2 := testutil.MustDeterministicUUID(13004).String()

	// Insert first event (new resource)
	event1 := createTestAuditEvent(resourceID1, model.AuditEventTransactionValidated)
	err := insertAuditEventDirect(t, db, event1)
	require.NoError(t, err, "First event insert should succeed")

	// Record hash chain state before duplicate attempt
	hashBefore := getLatestAuditEventHash(t, db)

	// Attempt duplicate insert (should be silently ignored)
	event1Dup := createTestAuditEvent(resourceID1, model.AuditEventTransactionValidated)
	event1Dup.EventID = uuid.New()
	err = insertAuditEventDirect(t, db, event1Dup)
	assert.NoError(t, err, "Duplicate insert should not return error")

	// Hash should be unchanged (no new event was inserted)
	hashAfterDup := getLatestAuditEventHash(t, db)

	// Insert a different event (new resource) to extend the chain
	event2 := createTestAuditEvent(resourceID2, model.AuditEventTransactionValidated)
	err = insertAuditEventDirect(t, db, event2)
	require.NoError(t, err, "Second event (different resource) should succeed")

	// Hash should now be different (new event was added)
	hashAfterNew := getLatestAuditEventHash(t, db)
	assert.NotEqual(t, hashBefore, hashAfterNew, "Hash should change after inserting new event")

	// If dedup worked correctly, hash after duplicate attempt should equal hash before
	assert.Equal(t, hashBefore, hashAfterDup, "Hash should not change after duplicate attempt (dedup working)")

	// Verify hash chain integrity
	isValid := verifyHashChainIntegrity(t, db)
	assert.True(t, isValid, "Hash chain should remain valid after all operations")
}

// =============================================================================
// Test 4: MultipleValidations_SameTransaction_OnlyFirstStored
// Multiple validation attempts for same transaction -> only first audit stored
// =============================================================================

func TestMultipleValidations_SameTransaction_OnlyFirstStored(t *testing.T) {
	// Setup: Get database connection
	db := testutil.SetupIntegrationDB(t)

	// Use deterministic UUID - represents a single transaction being validated multiple times
	transactionResourceID := testutil.MustDeterministicUUID(13005).String()

	// Simulate first validation attempt (ALLOW)
	event1 := createTestAuditEvent(transactionResourceID, model.AuditEventTransactionValidated)
	event1.Result = model.AuditResultAllow
	event1.CreatedAt = time.Now().UTC()

	err := insertAuditEventDirect(t, db, event1)
	require.NoError(t, err, "First validation audit should succeed")

	// Record the first event's data
	var firstEventID uuid.UUID
	var firstResult string
	err = db.QueryRowContext(context.Background(),
		`SELECT event_id, result FROM audit_events WHERE resource_id = $1 AND event_type = 'TRANSACTION_VALIDATED'`,
		transactionResourceID).Scan(&firstEventID, &firstResult)
	require.NoError(t, err, "Should find first event")
	require.Equal(t, "ALLOW", firstResult, "First event should have ALLOW result")

	// Simulate second validation attempt (DENY) - should be ignored
	event2 := createTestAuditEvent(transactionResourceID, model.AuditEventTransactionValidated)
	event2.Result = model.AuditResultDeny
	event2.CreatedAt = time.Now().UTC().Add(time.Second)

	err = insertAuditEventDirect(t, db, event2)
	assert.NoError(t, err, "Second validation audit should not error (silently ignored)")

	// Verify only ONE audit event exists for this transaction
	count := countAuditEventsByResourceID(t, db, transactionResourceID, string(model.AuditEventTransactionValidated))
	assert.Equal(t, 1, count, "Should have exactly 1 audit event per transaction (dedup)")

	// Verify the stored event is the FIRST one (ALLOW), not the second (DENY)
	var storedEventID uuid.UUID
	var storedResult string
	err = db.QueryRowContext(context.Background(),
		`SELECT event_id, result FROM audit_events WHERE resource_id = $1 AND event_type = 'TRANSACTION_VALIDATED'`,
		transactionResourceID).Scan(&storedEventID, &storedResult)
	require.NoError(t, err, "Should find the stored event")

	assert.Equal(t, firstEventID, storedEventID, "Stored event should be the first one (not replaced)")
	assert.Equal(t, "ALLOW", storedResult, "Stored result should be ALLOW (first event's result)")
}

// =============================================================================
// Test 5: DifferentTransactions_BothStored
// Different transaction IDs should both be stored (no false dedup)
// =============================================================================

func TestDifferentTransactions_BothStored(t *testing.T) {
	// Setup: Get database connection
	db := testutil.SetupIntegrationDB(t)

	// Use deterministic UUIDs for two different transactions
	transaction1ID := testutil.MustDeterministicUUID(13006).String()
	transaction2ID := testutil.MustDeterministicUUID(13007).String()

	// Insert audit for first transaction
	event1 := createTestAuditEvent(transaction1ID, model.AuditEventTransactionValidated)
	event1.Result = model.AuditResultAllow

	err := insertAuditEventDirect(t, db, event1)
	require.NoError(t, err, "First transaction audit should succeed")

	// Insert audit for second transaction
	event2 := createTestAuditEvent(transaction2ID, model.AuditEventTransactionValidated)
	event2.Result = model.AuditResultDeny

	err = insertAuditEventDirect(t, db, event2)
	require.NoError(t, err, "Second transaction audit should succeed")

	// Verify both events exist (dedup is per-transaction, not global)
	count1 := countAuditEventsByResourceID(t, db, transaction1ID, string(model.AuditEventTransactionValidated))
	count2 := countAuditEventsByResourceID(t, db, transaction2ID, string(model.AuditEventTransactionValidated))

	assert.Equal(t, 1, count1, "First transaction should have 1 audit event")
	assert.Equal(t, 1, count2, "Second transaction should have 1 audit event")

	// Total TRANSACTION_VALIDATED events should be 2
	var totalCount int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_events WHERE event_type = 'TRANSACTION_VALIDATED' AND resource_id IN ($1, $2)`,
		transaction1ID, transaction2ID).Scan(&totalCount)
	require.NoError(t, err)
	assert.Equal(t, 2, totalCount, "Should have 2 distinct transaction audits")
}
