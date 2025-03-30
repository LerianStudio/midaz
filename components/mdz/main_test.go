package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/root"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

// readCloserBuffer is a wrapper around bytes.Buffer that implements io.ReadCloser
type readCloserBuffer struct {
	*bytes.Buffer
}

func (r *readCloserBuffer) Close() error {
	return nil
}

func newReadCloserBuffer() *readCloserBuffer {
	return &readCloserBuffer{&bytes.Buffer{}}
}

func TestMainCommandExecution(t *testing.T) {
	// Save original args and restore them after the test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	tests := []struct {
		name          string
		args          []string
		expectedError bool
		outputCheck   func(t *testing.T, stdout, stderr string)
	}{
		{
			name:          "help command",
			args:          []string{"mdz", "--help"},
			expectedError: false,
			outputCheck: func(t *testing.T, stdout, stderr string) {
				assert.Contains(t, stdout, "Mdz CLI")
				assert.Contains(t, stdout, "AVAILABLE COMMANDS")
			},
		},
		{
			name:          "version command",
			args:          []string{"mdz", "version"},
			expectedError: false,
			outputCheck: func(t *testing.T, stdout, stderr string) {
				assert.Contains(t, stdout, "Mdz CLI")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up args for this test
			os.Args = tt.args

			// Create buffers to capture output
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			stdin := newReadCloserBuffer()

			// Create a new environment with test streams
			env := environment.New()
			
			// Create test IO streams
			io := &iostreams.IOStreams{
				In:  stdin,
				Out: stdout,
				Err: stderr,
			}

			// Create a factory with test environment
			f := factory.NewFactory(env)
			f.IOStreams = io

			// Create the root command
			cmd := root.NewCmdRoot(f)

			// Execute the command
			err := cmd.Execute()

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check output
			if tt.outputCheck != nil {
				tt.outputCheck(t, stdout.String(), stderr.String())
			}
		})
	}
}

// TestInvalidCommand tests that an invalid command returns an error
func TestInvalidCommand(t *testing.T) {
	// Save original args and restore them after the test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Set up args for this test
	os.Args = []string{"mdz", "invalid-command"}

	// Create a new environment
	env := environment.New()
	
	// Create a factory with the environment
	f := factory.NewFactory(env)

	// Create the root command
	cmd := root.NewCmdRoot(f)

	// Execute the command and verify it returns an error
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}

// TestMainFunction tests the components used by the main function
func TestMainFunction(t *testing.T) {
	// This is a test to ensure the main function components can be created without error
	env := environment.New()
	assert.NotNil(t, env)

	f := factory.NewFactory(env)
	assert.NotNil(t, f)

	cmd := root.NewCmdRoot(f)
	assert.NotNil(t, cmd)
	
	// Test that we can create a new command with the factory
	assert.NotPanics(t, func() {
		cmd := root.NewCmdRoot(f)
		cmd.SetArgs([]string{"--help"})
		cmd.Execute()
	})
}

// TestMain is a special function that allows us to set up and tear down test resources
func TestMain(m *testing.M) {
	// Run the tests
	exitCode := m.Run()
	
	// Exit with the same code
	os.Exit(exitCode)
}
