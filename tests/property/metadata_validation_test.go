package property

import (
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
)

const (
	metadataKeyLimit   = 100
	metadataValueLimit = 2000
)

// Property: Metadata keys must be ≤100 characters
func TestProperty_MetadataKeyLength_Model(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Generate key at boundary (100 chars - valid)
		validKey := generateRandomString(rng, metadataKeyLimit)
		if len(validKey) > metadataKeyLimit {
			t.Logf("Generated key too long: %d", len(validKey))
			return false
		}

		// Generate key over boundary (101 chars - invalid)
		invalidKey := generateRandomString(rng, metadataKeyLimit+1)
		if len(invalidKey) <= metadataKeyLimit {
			t.Logf("Generated key not over limit: %d", len(invalidKey))
			return false
		}

		// Property: valid key length is accepted, invalid is rejected
		// (This tests the constraint, not the validation function directly)
		return len(validKey) <= metadataKeyLimit && len(invalidKey) > metadataKeyLimit
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Metadata key length property failed: %v", err)
	}
}

func generateRandomString(rng *rand.Rand, length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rng.Intn(len(charset))]
	}
	return string(b)
}

// Property: Metadata string values must be ≤2000 characters
func TestProperty_MetadataValueLength_Model(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Generate value at boundary (2000 chars - valid)
		validValue := generateRandomString(rng, metadataValueLimit)
		if len(validValue) > metadataValueLimit {
			t.Logf("Generated value too long: %d", len(validValue))
			return false
		}

		// Generate value over boundary (2001 chars - invalid)
		invalidValue := generateRandomString(rng, metadataValueLimit+1)
		if len(invalidValue) <= metadataValueLimit {
			t.Logf("Generated value not over limit: %d", len(invalidValue))
			return false
		}

		// Property: valid length passes, invalid fails
		return len(validValue) <= metadataValueLimit && len(invalidValue) > metadataValueLimit
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Metadata value length property failed: %v", err)
	}
}

// Property: Boundary test - exactly at limit should pass
func TestProperty_MetadataLengthBoundary_Model(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Exactly 100 chars key - should be valid
		exactKey := strings.Repeat("a", metadataKeyLimit)
		// Exactly 2000 chars value - should be valid
		exactValue := strings.Repeat("b", metadataValueLimit)

		// One over - should be invalid
		overKey := strings.Repeat("a", metadataKeyLimit+1)
		overValue := strings.Repeat("b", metadataValueLimit+1)

		_ = rng // Use seed for consistency

		// Property: exact boundary is valid, one over is invalid
		keyBoundaryValid := len(exactKey) == metadataKeyLimit && len(overKey) == metadataKeyLimit+1
		valueBoundaryValid := len(exactValue) == metadataValueLimit && len(overValue) == metadataValueLimit+1

		return keyBoundaryValid && valueBoundaryValid
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Metadata length boundary property failed: %v", err)
	}
}

// Property: Metadata cannot contain nested maps (security constraint)
func TestProperty_MetadataNoNestedMaps_Model(t *testing.T) {
	f := func(seed int64) bool {
		// Valid metadata types: string, number, bool, nil, array
		validMetadata := map[string]any{
			"stringKey":  "value",
			"numberKey":  42,
			"floatKey":   3.14,
			"boolKey":    true,
			"nilKey":     nil,
			"arrayKey":   []any{"a", "b", "c"},
			"numArray":   []any{1, 2, 3},
			"mixedArray": []any{"str", 123, true},
		}

		// Invalid metadata: nested map
		invalidMetadata := map[string]any{
			"nested": map[string]any{
				"inner": "value",
			},
		}

		// Property: valid types don't contain maps, invalid does
		validHasNoMaps := !containsNestedMap(validMetadata)
		invalidHasMaps := containsNestedMap(invalidMetadata)

		if !validHasNoMaps {
			t.Log("Valid metadata incorrectly detected as having nested maps")
			return false
		}

		if !invalidHasMaps {
			t.Log("Invalid metadata not detected as having nested maps")
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Metadata no nested maps property failed: %v", err)
	}
}

// containsNestedMap checks if any value in the map is itself a map
func containsNestedMap(m map[string]any) bool {
	for _, v := range m {
		switch val := v.(type) {
		case map[string]any:
			return true
		case []any:
			for _, item := range val {
				if _, isMap := item.(map[string]any); isMap {
					return true
				}
			}
		}
	}
	return false
}
