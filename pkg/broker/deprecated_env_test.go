// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package broker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeprecatedBrokerEnvVariables(t *testing.T) {
	t.Run("detects deprecated prefixes and explicit keys", func(t *testing.T) {
		env := []string{
			"RABBITMQ_URI=amqp://localhost",
			"AUTHORIZER_RABBITMQ_ENABLED=true",
			"BROKER_HEALTH_CHECK_TIMEOUT=3s",
			"UNRELATED=value",
		}

		deprecated := DeprecatedBrokerEnvVariables(env)

		assert.Equal(t, []string{
			"AUTHORIZER_RABBITMQ_ENABLED",
			"BROKER_HEALTH_CHECK_TIMEOUT",
			"RABBITMQ_URI",
		}, deprecated)
	})

	t.Run("returns empty list when nothing deprecated is set", func(t *testing.T) {
		deprecated := DeprecatedBrokerEnvVariables([]string{"ENV_NAME=development", "REDPANDA_BROKERS=127.0.0.1:9092"})
		assert.Empty(t, deprecated)
	})
}
