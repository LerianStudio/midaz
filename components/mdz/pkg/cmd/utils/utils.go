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

// \1 performs an operation
func Format(commands ...string) string {
	return strings.Join(commands, "\n")
}

// \1 performs an operation
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

// \1 performs an operation
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

// \1 performs an operation
func WriteDetailsToFile(data []byte, outPath string) error {
	err := os.MkdirAll(filepath.Dir(outPath), 0750)

	if err != nil {
		return err
	}

	err = os.WriteFile(outPath, data, 0600)
	if err != nil {
		return err
	}

	return nil
}

// \1 performs an operation
func FormatAskFieldRequired(field string) string {
	return fmt.Sprintf("Enter the %s field", field)
}

// \1 performs an operation
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

// \1 performs an operation
func AssignOptionalStringPtr(flagValue string) *string {
	if len(flagValue) < 1 {
		return nil
	}

	return &flagValue
}

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

// \1 performs an operation
func SafeString(value *string) string {
	if value != nil {
		return *value
	}

	return ""
}

// \1 performs an operation
func ValidateDate(date string) error {
	const layout = "2006-01-02"

	_, err := time.Parse(layout, date)

	if err != nil {
		return errors.New("invalid date format: expected YYYY-MM-DD")
	}

	return nil
}
