// Package utils provides utility functions for MDZ CLI commands.
//
// This package contains helper functions for:
//   - Command formatting and display
//   - JSON file handling
//   - Interactive field assignment
//   - Date validation
//   - Pointer utilities
package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Format joins multiple command examples with newlines.
//
// Used for formatting command examples in Cobra command definitions.
//
// Parameters:
//   - commands: Variable number of command example strings
//
// Returns:
//   - string: Joined commands separated by newlines
func Format(commands ...string) string {
	return strings.Join(commands, "\n")
}

// FlagFileUnmarshalJSON reads and unmarshals JSON from a file or stdin.
//
// This function:
// 1. Opens file (or uses stdin if path is "-")
// 2. Reads JSON content
// 3. Unmarshals into provided struct
//
// Parameters:
//   - path: File path or "-" for stdin
//   - request: Pointer to struct to unmarshal into
//
// Returns:
//   - error: Error if file cannot be read or JSON is invalid
//
// TODO: CRITICAL - Bug in pointer handling: Line 65 passes &request (pointer to interface)
// and UnmarshalJSONFromReader takes &object again, resulting in **any being passed to
// json.Unmarshal. This causes silent failure - data unmarshals to temp variable, not the
// caller's struct. Fix: Pass request by value (remove &), and remove & in UnmarshalJSONFromReader.
// See PR#1394 kodus-ai review for details.
func FlagFileUnmarshalJSON(path string, request any) error {
	var (
		file *os.File
		err  error
	)

	if path == "-" {
		file = os.Stdin
	} else {
		file, err = os.Open(filepath.Clean(path))
		if err != nil {
			return errors.New("Failed to open a file. Verify if the path and file " +
				"exists and/or the file is corrupted and try the command again " + path)
		}
		defer file.Close()
	}

	return UnmarshalJSONFromReader(file, &request)
}

// UnmarshalJSONFromReader reads and unmarshals JSON from an io.Reader.
//
// Parameters:
//   - file: Reader to read JSON from
//   - object: Pointer to struct to unmarshal into
//
// Returns:
//   - error: Error if reading or unmarshalling fails
func UnmarshalJSONFromReader(file io.Reader, object any) error {
	jsonFile, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	err = json.Unmarshal(jsonFile, &object)
	if err != nil {
		return err
	}

	return nil
}

// WriteDetailsToFile writes data to a file, creating directories if needed.
//
// Parameters:
//   - data: Data to write
//   - outPath: Output file path
//
// Returns:
//   - error: Error if directory creation or file writing fails
func WriteDetailsToFile(data []byte, outPath string) error {
	err := os.MkdirAll(filepath.Dir(outPath), 0o750)
	if err != nil {
		return err
	}

	err = os.WriteFile(outPath, data, 0o600)
	if err != nil {
		return err
	}

	return nil
}

// FormatAskFieldRequired formats a prompt message for required field input.
//
// Parameters:
//   - field: Field name
//
// Returns:
//   - string: Formatted prompt message
func FormatAskFieldRequired(field string) string {
	return fmt.Sprintf("Enter the %s field", field)
}

// AssignStringField assigns a string value from flag or interactive input.
//
// If flagValue is empty, prompts user for input using inputFunc.
// Otherwise, returns flagValue.
//
// Parameters:
//   - flagValue: Value from command flag
//   - fieldName: Field name for prompt
//   - inputFunc: Function to prompt for input
//
// Returns:
//   - string: Assigned value
//   - error: Error if input fails
func AssignStringField(flagValue string, fieldName string, inputFunc func(string) (string, error)) (string, error) {
	if len(flagValue) < 1 {
		answer, err := inputFunc(fieldName)
		if err != nil {
			return "", err
		}

		return answer, nil
	}

	return flagValue, nil
}

// ParseAndAssign parses a string value and returns a pointer to the parsed value.
//
// If value is empty, returns nil. Otherwise, parses using parseFunc.
//
// Parameters:
//   - value: String value to parse
//   - parseFunc: Function to parse the string
//
// Returns:
//   - *T: Pointer to parsed value, or nil if value is empty
//   - error: Error if parsing fails
func ParseAndAssign[T any](value string, parseFunc func(string) (T, error)) (*T, error) {
	if len(value) == 0 {
		return nil, nil
	}

	parsedValue, err := parseFunc(value)
	if err != nil {
		return nil, err
	}

	return &parsedValue, nil
}

// AssignOptionalStringPtr returns a pointer to the string if not empty, nil otherwise.
//
// Parameters:
//   - flagValue: String value
//
// Returns:
//   - *string: Pointer to string if not empty, nil otherwise
func AssignOptionalStringPtr(flagValue string) *string {
	if len(flagValue) < 1 {
		return nil
	}

	return &flagValue
}

// SafeNestedString safely extracts a nested string pointer value.
//
// Returns empty string if parent is nil or field is nil.
//
// Parameters:
//   - parent: Pointer to parent struct
//   - fieldFunc: Function to extract string pointer from parent
//
// Returns:
//   - string: Field value or empty string if nil
func SafeNestedString[T any](parent *T, fieldFunc func(*T) *string) string {
	if parent == nil {
		return ""
	}

	value := fieldFunc(parent)
	if value == nil {
		return ""
	}

	return *value
}

// SafeString safely dereferences a string pointer.
//
// Returns empty string if pointer is nil.
//
// Parameters:
//   - value: String pointer
//
// Returns:
//   - string: Dereferenced value or empty string if nil
func SafeString(value *string) string {
	if value != nil {
		return *value
	}

	return ""
}

// ValidateDate validates a date string in YYYY-MM-DD format.
//
// Parameters:
//   - date: Date string to validate
//
// Returns:
//   - error: Error if date format is invalid
func ValidateDate(date string) error {
	const layout = "2006-01-02"

	_, err := time.Parse(layout, date)
	if err != nil {
		return errors.New("invalid date format: expected YYYY-MM-DD")
	}

	return nil
}
