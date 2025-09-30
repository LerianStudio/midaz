package helpers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// TestIsolation provides methods to ensure test data isolation
type TestIsolation struct {
	testRunID string
	timestamp string
}

// NewTestIsolation creates a new test isolation helper
func NewTestIsolation() *TestIsolation {
	// Generate a unique test run ID combining timestamp and random hex
	timestamp := time.Now().UTC().Format("20060102150405")
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)

	return &TestIsolation{
		testRunID: fmt.Sprintf("%s-%s", timestamp, randomHex),
		timestamp: timestamp,
	}
}

// UniqueOrgName generates a unique organization name for this test run
func (ti *TestIsolation) UniqueOrgName(prefix string) string {
	return fmt.Sprintf("%s-%s-%s", prefix, ti.testRunID, RandString(6))
}

// UniqueLedgerName generates a unique ledger name for this test run
func (ti *TestIsolation) UniqueLedgerName(prefix string) string {
	return fmt.Sprintf("%s-%s-%s", prefix, ti.testRunID, RandString(6))
}

// UniqueAccountAlias generates a unique account alias for this test run
func (ti *TestIsolation) UniqueAccountAlias(prefix string) string {
	return fmt.Sprintf("%s-%s-%s", prefix, ti.testRunID, RandString(8))
}

// UniqueAssetCode generates a unique asset code for this test run
// Asset codes are typically shorter, so we use a different pattern
func (ti *TestIsolation) UniqueAssetCode(prefix string) string {
	// Ensure total length doesn't exceed typical asset code limits
	return fmt.Sprintf("%s%s", prefix, RandHex(4))
}

// UniqueTransactionCode generates a unique transaction code
func (ti *TestIsolation) UniqueTransactionCode(prefix string) string {
	return fmt.Sprintf("%s-%s-%s", prefix, ti.testRunID, RandString(10))
}

// TestRunID returns the unique identifier for this test run
func (ti *TestIsolation) TestRunID() string {
	return ti.testRunID
}

// MakeTestHeaders creates headers with a unique request ID for this test run
func (ti *TestIsolation) MakeTestHeaders() map[string]string {
	return AuthHeaders(fmt.Sprintf("test-%s-%s", ti.testRunID, RandHex(8)))
}
