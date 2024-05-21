package common

import (
	"bytes"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// RemoveAccents removes accents of a given word and returns it
func RemoveAccents(word string) (string, error) {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

	s, _, err := transform.String(t, word)
	if err != nil {
		return "", err
	}

	return s, nil
}

// RemoveSpaces removes spaces of a given word and returns it
func RemoveSpaces(word string) string {
	rr := make([]rune, 0, len(word))

	for _, r := range word {
		if !unicode.IsSpace(r) {
			rr = append(rr, r)
		}
	}

	return string(rr)
}

// IsNilOrEmpty returns a boolean indicating if a *string is nil or empty.
// It's use TrimSpace so, a string "  " and "" will be considered empty
func IsNilOrEmpty(s *string) bool {
	return s == nil || strings.TrimSpace(*s) == ""
}

// IsUpper check if string is lower
func IsUpper(s string) error {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return ValidationError{
				Code:    "0004",
				Title:   "Invalid Data provided.",
				Message: "Invalid Data provided.",
			}
		}
	}

	return nil
}

// CamelToSnakeCase converts a given camelCase string to snake_case format.
func CamelToSnakeCase(str string) string {
	var buffer bytes.Buffer

	for i, character := range str {
		if unicode.IsUpper(character) {
			if i > 0 {
				buffer.WriteString("_")
			}

			buffer.WriteRune(unicode.ToLower(character))
		} else {
			buffer.WriteString(string(character))
		}
	}

	return buffer.String()
}
