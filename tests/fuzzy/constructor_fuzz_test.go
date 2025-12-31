package fuzzy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// assertionPanicRecovery returns true if the panic was an assertion failure (expected),
// false if it was an unexpected panic, and does not recover if no panic occurred.
func assertionPanicRecovery(t *testing.T, panicValue any, context string) bool {
	t.Helper()
	if panicValue == nil {
		return true // no panic
	}

	msg := fmt.Sprintf("%v", panicValue)
	if strings.Contains(msg, "assertion failed") {
		return true // expected assertion panic
	}

	t.Errorf("Unexpected panic (not assertion) in %s: %v", context, panicValue)
	return false
}

// FuzzNewHolder tests the NewHolder constructor with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzNewHolder -run=^$ -fuzztime=30s
func FuzzNewHolder(f *testing.F) {
	// Valid seeds
	validUUID := "00000000-0000-0000-0000-000000000001"
	f.Add(validUUID, "John Doe", "12345678901", "NATURAL_PERSON")
	f.Add(validUUID, "Jane Corp", "12345678000199", "LEGAL_PERSON")

	// Edge case seeds - invalid UUIDs
	f.Add("", "Name", "Doc", "NATURAL_PERSON")
	f.Add("invalid-uuid", "Name", "Doc", "NATURAL_PERSON")
	f.Add("00000000-0000-0000-0000-000000000000", "Name", "Doc", "NATURAL_PERSON") // Nil UUID

	// Edge case seeds - empty required fields
	f.Add(validUUID, "", "Doc", "NATURAL_PERSON")   // Empty name
	f.Add(validUUID, "Name", "", "NATURAL_PERSON")  // Empty document
	f.Add(validUUID, "Name", "Doc", "")             // Empty type
	f.Add(validUUID, "Name", "Doc", "invalid_type") // Invalid type

	// Edge case seeds - boundary values
	f.Add(validUUID, strings.Repeat("a", 1000), "Doc", "NATURAL_PERSON") // Long name
	f.Add(validUUID, "Name", strings.Repeat("9", 100), "LEGAL_PERSON")   // Long document

	f.Fuzz(func(t *testing.T, idStr, name, document, holderType string) {
		defer func() {
			assertionPanicRecovery(t, recover(), fmt.Sprintf(
				"FuzzNewHolder(id=%q, name=%q, doc=%q, type=%q)",
				idStr, name, document, holderType))
		}()

		id, err := uuid.Parse(idStr)
		if err != nil {
			return // Invalid UUID string - skip (pre-condition not met)
		}

		holder := mmodel.NewHolder(id, name, document, holderType)

		// If we reach here without panic, validate the result
		if holder == nil {
			t.Error("NewHolder returned nil without panicking")
			return
		}

		// Verify the holder has expected fields set
		if holder.ID == nil || *holder.ID != id {
			t.Errorf("ID mismatch: got %v, want %v", holder.ID, id)
		}
		if holder.Name == nil || *holder.Name != name {
			t.Errorf("Name mismatch: got %v, want %v", holder.Name, name)
		}
	})
}
