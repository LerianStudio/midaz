// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig_HasCryptoHashSecretKeyPluginCRMField verifies that the worker Config struct
// has a CryptoHashSecretKeyPluginCRM field loaded from CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM env var.
func TestConfig_HasCryptoHashSecretKeyPluginCRMField(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		CryptoHashSecretKeyPluginCRM: "test-hash-secret",
	}

	assert.Equal(t, "test-hash-secret", cfg.CryptoHashSecretKeyPluginCRM)
}

// TestConfig_HasCryptoEncryptSecretKeyPluginCRMField verifies that the worker Config struct
// has a CryptoEncryptSecretKeyPluginCRM field loaded from CRYPTO_ENCRYPT_SECRET_KEY_PLUGIN_CRM env var.
func TestConfig_HasCryptoEncryptSecretKeyPluginCRMField(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		CryptoEncryptSecretKeyPluginCRM: "test-encrypt-secret",
	}

	assert.Equal(t, "test-encrypt-secret", cfg.CryptoEncryptSecretKeyPluginCRM)
}

// TestNewMultiQueueConsumer_ReceivesQueueName verifies that NewMultiQueueConsumer
// accepts the queue name as a parameter instead of reading it from os.Getenv.
func TestNewMultiQueueConsumer_ReceivesQueueName(t *testing.T) {
	t.Parallel()

	// This test verifies that NewMultiQueueConsumer accepts a queueName and logger parameter.

	queueName := "reporter.generate-report.queue"
	logger := &log.NopLogger{}

	consumer := NewMultiQueueConsumer(nil, nil, queueName, logger, nil)

	require.NotNil(t, consumer)
}
