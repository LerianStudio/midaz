package testutils

import (
	"testing"

	libCrypto "github.com/LerianStudio/lib-commons/v3/commons/crypto"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	"github.com/stretchr/testify/require"
)

// Test-only encryption keys. These are hex-encoded 32-byte (64 hex chars) values.
// WARNING: These keys are for testing only and MUST NOT be used in production.
const (
	TestHashKey    = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	TestEncryptKey = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
)

// SetupCrypto creates a Crypto instance configured for testing.
// It uses fixed test keys for deterministic encryption/hashing in tests.
//
// Example:
//
//	func TestEncryption(t *testing.T) {
//	    crypto := testutils.SetupCrypto(t)
//	    encrypted, err := crypto.Encrypt(testutils.Ptr("sensitive-data"))
//	    require.NoError(t, err)
//	}
func SetupCrypto(t *testing.T) *libCrypto.Crypto {
	t.Helper()

	logger := &libLog.GoLogger{Level: libLog.InfoLevel}

	crypto := &libCrypto.Crypto{
		HashSecretKey:    TestHashKey,
		EncryptSecretKey: TestEncryptKey,
		Logger:           logger,
	}

	err := crypto.InitializeCipher()
	require.NoError(t, err, "failed to initialize crypto cipher for testing")

	return crypto
}
