package queuekit

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Matcher defines a function that determines if a message matches criteria.
// Matchers operate on the raw Message before unmarshaling to allow filtering
// based on headers, routing keys, or raw body content.
type Matcher func(msg Message) bool

// MatchAll returns a matcher that requires all matchers to match.
func MatchAll(matchers ...Matcher) Matcher {
	return func(msg Message) bool {
		for _, m := range matchers {
			if !m(msg) {
				return false
			}
		}

		return true
	}
}

// MatchAny returns a matcher that requires at least one matcher to match.
func MatchAny(matchers ...Matcher) Matcher {
	return func(msg Message) bool {
		if len(matchers) == 0 {
			return true
		}

		for _, m := range matchers {
			if m(msg) {
				return true
			}
		}

		return false
	}
}

// MatchNone returns a matcher that inverts the result.
func MatchNone(matcher Matcher) Matcher {
	return func(msg Message) bool {
		return !matcher(msg)
	}
}

// MatchAlways returns a matcher that always matches.
func MatchAlways() Matcher {
	return func(msg Message) bool {
		return true
	}
}

// MatchNever returns a matcher that never matches.
func MatchNever() Matcher {
	return func(msg Message) bool {
		return false
	}
}

// MatchRoutingKey returns a matcher that checks the routing key.
func MatchRoutingKey(key string) Matcher {
	return func(msg Message) bool {
		return msg.RoutingKey == key
	}
}

// MatchRoutingKeyPrefix returns a matcher that checks the routing key prefix.
func MatchRoutingKeyPrefix(prefix string) Matcher {
	return func(msg Message) bool {
		return strings.HasPrefix(msg.RoutingKey, prefix)
	}
}

// MatchRoutingKeyPattern returns a matcher that checks the routing key against a regex.
func MatchRoutingKeyPattern(pattern string) Matcher {
	re := regexp.MustCompile(pattern)

	return func(msg Message) bool {
		return re.MatchString(msg.RoutingKey)
	}
}

// MatchHeader returns a matcher that checks for a header with a specific value.
func MatchHeader(key string, value any) Matcher {
	return func(msg Message) bool {
		if msg.Headers == nil {
			return false
		}

		v, ok := msg.Headers[key]
		if !ok {
			return false
		}

		return v == value
	}
}

// MatchHeaderExists returns a matcher that checks if a header exists.
func MatchHeaderExists(key string) Matcher {
	return func(msg Message) bool {
		if msg.Headers == nil {
			return false
		}

		_, ok := msg.Headers[key]

		return ok
	}
}

// MatchCorrelationID returns a matcher that checks the correlation ID.
func MatchCorrelationID(id string) Matcher {
	return func(msg Message) bool {
		return msg.CorrelationID == id
	}
}

// MatchMessageID returns a matcher that checks the message ID.
func MatchMessageID(id string) Matcher {
	return func(msg Message) bool {
		return msg.MessageID == id
	}
}

// MatchBodyContains returns a matcher that checks if the body contains a substring.
func MatchBodyContains(substr string) Matcher {
	return func(msg Message) bool {
		return strings.Contains(string(msg.Body), substr)
	}
}

// MatchBodyPattern returns a matcher that checks the body against a regex.
func MatchBodyPattern(pattern string) Matcher {
	re := regexp.MustCompile(pattern)

	return func(msg Message) bool {
		return re.Match(msg.Body)
	}
}

// MatchJSONField returns a matcher that checks a JSON field value.
// The path supports dot notation for nested fields (e.g., "user.id").
// For simple top-level fields, use just the field name (e.g., "jobId").
func MatchJSONField(path string, expectedValue any) Matcher {
	return func(msg Message) bool {
		var data map[string]any
		if err := json.Unmarshal(msg.Body, &data); err != nil {
			return false
		}

		value := getNestedValue(data, path)

		return compareValues(value, expectedValue)
	}
}

// MatchJSONFieldExists returns a matcher that checks if a JSON field exists.
func MatchJSONFieldExists(path string) Matcher {
	return func(msg Message) bool {
		var data map[string]any
		if err := json.Unmarshal(msg.Body, &data); err != nil {
			return false
		}

		return hasNestedValue(data, path)
	}
}

// MatchJSONFieldPattern returns a matcher that checks a JSON field against a regex.
func MatchJSONFieldPattern(path string, pattern string) Matcher {
	re := regexp.MustCompile(pattern)

	return func(msg Message) bool {
		var data map[string]any
		if err := json.Unmarshal(msg.Body, &data); err != nil {
			return false
		}

		value := getNestedValue(data, path)
		if value == nil {
			return false
		}

		switch v := value.(type) {
		case string:
			return re.MatchString(v)
		default:
			return false
		}
	}
}

// MatchContentType returns a matcher that checks the content type.
func MatchContentType(contentType string) Matcher {
	return func(msg Message) bool {
		return msg.ContentType == contentType
	}
}

// getNestedValue retrieves a value from a nested map using dot notation.
func getNestedValue(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	current := any(data)

	for _, part := range parts {
		if m, ok := current.(map[string]any); ok {
			current = m[part]
		} else {
			return nil
		}
	}

	return current
}

// hasNestedValue checks if a nested path exists in the map.
func hasNestedValue(data map[string]any, path string) bool {
	parts := strings.Split(path, ".")
	current := any(data)

	for i, part := range parts {
		if m, ok := current.(map[string]any); ok {
			v, exists := m[part]
			if !exists {
				return false
			}

			if i == len(parts)-1 {
				return true
			}

			current = v
		} else {
			return false
		}
	}

	return true
}

// compareValues compares two values for equality, handling type conversions.
func compareValues(actual, expected any) bool {
	if actual == nil && expected == nil {
		return true
	}

	if actual == nil || expected == nil {
		return false
	}

	// Handle numeric comparisons (JSON unmarshals numbers as float64)
	switch e := expected.(type) {
	case int:
		if f, ok := actual.(float64); ok {
			return f == float64(e)
		}
	case int64:
		if f, ok := actual.(float64); ok {
			return f == float64(e)
		}
	case float64:
		if f, ok := actual.(float64); ok {
			return f == e
		}
	}

	return actual == expected
}
