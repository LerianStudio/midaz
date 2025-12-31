package outbox

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// FuzzNewMetadataOutbox tests the NewMetadataOutbox constructor validation.
// Run with: go test -v ./components/transaction/internal/adapters/postgres/outbox -fuzz=FuzzNewMetadataOutbox -run=^$ -fuzztime=30s
func FuzzNewMetadataOutbox(f *testing.F) {
	// Valid inputs
	f.Add("valid-entity-id", "Transaction", `{"key": "value"}`)
	f.Add("another-id", "Operation", `{"nested": {"key": "value"}}`)

	// Edge case: entity ID boundaries
	f.Add("", "Transaction", `{}`)                       // Empty ID (invalid)
	f.Add(strings.Repeat("a", 255), "Transaction", `{}`) // Max length ID
	f.Add(strings.Repeat("a", 256), "Transaction", `{}`) // Over max length (invalid)
	f.Add("x", "Transaction", `{}`)                      // Min length ID

	// Edge case: invalid entity types
	f.Add("valid-id", "InvalidType", `{}`)
	f.Add("valid-id", "", `{}`)
	f.Add("valid-id", "transaction", `{}`) // Wrong case
	f.Add("valid-id", "TRANSACTION", `{}`) // Wrong case

	// Edge case: metadata variations
	f.Add("valid-id", "Transaction", `null`) // null metadata (will be nil)
	f.Add("valid-id", "Transaction", `{"a": 1, "b": "test"}`)
	f.Add("valid-id", "Transaction", `[]`) // Array (invalid for map)

	f.Fuzz(func(t *testing.T, entityID, entityType, metadataJSON string) {
		var metadata map[string]any

		// Try to parse metadata JSON
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			// Invalid JSON or wrong type - test with nil
			metadata = nil
		}

		result, err := NewMetadataOutbox(entityID, entityType, metadata)

		// Validate error conditions
		if entityID == "" {
			if err == nil || !errors.Is(err, ErrEntityIDEmpty) {
				t.Errorf("Expected ErrEntityIDEmpty for empty entityID, got: %v", err)
			}
			return
		}

		if len(entityID) > MaxEntityIDLength {
			if err == nil || !errors.Is(err, ErrEntityIDTooLong) {
				t.Errorf("Expected ErrEntityIDTooLong for entityID len=%d, got: %v", len(entityID), err)
			}
			return
		}

		if entityType != EntityTypeTransaction && entityType != EntityTypeOperation {
			if err == nil || !errors.Is(err, ErrInvalidEntityType) {
				t.Errorf("Expected ErrInvalidEntityType for entityType=%q, got: %v", entityType, err)
			}
			return
		}

		if metadata == nil {
			if err == nil || !errors.Is(err, ErrMetadataNil) {
				t.Errorf("Expected ErrMetadataNil for nil metadata, got: %v", err)
			}
			return
		}

		// If we expect success
		if err != nil {
			// Could be metadata too large or other validation error
			if !errors.Is(err, ErrMetadataTooLarge) {
				t.Errorf("Unexpected error for valid inputs: %v", err)
			}
			return
		}

		// Validate successful result
		if result == nil {
			t.Error("NewMetadataOutbox returned nil without error")
			return
		}

		if result.EntityID != entityID {
			t.Errorf("EntityID mismatch: got %q, want %q", result.EntityID, entityID)
		}

		if result.Status != StatusPending {
			t.Errorf("Status should be PENDING for new outbox entry, got %v", result.Status)
		}
	})
}
