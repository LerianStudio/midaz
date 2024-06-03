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

// RegexIgnoreAccents receives a regex, then, for each char it's adds the accents variations to expression
// Ex: Given "a" -> "aáàãâ"
// Ex: Given "c" -> "ç"
func RegexIgnoreAccents(regex string) string {
	m1 := map[string]string{
		"a": "[aáàãâ]",
		"e": "[eéèê]",
		"i": "[iíìî]",
		"o": "[oóòõô]",
		"u": "[uùúû]",
		"c": "[cç]",
		"A": "[AÁÀÃÂ]",
		"E": "[EÉÈÊ]",
		"I": "[IÍÌÎ]",
		"O": "[OÓÒÕÔ]",
		"U": "[UÙÚÛ]",
		"C": "[CÇ]",
	}
	m2 := map[string]string{
		"a": "a",
		"á": "a",
		"à": "a",
		"ã": "a",
		"â": "a",
		"e": "e",
		"é": "e",
		"è": "e",
		"ê": "e",
		"i": "i",
		"í": "i",
		"ì": "i",
		"î": "i",
		"o": "o",
		"ó": "o",
		"ò": "o",
		"õ": "o",
		"ô": "o",
		"u": "u",
		"ù": "u",
		"ú": "u",
		"û": "u",
		"c": "c",
		"ç": "c",
		"A": "A",
		"Á": "A",
		"À": "A",
		"Ã": "A",
		"Â": "A",
		"E": "E",
		"É": "E",
		"È": "E",
		"Ê": "E",
		"I": "I",
		"Í": "I",
		"Ì": "I",
		"Î": "I",
		"O": "O",
		"Ó": "O",
		"Ò": "O",
		"Õ": "O",
		"Ô": "O",
		"U": "U",
		"Ù": "U",
		"Ú": "U",
		"Û": "U",
		"C": "C",
		"Ç": "C",
	}
	s := ""

	for _, ch := range regex {
		c := string(ch)
		if v1, found := m2[c]; found {
			if v2, found2 := m1[v1]; found2 {
				s += v2
				continue
			}
		}

		s += string(ch)
	}

	return s
}

// RemoveChars from a string
func RemoveChars(str string, chars map[string]bool) string {
	s := ""

	for _, ch := range str {
		c := string(ch)
		if _, found := chars[c]; found {
			continue
		}

		s += string(ch)
	}

	return s
}
