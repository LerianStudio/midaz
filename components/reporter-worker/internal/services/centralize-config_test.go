// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestUseCase_HasCryptoHashSecretKeyField verifies that the worker UseCase struct
// has a CryptoHashSecretKeyCRM field for centralized configuration
// instead of using os.Getenv("CRYPTO_HASH_SECRET_KEY_CRM").
func TestUseCase_HasCryptoHashSecretKeyField(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:                 log.NewNop(),
		Tracer:                 noop.NewTracerProvider().Tracer("test"),
		CryptoHashSecretKeyCRM: "test-hash-secret-key",
	}

	assert.Equal(t, "test-hash-secret-key", uc.CryptoHashSecretKeyCRM)
}

// TestUseCase_HasCryptoEncryptSecretKeyField verifies that the worker UseCase struct
// has a CryptoEncryptSecretKeyCRM field for centralized configuration
// instead of using os.Getenv("CRYPTO_ENCRYPT_SECRET_KEY_CRM").
func TestUseCase_HasCryptoEncryptSecretKeyField(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:                    log.NewNop(),
		Tracer:                    noop.NewTracerProvider().Tracer("test"),
		CryptoEncryptSecretKeyCRM: "test-encrypt-secret-key",
	}

	assert.Equal(t, "test-encrypt-secret-key", uc.CryptoEncryptSecretKeyCRM)
}
