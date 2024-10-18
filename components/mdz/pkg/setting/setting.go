package setting

import (
	"os"
	"path/filepath"
)

type Setting struct {
	Token string
}

// Save saves the b file in the ~/.config/mdz/mdz.toml directory, creating the directory if necessary.
func (s *Setting) Save(b []byte) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Set the full path of the directory ~/.config/mdz/
	dir := filepath.Join(homeDir, ".config", "mdz")

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
