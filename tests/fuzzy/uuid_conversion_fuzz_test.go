package fuzzy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// FuzzTransactionIDtoUUID tests the Transaction.IDtoUUID method with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzTransactionIDtoUUID -run=^$ -fuzztime=30s
func FuzzTransactionIDtoUUID(f *testing.F) {
	// Valid UUIDs
	f.Add("00000000-0000-0000-0000-000000000000") // Nil UUID (may be rejected)
	f.Add("00000000-0000-0000-0000-000000000001") // Valid
	f.Add("ffffffff-ffff-ffff-ffff-ffffffffffff") // All F

	// Invalid UUIDs - should trigger assertion
	f.Add("not-a-uuid")
	f.Add("")
	f.Add(strings.Repeat("0", 36))                 // No hyphens
	f.Add("00000000-0000-0000-0000-00000000000")   // Too short
	f.Add("00000000-0000-0000-0000-0000000000001") // Too long
	f.Add("00000000_0000_0000_0000_000000000001")  // Wrong separator

	f.Fuzz(func(t *testing.T, id string) {
		defer func() {
			assertionPanicRecovery(t, recover(), fmt.Sprintf(
				"FuzzTransactionIDtoUUID(id=%q)", id))
		}()

		tx := mmodel.Transaction{ID: id}
		result := tx.IDtoUUID()

		// If we reach here, the conversion succeeded
		// Verify it matches the expected UUID
		expected, err := uuid.Parse(id)
		if err != nil {
			t.Errorf("IDtoUUID succeeded but uuid.Parse failed for %q", id)
			return
		}

		if result != expected {
			t.Errorf("IDtoUUID(%q) = %v, want %v", id, result, expected)
		}
	})
}
