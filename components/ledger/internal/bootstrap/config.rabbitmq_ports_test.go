// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"go/ast"
	"go/parser"
	"go/token"
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

// TestEveryAMQPDialUsesThePortHostField pins the claim the two variables' names
// invert, in the CODE rather than in the dotenv files.
//
// The test above crosses the dotenv files against rabbitmq.conf, which catches an
// operator swapping the two values. It cannot see the swap that matters more: a
// call site handing buildRabbitMQConnectionString the management port field, so
// every producer, consumer and readiness probe dials AMQP against the management
// listener. The documentation commit asserted which field each call site uses and
// executed nothing, so that assertion was prose.
//
// The port argument is read positionally from the call, which is what makes this
// fail on a swap: buildRabbitMQConnectionString(uri, user, pass, host, PORT,
// vhost).
func TestEveryAMQPDialUsesThePortHostField(t *testing.T) {
	t.Parallel()

	const (
		builder      = "buildRabbitMQConnectionString"
		portArgIndex = 4
		wantField    = "RabbitMQPortHost"
	)

	root := rabbitmqPortsRepoRoot(t)
	dir := filepath.Join(root, "components", "ledger", "internal", "bootstrap")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "read the bootstrap package")

	calls := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		fset := token.NewFileSet()

		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		require.NoErrorf(t, parseErr, "parse %s", entry.Name())

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			name, ok := call.Fun.(*ast.Ident)
			if !ok || name.Name != builder {
				return true
			}

			calls++

			require.Greaterf(t, len(call.Args), portArgIndex,
				"%s:%d: %s called with %d arguments; the port is argument %d",
				entry.Name(), fset.Position(call.Pos()).Line, builder, len(call.Args), portArgIndex+1)

			selector, ok := call.Args[portArgIndex].(*ast.SelectorExpr)
			require.Truef(t, ok,
				"%s:%d: the port argument must be a config field so this assertion can name it, got %T",
				entry.Name(), fset.Position(call.Pos()).Line, call.Args[portArgIndex])

			require.Equalf(t, wantField, selector.Sel.Name,
				"%s:%d: the amqp:// URL must be built from %s, the AMQP listener port. %s is the management HTTP port, and the variable names invite exactly this swap",
				entry.Name(), fset.Position(call.Pos()).Line, wantField, selector.Sel.Name)

			return true
		})
	}

	require.Equalf(t, 3, calls,
		"expected the three known %s call sites (producer, consumer, readiness probe); found %d. A new dial must be checked here too, and a walk that finds none passes silently",
		builder, calls)
}
