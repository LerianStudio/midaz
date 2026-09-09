// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// rabbitmqPortsRepoRoot walks up from the test's directory to the module root.
func rabbitmqPortsRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "reached the filesystem root without finding go.mod")

		dir = parent
	}
}

// readKeyedFile parses a KEY=VALUE / KEY = VALUE file into a map, ignoring
// comments and blank lines. It serves both the dotenv files and rabbitmq.conf,
// whose shapes only differ in the spacing around the separator.
func readKeyedFile(t *testing.T, path string) map[string]string {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "cannot read %s", path)

	values := make(map[string]string)

	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return values
}

// TestRabbitMQPortVariablesMatchTheBrokerListeners pins the mapping the two port
// variable names invert. RABBITMQ_PORT_HOST is composed into the amqp:// URL the
// producer, the consumer and the readiness probe dial, so it must carry the
// broker's AMQP listener port; RABBITMQ_PORT_AMQP only qualifies the health-check
// host, so it must carry the management HTTP port.
//
// The assertion crosses file boundaries on purpose: the dotenv files and
// rabbitmq.conf are edited independently, and the names invite exactly the swap
// that leaves every connection dialling the management port over AMQP.
func TestRabbitMQPortVariablesMatchTheBrokerListeners(t *testing.T) {
	t.Parallel()

	root := rabbitmqPortsRepoRoot(t)

	broker := readKeyedFile(t, filepath.Join(root, "components", "infra", "rabbitmq", "etc", "rabbitmq.conf"))

	amqpPort := broker["listeners.tcp.default"]
	managementPort := broker["management.tcp.port"]

	require.NotEmpty(t, amqpPort, "rabbitmq.conf must declare listeners.tcp.default")
	require.NotEmpty(t, managementPort, "rabbitmq.conf must declare management.tcp.port")
	require.NotEqual(t, amqpPort, managementPort, "the two listeners must differ for this test to mean anything")

	for _, envFile := range []string{
		filepath.Join("components", "ledger", ".env.example"),
		filepath.Join("components", "infra", ".env.example"),
	} {
		t.Run(envFile, func(t *testing.T) {
			t.Parallel()

			env := readKeyedFile(t, filepath.Join(root, envFile))

			require.Equal(t, amqpPort, env["RABBITMQ_PORT_HOST"],
				"RABBITMQ_PORT_HOST is the AMQP port: it is what buildRabbitMQConnectionString dials")
			require.Equal(t, managementPort, env["RABBITMQ_PORT_AMQP"],
				"RABBITMQ_PORT_AMQP is the management HTTP port, not the AMQP port")
		})
	}
}
