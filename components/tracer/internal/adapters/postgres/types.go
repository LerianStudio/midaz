// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"database/sql/driver"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// StringArray implements sql.Scanner and driver.Value for PostgreSQL text[] arrays.
// This provides compatibility with database/sql and sqlmock without requiring lib/pq.
//
// NULL handling follows the lib/pq convention:
//   - Database NULL -> nil slice (not empty slice)
//   - Empty array '{}' -> empty slice []string{}
//
// This distinction allows callers to differentiate between a NULL column
// (value was never set) and an explicitly empty array.
type StringArray []string

// Scan implements the sql.Scanner interface for PostgreSQL array parsing.
// Handles the PostgreSQL array literal format: {val1,val2,val3}
//
// NULL values are converted to nil (not an empty slice) following the lib/pq
// convention. This is intentional behavior that preserves the semantic
// difference between NULL and an empty array in PostgreSQL.
func (a *StringArray) Scan(src any) error {
	// NULL column -> nil slice (preserves NULL vs empty array distinction)
	// This follows the same pattern as lib/pq StringArray.Scan
	if src == nil {
		*a = nil
		return nil
	}

	var source string

	switch s := src.(type) {
	case string:
		source = s
	case []byte:
		source = string(s)
	default:
		return errors.New("incompatible type for StringArray")
	}

	// Handle empty array
	if source == "{}" {
		*a = StringArray{}
		return nil
	}

	// Parse PostgreSQL array format: {elem1,elem2,elem3}
	// Remove surrounding braces
	if len(source) < 2 || source[0] != '{' || source[len(source)-1] != '}' {
		return errors.New("invalid PostgreSQL array format")
	}

	content := source[1 : len(source)-1]
	if content == "" {
		*a = StringArray{}
		return nil
	}

	// Parse elements handling quoted strings and escapes
	elements, err := parseArrayElements(content)
	if err != nil {
		return err
	}

	*a = elements

	return nil
}

// Value implements the driver.Value interface for PostgreSQL array serialization.
func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}

	if len(a) == 0 {
		return "{}", nil
	}

	// Build PostgreSQL array literal with proper escaping
	var b strings.Builder
	b.WriteByte('{')

	for i, elem := range a {
		if i > 0 {
			b.WriteByte(',')
		}

		b.WriteString(quoteArrayElement(elem))
	}

	b.WriteByte('}')

	return b.String(), nil
}

// parseArrayElements parses comma-separated elements from PostgreSQL array content.
// Handles quoted strings and escape sequences.
// Returns an error if the content has unclosed quotes.
func parseArrayElements(content string) ([]string, error) {
	var result []string

	var current strings.Builder

	inQuotes := false
	escaped := false

	for i := 0; i < len(content); i++ {
		c := content[i]

		if escaped {
			current.WriteByte(c)

			escaped = false

			continue
		}

		switch c {
		case '\\':
			escaped = true
		case '"':
			inQuotes = !inQuotes
		case ',':
			if inQuotes {
				current.WriteByte(c)
			} else {
				result = append(result, unquoteElement(current.String()))
				current.Reset()
			}
		default:
			current.WriteByte(c)
		}
	}

	// Validate that all quotes are closed
	if inQuotes {
		return nil, errors.New("invalid PostgreSQL array format: unclosed quote")
	}

	// Handle trailing backslash (if escaped is still true, append it)
	if escaped {
		current.WriteByte('\\')
	}

	// Add last element - for non-empty content, always add at least one element.
	// This handles the case of a single empty quoted string like `""` where
	// current.Len() is 0 (quotes are consumed, not written) and len(result) is 0.
	if current.Len() > 0 || len(result) > 0 || len(content) > 0 {
		result = append(result, unquoteElement(current.String()))
	}

	return result, nil
}

// NullString is a sentinel value representing SQL NULL in array elements.
// This distinguishes SQL NULL from an empty string "" in PostgreSQL arrays.
//
// # Usage and Propagation
//
// When parsing PostgreSQL arrays containing NULL elements (e.g., '{a,NULL,b}'),
// the unquoteElement function returns this sentinel instead of the literal "NULL" string.
// This allows callers to differentiate between:
//   - An actual SQL NULL (no value was set) -> returns NullString sentinel
//   - A string containing "NULL" as text   -> returns "NULL" (quoted in source)
//   - An empty string ""                   -> returns "" (quoted as "" in source)
//
// # Caller Responsibility
//
// Functions that receive parsed array elements MUST check for NullString and handle
// it appropriately before using the values in business logic. For example:
//
//	for _, elem := range parsedArray {
//	    if elem == NullString {
//	        // Handle NULL case: skip, use default, or error
//	        continue
//	    }
//	    // Safe to use elem as a valid string
//	}
//
// Failure to check for NullString may result in the sentinel value ("\x00NULL\x00")
// propagating to business logic or being persisted, which is likely unintended.
//
// # Implementation Note
//
// The sentinel uses null bytes (\x00) as delimiters because they cannot appear
// in valid PostgreSQL text data (PostgreSQL rejects embedded null bytes).
const NullString = "\x00NULL\x00"

// unquoteElement removes surrounding quotes and handles escape sequences.
// Returns NullString for SQL NULL (unquoted NULL literal) to distinguish from empty strings.
// Callers MUST check for NullString sentinel before using returned values.
// See NullString documentation for handling guidelines.
func unquoteElement(s string) string {
	// Handle NULL (unquoted) - return sentinel to distinguish from empty string
	if s == "NULL" {
		return NullString
	}

	// Check if element is quoted
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		// Remove surrounding quotes - preserve internal spaces
		s = s[1 : len(s)-1]
		// Unescape backslash sequences using single-pass replacer to avoid
		// re-processing (e.g., `\\"` becoming `\"` then `"` with separate ReplaceAll calls)
		replacer := strings.NewReplacer(`\"`, `"`, `\\`, `\`)
		s = replacer.Replace(s)
	}

	return s
}

// quoteArrayElement quotes and escapes a string for PostgreSQL array literal.
func quoteArrayElement(s string) string {
	// Check if quoting is needed
	needsQuotes := s == "" || strings.ContainsAny(s, `{},"\`) || strings.ContainsAny(s, " \t\n\r")

	if !needsQuotes {
		return s
	}

	// Escape backslashes and quotes, then wrap in quotes
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)

	return `"` + s + `"`
}

// IsUniqueViolation checks if an error is a PostgreSQL unique constraint violation (SQLSTATE 23505).
// Works with pgx/v5 pgconn.PgError.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}

	return false
}

// IsUniqueViolationOf checks if an error is a unique constraint violation for a specific constraint name.
func IsUniqueViolationOf(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraintName
	}

	return false
}
