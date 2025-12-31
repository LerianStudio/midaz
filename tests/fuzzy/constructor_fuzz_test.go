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

func isValidUUIDString(s string) bool {
	if s == "" {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
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
		id, err := uuid.Parse(idStr)
		if err != nil {
			return // Invalid UUID string - skip (pre-condition not met)
		}

		shouldPanic := id == uuid.Nil ||
			name == "" ||
			document == "" ||
			holderType == "" ||
			(holderType != mmodel.HolderTypeNaturalPerson && holderType != mmodel.HolderTypeLegalPerson)

		defer func() {
			if r := recover(); r != nil {
				assertionPanicRecovery(t, r, fmt.Sprintf(
					"FuzzNewHolder(id=%q, name=%q, doc=%q, type=%q)",
					idStr, name, document, holderType))
				return
			}
			if shouldPanic {
				t.Errorf("Expected assertion panic for invalid input: id=%q name=%q doc=%q type=%q",
					idStr, name, document, holderType)
			}
		}()

		holder := mmodel.NewHolder(id, name, document, holderType)

		if shouldPanic {
			return
		}

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

// FuzzNewBalance tests the NewBalance constructor with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzNewBalance -run=^$ -fuzztime=30s
func FuzzNewBalance(f *testing.F) {
	validUUID := "00000000-0000-0000-0000-000000000001"

	// Valid seeds
	f.Add(validUUID, validUUID, validUUID, validUUID, "@alias", "USD", "checking")
	f.Add(validUUID, validUUID, validUUID, validUUID, "@person1", "BRL", "savings")

	// Edge case seeds - invalid UUIDs
	f.Add("", validUUID, validUUID, validUUID, "@alias", "USD", "checking")
	f.Add(validUUID, "", validUUID, validUUID, "@alias", "USD", "checking")
	f.Add(validUUID, validUUID, "", validUUID, "@alias", "USD", "checking")
	f.Add(validUUID, validUUID, validUUID, "", "@alias", "USD", "checking")
	f.Add("invalid", "invalid", "invalid", "invalid", "@a", "B", "c")

	// Edge case seeds - empty required fields
	f.Add(validUUID, validUUID, validUUID, validUUID, "", "USD", "checking")    // Empty alias
	f.Add(validUUID, validUUID, validUUID, validUUID, "@alias", "", "checking") // Empty assetCode

	// Edge case seeds - unusual but potentially valid
	f.Add(validUUID, validUUID, validUUID, validUUID, "@alias", "usd", "checking") // Lowercase asset
	f.Add(validUUID, validUUID, validUUID, validUUID, "no-at-prefix", "USD", "")   // No @ prefix, empty type

	f.Fuzz(func(t *testing.T, id, orgID, ledgerID, accountID, alias, assetCode, accountType string) {
		shouldPanic := !isValidUUIDString(id) ||
			!isValidUUIDString(orgID) ||
			!isValidUUIDString(ledgerID) ||
			!isValidUUIDString(accountID) ||
			alias == "" ||
			assetCode == ""

		defer func() {
			if r := recover(); r != nil {
				assertionPanicRecovery(t, r, fmt.Sprintf(
					"FuzzNewBalance(id=%q, orgID=%q, ledgerID=%q, accountID=%q, alias=%q, asset=%q, type=%q)",
					id, orgID, ledgerID, accountID, alias, assetCode, accountType))
				return
			}
			if shouldPanic {
				t.Errorf("Expected assertion panic for invalid input: id=%q orgID=%q ledgerID=%q accountID=%q alias=%q asset=%q type=%q",
					id, orgID, ledgerID, accountID, alias, assetCode, accountType)
			}
		}()

		balance := mmodel.NewBalance(id, orgID, ledgerID, accountID, alias, assetCode, accountType)

		if shouldPanic {
			return
		}

		// If we reach here without panic, validate the result
		if balance == nil {
			t.Error("NewBalance returned nil without panicking")
			return
		}

		// Verify critical fields
		if balance.ID != id {
			t.Errorf("ID mismatch: got %q, want %q", balance.ID, id)
		}
		if balance.Alias != alias {
			t.Errorf("Alias mismatch: got %q, want %q", balance.Alias, alias)
		}
		if balance.Version != 1 {
			t.Errorf("Version should be 1 for new balance, got %d", balance.Version)
		}
	})
}

// FuzzNewAccount tests the NewAccount constructor with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzNewAccount -run=^$ -fuzztime=30s
func FuzzNewAccount(f *testing.F) {
	validUUID := "00000000-0000-0000-0000-000000000001"

	// Valid seeds
	f.Add(validUUID, validUUID, validUUID, "USD", "checking")
	f.Add(validUUID, validUUID, validUUID, "BRL", "savings")
	f.Add(validUUID, validUUID, validUUID, "EUR", "deposit")

	// Edge case seeds - invalid UUIDs
	f.Add("", validUUID, validUUID, "USD", "checking")
	f.Add(validUUID, "", validUUID, "USD", "checking")
	f.Add(validUUID, validUUID, "", "USD", "checking")
	f.Add("invalid-uuid", validUUID, validUUID, "USD", "checking")

	// Edge case seeds - empty required fields
	f.Add(validUUID, validUUID, validUUID, "", "checking") // Empty assetCode
	f.Add(validUUID, validUUID, validUUID, "USD", "")      // Empty accountType

	// Edge case seeds - boundary values
	f.Add(validUUID, validUUID, validUUID, strings.Repeat("A", 100), "checking")
	f.Add(validUUID, validUUID, validUUID, "USD", strings.Repeat("x", 256))

	f.Fuzz(func(t *testing.T, id, orgID, ledgerID, assetCode, accountType string) {
		shouldPanic := !isValidUUIDString(id) ||
			!isValidUUIDString(orgID) ||
			!isValidUUIDString(ledgerID) ||
			assetCode == "" ||
			accountType == ""

		defer func() {
			if r := recover(); r != nil {
				assertionPanicRecovery(t, r, fmt.Sprintf(
					"FuzzNewAccount(id=%q, orgID=%q, ledgerID=%q, asset=%q, type=%q)",
					id, orgID, ledgerID, assetCode, accountType))
				return
			}
			if shouldPanic {
				t.Errorf("Expected assertion panic for invalid input: id=%q orgID=%q ledgerID=%q asset=%q type=%q",
					id, orgID, ledgerID, assetCode, accountType)
			}
		}()

		account := mmodel.NewAccount(id, orgID, ledgerID, assetCode, accountType)

		if shouldPanic {
			return
		}

		// If we reach here without panic, validate the result
		if account == nil {
			t.Error("NewAccount returned nil without panicking")
			return
		}

		// Verify critical fields
		if account.ID != id {
			t.Errorf("ID mismatch: got %q, want %q", account.ID, id)
		}
		if account.AssetCode != assetCode {
			t.Errorf("AssetCode mismatch: got %q, want %q", account.AssetCode, assetCode)
		}
		if account.Status.Code != mmodel.AccountStatusActive {
			t.Errorf("Status should be ACTIVE for new account, got %q", account.Status.Code)
		}
	})
}
