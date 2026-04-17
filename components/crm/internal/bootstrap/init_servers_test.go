// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// InitServers / InitServersWithOptions cannot reach their happy path without a
// live MongoDB, but they contain several early-failure branches (config load,
// telemetry init, cipher init, mongo connect) that are unit-testable by
// driving env vars. The tests below exercise those branches deterministically.
//
// Because these tests manipulate process-wide environment variables via
// t.Setenv, they cannot use t.Parallel(); Go's test framework serialises them
// automatically.

// setRequiredEnv populates the minimum set of env vars that pass config
// loading (SetConfigFromEnvVars never errors on empty strings, so this is
// mostly about steering the downstream branches we want to drive).
func setRequiredEnv(t *testing.T) {
	t.Helper()

	// Telemetry disabled so the OTLP exporters are not contacted.
	t.Setenv("ENABLE_TELEMETRY", "false")

	// A plausible address set so NewServer wires a non-empty address; the
	// server itself is not started.
	t.Setenv("SERVER_ADDRESS", ":0")

	// Auth disabled so NewAuthClient returns early without a healthcheck HTTP
	// request.
	t.Setenv("PLUGIN_AUTH_ENABLED", "false")
	t.Setenv("PLUGIN_AUTH_ADDRESS", "")

	// Force the fallback that clamps MaxPoolSize to 100 when unset.
	t.Setenv("MONGO_MAX_POOL_SIZE", "0")
}

// TestInitServersWithOptions_InvalidCipherKey exercises the cipher-init
// failure branch of InitServersWithOptions. A non-hex EncryptSecretKey causes
// Crypto.InitializeCipher to return an error, which wraps into the
// "failed to initialize cipher" message. This drives the config-load,
// telemetry-init, and mongo-struct-build branches before the cipher check.
func TestInitServersWithOptions_InvalidCipherKey(t *testing.T) {
	setRequiredEnv(t)

	// EncryptSecretKey must be hex; garbage forces aes.NewCipher to fail via
	// hex.DecodeString returning an error first.
	t.Setenv("LCRYPTO_HASH_SECRET_KEY", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")
	t.Setenv("LCRYPTO_ENCRYPT_SECRET_KEY", "nothex")

	// Mongo URI is ignored in this test: cipher init happens before any
	// mongo repository is constructed, so even an invalid URI will not cause
	// failure first.
	t.Setenv("MONGO_URI", "mongodb")
	t.Setenv("MONGO_HOST", "127.0.0.1")
	t.Setenv("MONGO_PORT", "1")
	t.Setenv("MONGO_NAME", "test_db")

	opts := &Options{Logger: libLog.Logger(&libLog.GoLogger{Level: libLog.InfoLevel})}

	svc, err := InitServersWithOptions(opts)

	require.Error(t, err, "non-hex cipher key should cause InitializeCipher to fail")
	assert.Nil(t, svc)
	assert.Contains(t, err.Error(), "failed to initialize cipher")
}

// TestInitServersWithOptions_MongoConnectFailure exercises the holder
// repository construction failure path. Valid cipher keys let
// InitializeCipher succeed, and a bogus mongo URI then fails when
// holder.NewMongoDBRepository calls GetDB against an unreachable server.
func TestInitServersWithOptions_MongoConnectFailure(t *testing.T) {
	setRequiredEnv(t)

	// Valid 32-byte hex keys so InitializeCipher succeeds.
	t.Setenv("LCRYPTO_HASH_SECRET_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("LCRYPTO_ENCRYPT_SECRET_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")

	// An entirely malformed mongo URI makes mongo.Connect fail synchronously.
	t.Setenv("MONGO_URI", "not-a-scheme")
	t.Setenv("MONGO_HOST", "")
	t.Setenv("MONGO_PORT", "")
	t.Setenv("MONGO_NAME", "test_db")
	t.Setenv("MONGO_USER", "")
	t.Setenv("MONGO_PASSWORD", "")
	t.Setenv("MONGO_PARAMETERS", "")

	svc, err := InitServersWithOptions(&Options{
		Logger: libLog.Logger(&libLog.GoLogger{Level: libLog.InfoLevel}),
	})

	require.Error(t, err, "bogus mongo URI should cause repository init to fail")
	assert.Nil(t, svc)
	// The error message should reference the holder repository — which is
	// constructed before the alias repository — so that branch is the one we
	// expect to trip first.
	assert.Contains(t, err.Error(), "failed to initialize holder repository")
}

// TestInitServers_DelegatesToInitServersWithOptions ensures the shim wrapper
// simply forwards to InitServersWithOptions with nil options. We trigger the
// same cipher-failure path to observe the shared error surface.
func TestInitServers_DelegatesToInitServersWithOptions(t *testing.T) {
	setRequiredEnv(t)

	t.Setenv("LCRYPTO_HASH_SECRET_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("LCRYPTO_ENCRYPT_SECRET_KEY", "not-valid-hex")

	svc, err := InitServers()

	require.Error(t, err)
	assert.Nil(t, svc)
	assert.Contains(t, err.Error(), "failed to initialize cipher")
}
