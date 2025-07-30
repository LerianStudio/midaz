package setting

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/environment"
	"github.com/pelletier/go-toml/v2"
)

type Setting struct {
	Token string
	environment.Env
}

// getPathSetting returns the path of the configuration directory of ~/.config/mdz/.
// this is the path where some cli configs will be persisted
func getPathSetting() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Set the full path of the directory ~/.config/mdz/
	return filepath.Join(homeDir, ".config", "mdz"), nil
}

// Save saves the b file in the ~/.config/mdz/mdz.toml directory, creating the directory if necessary.
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
	err = os.MkdirAll(dir, 0750)
	if err != nil {
		return err
	}

	filePath := filepath.Join(dir, "mdz.toml")

	err = os.WriteFile(filePath, b, 0600)
	if err != nil {
		return err
	}

	return nil
}

// Read loads the configuration TOML file and deserializes it to the struct Setting.
func Read() (*Setting, error) {
	dir, err := getPathSetting()
	if err != nil {
		return nil, err
	}

	dir = filepath.Clean(dir)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.MkdirAll(dir, 0750)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed check dir %s ", dir)
	}

	dir = filepath.Join(dir, "mdz.toml")
	dir = filepath.Clean(dir)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.WriteFile(dir, []byte(""), 0600)
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
