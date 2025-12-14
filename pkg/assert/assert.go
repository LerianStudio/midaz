package assert

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
)

// That panics if ok is false. Use for general-purpose assertions.
//
// Example:
//
//	assert.That(len(items) > 0, "items must not be empty", "count", len(items))
func That(ok bool, msg string, kv ...any) {
	if !ok {
		panicWithContext(msg, kv...)
	}
}

// NotNil panics if v is nil. This function correctly handles both untyped nil
// and typed nil (nil interface values with concrete types).
//
// Example:
//
//	assert.NotNil(config, "config must be initialized")
//	assert.NotNil(handler, "handler must not be nil", "name", handlerName)
func NotNil(v any, msg string, kv ...any) {
	if isNil(v) {
		panicWithContext(msg, kv...)
	}
}

// NotEmpty panics if s is an empty string.
//
// Example:
//
//	assert.NotEmpty(userID, "userID must be provided")
func NotEmpty(s string, msg string, kv ...any) {
	if s == "" {
		panicWithContext(msg, kv...)
	}
}

// NoError panics if err is not nil. The error message and type are
// automatically included in the panic context for debugging.
//
// Example:
//
//	result, err := compute()
//	assert.NoError(err, "compute must succeed", "input", input)
func NoError(err error, msg string, kv ...any) {
	if err != nil {
		// Prepend error and error_type to key-value pairs for richer debugging
		// errorKVPairs: 2 pairs added (error + error_type), each pair = 2 elements
		const errorKVPairs = 4

		kvWithError := make([]any, 0, len(kv)+errorKVPairs)
		kvWithError = append(kvWithError, "error", err.Error())
		kvWithError = append(kvWithError, "error_type", fmt.Sprintf("%T", err))
		kvWithError = append(kvWithError, kv...)
		panicWithContext(msg, kvWithError...)
	}
}

// Never always panics. Use for code paths that should be unreachable.
//
// Example:
//
//	switch status {
//	case Active:
//	    return handleActive()
//	case Inactive:
//	    return handleInactive()
//	default:
//	    assert.Never("unhandled status", "status", status)
//	}
func Never(msg string, kv ...any) {
	panicWithContext(msg, kv...)
}

const maxValueLength = 200 // Truncate values longer than this

// truncateValue truncates long values for logging safety.
// This prevents log bloat and reduces risk of sensitive data exposure.
func truncateValue(v any) string {
	s := fmt.Sprintf("%v", v)
	if len(s) <= maxValueLength {
		return s
	}

	return s[:maxValueLength] + "... (truncated " + fmt.Sprintf("%d", len(s)-maxValueLength) + " chars)"
}

// panicWithContext formats the message with key-value pairs and stack trace,
// then panics with the formatted message.
func panicWithContext(msg string, kv ...any) {
	var sb strings.Builder

	sb.WriteString("assertion failed: ")
	sb.WriteString(msg)

	// Format key-value pairs
	if len(kv) > 0 {
		sb.WriteString("\n")

		for i := 0; i < len(kv); i += 2 {
			var value any
			if i+1 < len(kv) {
				value = kv[i+1]
			} else {
				value = "MISSING_VALUE"
			}
			// Truncate large values to prevent log bloat and sensitive data exposure
			fmt.Fprintf(&sb, "    %v=%v\n", kv[i], truncateValue(value))
		}
	}

	// Append stack trace
	sb.WriteString("\nstack trace:\n")
	sb.WriteString(string(debug.Stack()))

	panic(sb.String())
}

// isNil checks if a value is nil, handling both untyped nil and typed nil
// (nil interface values with concrete types).
func isNil(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}
