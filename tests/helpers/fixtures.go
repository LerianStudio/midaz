package helpers

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

// LoadFixtureRaw reads a fixture file from tests/fixtures, applies string replacements, and returns bytes.
// The path is relative to the repository root (e.g., "tests/fixtures/onboarding/create_organization.json").
func LoadFixtureRaw(path string, replacements map[string]string) ([]byte, error) {
    data, err := os.ReadFile(filepath.Clean(path))
    if err != nil { return nil, fmt.Errorf("read fixture: %w", err) }
    s := string(data)
    for k, v := range replacements {
        s = strings.ReplaceAll(s, k, v)
    }
    return []byte(s), nil
}

// LoadFixtureJSON loads a JSON fixture and unmarshals into a generic map or slice.
func LoadFixtureJSON(path string, replacements map[string]string) (any, error) {
    raw, err := LoadFixtureRaw(path, replacements)
    if err != nil { return nil, err }
    var v any
    if err := json.Unmarshal(raw, &v); err != nil { return nil, fmt.Errorf("unmarshal fixture: %w", err) }
    return v, nil
}

