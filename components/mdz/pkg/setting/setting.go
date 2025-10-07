// Package setting provides configuration persistence for the MDZ CLI.
//
// This package manages CLI configuration storage in the user's home directory,
// including authentication tokens, API endpoints, and credentials. Configuration
// is stored in TOML format at ~/.config/mdz/mdz.toml.
package setting

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/environment"
	"github.com/pelletier/go-toml/v2"
)

// Setting represents the CLI configuration structure.
//
// This struct holds all persistent configuration including:
//   - Token: Authentication token
//   - Env: Environment configuration (API URLs, credentials)
type Setting struct {
	Token string // OAuth access token
	environment.Env
}

// getPathSetting returns the path to the CLI configuration directory.
//
// Returns the path ~/.config/mdz/ where CLI configuration is stored.
//
// Returns:
//   - string: Configuration directory path
//   - error: Error if home directory cannot be determined
func getPathSetting() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Set the full path of the directory ~/.config/mdz/
	return filepath.Join(homeDir, ".config", "mdz"), nil
}

// Save persists CLI configuration to ~/.config/mdz/mdz.toml.
//
// This function:
// 1. Marshals Setting to TOML format
// 2. Creates ~/.config/mdz/ directory if it doesn't exist
// 3. Writes configuration to mdz.toml file
// 4. Sets file permissions to 0600 (user read/write only)
//
// Parameters:
//   - sett: Setting to save
//
// Returns:
//   - error: Error if marshalling or writing fails
func Save(sett Setting) error {
	b, err := toml.Marshal(sett)
	if err != nil {
		return errors.New("while marshalling toml file " + err.Error())
	}

	dir, err := getPathSetting()
	if err != nil {
		return err
	}

	// Create the directory if it doesn't exist
	err = os.MkdirAll(dir, 0o750)
	if err != nil {
		return err
	}

	filePath := filepath.Join(dir, "mdz.toml")

	err = os.WriteFile(filePath, b, 0o600)
	if err != nil {
		return err
	}

	return nil
}

// Read loads CLI configuration from ~/.config/mdz/mdz.toml.
//
// This function:
// 1. Creates configuration directory if it doesn't exist
// 2. Creates empty configuration file if it doesn't exist
// 3. Reads and unmarshals TOML configuration
// 4. Returns Setting struct
//
// Returns:
//   - *Setting: Loaded configuration
//   - error: Error if reading or parsing fails
func Read() (*Setting, error) {
	dir, err := getPathSetting()
	if err != nil {
		return nil, err
	}

	dir = filepath.Clean(dir)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.MkdirAll(dir, 0o750)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed check dir %s ", dir)
	}

	dir = filepath.Join(dir, "mdz.toml")
	dir = filepath.Clean(dir)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.WriteFile(dir, []byte(""), 0o600)
		if err != nil {
			return nil, fmt.Errorf("failed to create file %s: %v", dir, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed check dir %s ", dir)
	}

	fileContent, err := os.ReadFile(dir)
	if err != nil {
		return nil, errors.New("opening TOML file: " + err.Error())
	}

	var sett Setting
	if err := toml.Unmarshal(fileContent, &sett); err != nil {
		return nil, errors.New("decoding the TOML file: " + err.Error())
	}

	return &sett, nil
}
