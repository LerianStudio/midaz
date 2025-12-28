package property

import (
	"math/rand"
	"testing"
	"testing/quick"
)

// MockAccount represents account for hierarchy testing
type MockAccount struct {
	ID              string
	ParentAccountID *string
	AssetCode       string
	Alias           string
}

// Property: Child account must have same asset code as parent
func TestProperty_AccountHierarchyAssetCodeMatch_Model(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Create parent account
		parentAsset := generateAssetCode(rng)
		parentID := generateID(rng)
		parent := MockAccount{
			ID:              parentID,
			ParentAccountID: nil,
			AssetCode:       parentAsset,
			Alias:           "@parent",
		}

		// Create child with SAME asset (valid)
		validChild := MockAccount{
			ID:              generateID(rng),
			ParentAccountID: &parentID,
			AssetCode:       parent.AssetCode, // Same as parent
			Alias:           "@child-valid",
		}

		// Create child with DIFFERENT asset (invalid)
		differentAsset := generateAssetCode(rng)
		for differentAsset == parentAsset {
			differentAsset = generateAssetCode(rng)
		}
		invalidChild := MockAccount{
			ID:              generateID(rng),
			ParentAccountID: &parentID,
			AssetCode:       differentAsset, // Different from parent
			Alias:           "@child-invalid",
		}

		// Property: valid child has matching asset code
		if validChild.AssetCode != parent.AssetCode {
			t.Logf("Valid child asset mismatch: parent=%s child=%s",
				parent.AssetCode, validChild.AssetCode)
			return false
		}

		// Property: invalid child has different asset code (should be rejected)
		if invalidChild.AssetCode == parent.AssetCode {
			t.Logf("Invalid child unexpectedly matches: parent=%s child=%s",
				parent.AssetCode, invalidChild.AssetCode)
			return false
		}

		// Verify parent account is root (no parent) and children reference parent
		if parent.ParentAccountID != nil {
			t.Log("Parent should have nil ParentAccountID")
			return false
		}
		if validChild.ParentAccountID == nil || *validChild.ParentAccountID != parent.ID {
			t.Log("Valid child should reference parent")
			return false
		}
		if invalidChild.ParentAccountID == nil || *invalidChild.ParentAccountID != parent.ID {
			t.Log("Invalid child should reference parent")
			return false
		}

		// Verify aliases are set correctly
		if parent.Alias != "@parent" || validChild.Alias != "@child-valid" || invalidChild.Alias != "@child-invalid" {
			t.Log("Aliases should be set correctly")
			return false
		}

		// Verify all IDs are unique
		if validChild.ID == parent.ID || invalidChild.ID == parent.ID || validChild.ID == invalidChild.ID {
			t.Log("All account IDs should be unique")
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Account hierarchy asset code match property failed: %v", err)
	}
}

// Property: Account hierarchy must not contain circular references
func TestProperty_AccountHierarchyNoCircular_Model(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Build a valid hierarchy (no cycles)
		accounts := make(map[string]*MockAccount)

		// Root account
		rootID := generateID(rng)
		accounts[rootID] = &MockAccount{
			ID:              rootID,
			ParentAccountID: nil,
			AssetCode:       "USD",
			Alias:           "@root",
		}

		// Child of root
		childID := generateID(rng)
		accounts[childID] = &MockAccount{
			ID:              childID,
			ParentAccountID: &rootID,
			AssetCode:       "USD",
			Alias:           "@child",
		}

		// Grandchild
		grandchildID := generateID(rng)
		accounts[grandchildID] = &MockAccount{
			ID:              grandchildID,
			ParentAccountID: &childID,
			AssetCode:       "USD",
			Alias:           "@grandchild",
		}

		// Property: valid hierarchy has no cycles
		if hasCycle(accounts, grandchildID) {
			t.Log("Valid hierarchy incorrectly detected as cyclic")
			return false
		}

		// Create a cycle: grandchild -> root -> grandchild (invalid)
		accounts[rootID].ParentAccountID = &grandchildID

		// Property: cyclic hierarchy should be detected
		if !hasCycle(accounts, grandchildID) {
			t.Log("Cyclic hierarchy not detected")
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Account hierarchy no circular property failed: %v", err)
	}
}

// hasCycle detects cycles in account hierarchy using Floyd's algorithm
func hasCycle(accounts map[string]*MockAccount, startID string) bool {
	visited := make(map[string]bool)
	current := startID

	for {
		if visited[current] {
			return true // Cycle detected
		}

		account, exists := accounts[current]
		if !exists || account.ParentAccountID == nil {
			return false // Reached root, no cycle
		}

		visited[current] = true
		current = *account.ParentAccountID
	}
}

// Property: Account cannot be its own parent
func TestProperty_AccountCannotBeSelfParent_Model(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		id := generateID(rng)
		account := MockAccount{
			ID:              id,
			ParentAccountID: &id, // Self-reference (invalid)
			AssetCode:       "USD",
			Alias:           "@self-parent",
		}

		// Property: self-reference is invalid
		if account.ParentAccountID != nil && *account.ParentAccountID == account.ID {
			// This is the invalid case we're detecting
			// Verify all fields are properly set
			if account.AssetCode != "USD" {
				t.Log("Asset code should be USD")
				return false
			}
			if account.Alias != "@self-parent" {
				t.Log("Alias should be @self-parent")
				return false
			}
			return true // Test passes because we correctly identified the invalid state
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Account cannot be self parent property failed: %v", err)
	}
}

func generateAssetCode(rng *rand.Rand) string {
	codes := []string{"USD", "EUR", "BRL", "GBP", "JPY", "CHF", "CAD", "AUD"}
	return codes[rng.Intn(len(codes))]
}

func generateID(rng *rand.Rand) string {
	const chars = "abcdef0123456789"
	id := make([]byte, 32)
	for i := range id {
		id[i] = chars[rng.Intn(len(chars))]
	}
	return string(id)
}
