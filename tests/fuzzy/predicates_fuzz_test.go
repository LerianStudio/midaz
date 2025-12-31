package fuzzy

import (
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var _ = decimal.Zero

// FuzzValidUUID tests the ValidUUID predicate against uuid.Parse for consistency.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidUUID -run=^$ -fuzztime=30s
func FuzzValidUUID(f *testing.F) {
	// Valid UUIDs
	f.Add("00000000-0000-0000-0000-000000000000") // Nil UUID
	f.Add("00000000-0000-0000-0000-000000000001") // Valid
	f.Add("ffffffff-ffff-ffff-ffff-ffffffffffff") // All F
	f.Add("a1b2c3d4-e5f6-7890-abcd-ef1234567890") // Mixed case

	// Invalid UUIDs
	f.Add("not-a-uuid")
	f.Add("")
	f.Add(strings.Repeat("0", 36))                 // No hyphens
	f.Add("00000000-0000-0000-0000-00000000000")   // Too short (35 chars)
	f.Add("00000000-0000-0000-0000-0000000000001") // Too long (37 chars)
	f.Add("00000000_0000_0000_0000_000000000001")  // Wrong separator
	f.Add("g0000000-0000-0000-0000-000000000001")  // Invalid hex char
	f.Add("00000000-0000-0000-0000-000000000001\n") // Trailing newline
	f.Add(" 00000000-0000-0000-0000-000000000001") // Leading space

	f.Fuzz(func(t *testing.T, s string) {
		result := assert.ValidUUID(s)

		// Cross-check with uuid.Parse
		_, err := uuid.Parse(s)
		expected := err == nil

		if result != expected {
			t.Errorf("ValidUUID(%q) = %v, but uuid.Parse returned err=%v", s, result, err)
		}
	})
}
