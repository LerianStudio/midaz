package mt001_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndToEndWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end tests in short mode")
	}

	t.Run("complete user journey", func(t *testing.T) {
		// Build the application first
		binaryPath := buildApplication(t)
		defer os.Remove(binaryPath)

		// Test 1: Version command
		t.Run("version command", func(t *testing.T) {
			output, err := runCommand(t, binaryPath, []string{"version"}, nil)
			assert.NoError(t, err)
			assert.Contains(t, output, "Demo Data Generator")
			assert.Contains(t, output, "Hexagonal")
		})

		// Test 2: Help command
		t.Run("help command", func(t *testing.T) {
			output, err := runCommand(t, binaryPath, []string{"--help"}, nil)
			assert.NoError(t, err)
			assert.Contains(t, output, "Available Commands")
			assert.Contains(t, output, "validate")
			assert.Contains(t, output, "test-connection")
		})

		// Test 3: Validation without config (should fail)
		t.Run("validation without config", func(t *testing.T) {
			_, err := runCommand(t, binaryPath, []string{"validate"}, nil)
			assert.Error(t, err, "Should fail without auth token")
		})

		// Test 4: Validation with environment config
		t.Run("validation with environment config", func(t *testing.T) {
			env := map[string]string{
				"DEMO_DATA_API_BASE_URL": "https://api.midaz.io",
				"DEMO_DATA_AUTH_TOKEN":   "test-token-12345",
			}

			output, err := runCommand(t, binaryPath, []string{"validate"}, env)
			assert.NoError(t, err)
			assert.Contains(t, output, "Configuration loaded and validated successfully")
			assert.Contains(t, output, "https://api.midaz.io")
		})

		// Test 5: Test connection
		t.Run("test connection", func(t *testing.T) {
			env := map[string]string{
				"DEMO_DATA_API_BASE_URL": "https://api.midaz.io",
				"DEMO_DATA_AUTH_TOKEN":   "test-token-12345",
			}

			output, err := runCommand(t, binaryPath, []string{"test-connection"}, env)
			assert.NoError(t, err)
			assert.Contains(t, output, "Testing connection to Midaz API")
		})

		// Test 6: Flag overrides
		t.Run("flag overrides", func(t *testing.T) {
			env := map[string]string{
				"DEMO_DATA_AUTH_TOKEN": "env-token",
			}

			output, err := runCommand(t, binaryPath, []string{
				"--api-url", "https://flag.override.com",
				"--debug",
				"validate",
			}, env)

			assert.NoError(t, err)
			assert.Contains(t, output, "https://flag.override.com")
			assert.Contains(t, output, "Debug Mode:        true")
		})

		// Test 7: .env file support
		t.Run(".env file support", func(t *testing.T) {
			// Create temporary .env file
			configContent := `DEMO_DATA_API_BASE_URL=https://file.config.com
DEMO_DATA_AUTH_TOKEN=file-token-12345
DEMO_DATA_DEBUG=false
DEMO_DATA_LOG_LEVEL=info
DEMO_DATA_TIMEOUT_DURATION=30s`

			tmpFile, err := os.CreateTemp("", ".env")
			require.NoError(t, err)
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString(configContent)
			require.NoError(t, err)
			tmpFile.Close()

			// Copy the .env file to the current directory since that's where the app looks for it
			envPath := ".env"
			defer os.Remove(envPath)

			envContent, err := os.ReadFile(tmpFile.Name())
			require.NoError(t, err)

			err = os.WriteFile(envPath, envContent, 0644)
			require.NoError(t, err)

			output, err := runCommand(t, binaryPath, []string{
				"validate",
			}, nil)

			assert.NoError(t, err)
			assert.Contains(t, output, "https://file.config.com")
		})
	})

	t.Run("error scenarios", func(t *testing.T) {
		binaryPath := buildApplication(t)
		defer os.Remove(binaryPath)

		// Test invalid command
		t.Run("invalid command", func(t *testing.T) {
			_, err := runCommand(t, binaryPath, []string{"invalid-command"}, nil)
			assert.Error(t, err)
		})

		// Test invalid flag
		t.Run("invalid flag", func(t *testing.T) {
			env := map[string]string{
				"DEMO_DATA_AUTH_TOKEN": "test-token",
			}
			_, err := runCommand(t, binaryPath, []string{"--invalid-flag", "validate"}, env)
			assert.Error(t, err)
		})

		// Test missing required auth token
		t.Run("missing auth token", func(t *testing.T) {
			_, err := runCommand(t, binaryPath, []string{"validate"}, nil)
			assert.Error(t, err)
		})
	})
}

func TestCrossплатформCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cross-platform tests in short mode")
	}

	t.Run("current platform build and execute", func(t *testing.T) {
		// This test ensures the build works on the current platform
		binaryPath := buildApplication(t)
		defer os.Remove(binaryPath)

		// Test basic functionality
		output, err := runCommand(t, binaryPath, []string{"version"}, nil)
		assert.NoError(t, err)
		assert.Contains(t, output, "Demo Data Generator")

		// Check platform information is correct
		expectedOS := runtime.GOOS
		expectedArch := runtime.GOARCH
		assert.Contains(t, output, expectedOS)
		assert.Contains(t, output, expectedArch)
	})
}

func TestDocumentationAccuracy(t *testing.T) {
	t.Run("help text accuracy", func(t *testing.T) {
		binaryPath := buildApplication(t)
		defer os.Remove(binaryPath)

		// Test main help
		output, err := runCommand(t, binaryPath, []string{"--help"}, nil)
		assert.NoError(t, err)

		// Verify all documented commands are present
		expectedCommands := []string{"validate", "test-connection", "version"}
		for _, cmd := range expectedCommands {
			assert.Contains(t, output, cmd, "Command %s should be documented in help", cmd)
		}

		// Verify all documented flags are present
		expectedFlags := []string{"--auth-token", "--api-url", "--debug", "--config", "--log-level"}
		for _, flag := range expectedFlags {
			assert.Contains(t, output, flag, "Flag %s should be documented in help", flag)
		}
	})

	t.Run("command specific help", func(t *testing.T) {
		binaryPath := buildApplication(t)
		defer os.Remove(binaryPath)

		commands := []string{"validate", "test-connection", "version"}

		for _, cmd := range commands {
			t.Run(cmd+" help", func(t *testing.T) {
				output, err := runCommand(t, binaryPath, []string{cmd, "--help"}, nil)
				assert.NoError(t, err)
				assert.Contains(t, output, cmd, "Command help should mention the command name")
			})
		}
	})
}

// Helper function to build the application and return the binary path
func buildApplication(t *testing.T) string {
	// Create temporary directory for the binary
	tmpDir, err := os.MkdirTemp("", "demo-data-test-*")
	require.NoError(t, err)

	binaryName := "demo-data-test"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(tmpDir, binaryName)

	// Build the application
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/demo-data")
	cmd.Dir = getProjectRoot(t)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Build failed: %s", string(output))

	// Verify the binary was created
	_, err = os.Stat(binaryPath)
	require.NoError(t, err, "Binary not found at %s", binaryPath)

	return binaryPath
}

// Helper function to run a command and return its output
func runCommand(t *testing.T, binaryPath string, args []string, env map[string]string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)

	// Set environment variables
	if env != nil {
		cmd.Env = os.Environ()
		for key, value := range env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}

	// Capture both stdout and stderr
	var stdout, stderr []byte
	var err error

	// Use pipes to capture output
	stdoutPipe, err := cmd.StdoutPipe()
	require.NoError(t, err)

	stderrPipe, err := cmd.StderrPipe()
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	// Read output
	stdout, _ = io.ReadAll(stdoutPipe)
	stderr, _ = io.ReadAll(stderrPipe)

	err = cmd.Wait()

	// Combine stdout and stderr for complete output
	output := string(stdout) + string(stderr)

	return output, err
}

// Helper function to get the project root directory
func getProjectRoot(t *testing.T) string {
	// Start from current directory and walk up to find go.mod
	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find project root (go.mod not found)")
		}
		dir = parent
	}
}
