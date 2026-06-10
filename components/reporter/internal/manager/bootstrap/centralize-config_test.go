// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig_HasRabbitMQExchangeField verifies that the manager Config struct
// has a RabbitMQExchange field loaded from RABBITMQ_EXCHANGE env var.
func TestConfig_HasRabbitMQExchangeField(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		RabbitMQExchange: "reporter.generate-report.exchange",
	}

	assert.Equal(t, "reporter.generate-report.exchange", cfg.RabbitMQExchange)
}

// TestConfig_HasRabbitMQGenerateReportKeyField verifies that the manager Config struct
// has a RabbitMQGenerateReportKey field loaded from RABBITMQ_GENERATE_REPORT_KEY env var.
func TestConfig_HasRabbitMQGenerateReportKeyField(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		RabbitMQGenerateReportKey: "reporter.generate-report.key",
	}

	assert.Equal(t, "reporter.generate-report.key", cfg.RabbitMQGenerateReportKey)
}

// TestConfig_Validate_RequiresRabbitMQExchange verifies that the RabbitMQExchange
// field is validated as required during config validation.
func TestConfig_Validate_RequiresRabbitMQExchange(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	cfg.RabbitMQExchange = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RABBITMQ_EXCHANGE is required")
}

// TestConfig_Validate_RequiresRabbitMQGenerateReportKey verifies that the
// RabbitMQGenerateReportKey field is validated as required during config validation.
func TestConfig_Validate_RequiresRabbitMQGenerateReportKey(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	cfg.RabbitMQGenerateReportKey = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RABBITMQ_GENERATE_REPORT_KEY is required")
}
