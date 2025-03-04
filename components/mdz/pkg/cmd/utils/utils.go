package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func Format(commands ...string) string {
	return strings.Join(commands, "\n")
}

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

func FormatAskFieldRequired(field string) string {
	return fmt.Sprintf("Enter the %s field", field)
}

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

func SafeString(value *string) string {
	if value != nil {
		return *value
	}

	return ""
}

func ValidateDate(date string) error {
	const layout = "2006-01-02"

	_, err := time.Parse(layout, date)
	if err != nil {
		return errors.New("invalid date format: expected YYYY-MM-DD")
	}

	return nil
}

// ParseBool parses a string to a boolean value. It accepts "true", "false", "1", "0", "yes", "no", "y", "n"
func ParseBool(s string) (bool, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	switch s {
	case "true", "1", "yes", "y":
		return true, nil
	case "false", "0", "no", "n", "":
		return false, nil
	default:
		// Try to parse as integer
		if i, err := strconv.Atoi(s); err == nil {
			return i != 0, nil
		}

		return false, fmt.Errorf("invalid boolean value: %s", s)
	}
}

// TruncateString truncates a string to the specified length, adding "..." if truncated
func TruncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3] + "..."
}

// PrintTable prints data in tabular format
func PrintTable(headers []string, data [][]string) error {
	// Calculate column widths
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	
	// Update column widths based on data
	for _, row := range data {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	
	// Print headers
	for i, header := range headers {
		fmt.Printf("%-*s", widths[i]+2, header)
	}
	fmt.Println()
	
	// Print separator line
	for _, width := range widths {
		fmt.Print(strings.Repeat("-", width) + "  ")
	}
	fmt.Println()
	
	// Print data rows
	for _, row := range data {
		for i, cell := range row {
			if i < len(widths) {
				fmt.Printf("%-*s", widths[i]+2, cell)
			}
		}
		fmt.Println()
	}
	
	return nil
}
