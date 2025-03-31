package setting

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestEnv creates a temporary home directory for testing
func setupTestEnv(t *testing.T) (string, func()) {
	// Create a temporary directory to serve as the home directory
	tempDir, err := os.MkdirTemp("", "mdz-test-*")
	require.NoError(t, err)

	// Save the original HOME environment variable
	originalHome := os.Getenv("HOME")

	// Set the HOME environment variable to our temporary directory
	err = os.Setenv("HOME", tempDir)
	require.NoError(t, err)

	// Return a cleanup function
	cleanup := func() {
		// Restore the original HOME environment variable
		os.Setenv("HOME", originalHome)
		// Remove the temporary directory
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup
}

func TestGetPathSetting(t *testing.T) {
	// Setup test environment
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Call getPathSetting
	path, err := getPathSetting()
	require.NoError(t, err)

	// Verify the path is correct
	expectedPath := filepath.Join(tempDir, ".config", "mdz")
	assert.Equal(t, expectedPath, path)
}

func TestSave(t *testing.T) {
	// Setup test environment
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create the config directory
	configDir := filepath.Join(tempDir, ".config", "mdz")
	err := os.MkdirAll(configDir, 0750)
	require.NoError(t, err)

	// Create a test setting
	testSetting := Setting{
		ClientID:          "test-client-id",
		ClientSecret:      "test-client-secret",
		URLAPIAuth:        "https://auth.example.com",
		URLAPILedger:      "https://ledger.example.com",
		URLAPITransaction: "https://transaction.example.com",
	}

	// Save the setting
	err = Save(testSetting)
	require.NoError(t, err)

	// Verify the file exists
	filePath := filepath.Join(configDir, "mdz.toml")
	_, err = os.Stat(filePath)
	require.NoError(t, err)

	// Read the file content to verify it contains the expected values
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	contentStr := string(content)

	// Check for the presence of the values in the content
	assert.Contains(t, contentStr, "test-client-id")
	assert.Contains(t, contentStr, "test-client-secret")
	assert.Contains(t, contentStr, "https://auth.example.com")
	assert.Contains(t, contentStr, "https://ledger.example.com")
	assert.Contains(t, contentStr, "https://transaction.example.com")
}

func TestRead(t *testing.T) {
	// Setup test environment
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create the config directory
	configDir := filepath.Join(tempDir, ".config", "mdz")
	err := os.MkdirAll(configDir, 0750)
	require.NoError(t, err)

	// Create an empty file
	filePath := filepath.Join(configDir, "mdz.toml")
	err = os.WriteFile(filePath, []byte(""), 0600)
	require.NoError(t, err)

	// Read the setting
	readSetting, err := Read()
	require.NoError(t, err)

	// Verify the setting is empty
	assert.Equal(t, "", readSetting.Token)
	assert.Equal(t, "", readSetting.ClientID)
	assert.Equal(t, "", readSetting.ClientSecret)
	assert.Equal(t, "", readSetting.URLAPIAuth)
	assert.Equal(t, "", readSetting.URLAPILedger)
	assert.Equal(t, "", readSetting.URLAPITransaction)
}

func TestReadNonExistentDirectory(t *testing.T) {
	// Setup test environment with a non-existent directory
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Remove the temp directory to simulate a non-existent directory
	os.RemoveAll(tempDir)

	// Read should create the directory and file
	readSetting, err := Read()
	require.NoError(t, err)

	// Verify the setting is empty
	assert.Equal(t, "", readSetting.Token)
}
