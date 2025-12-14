package assert

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// TestThat_Pass verifies That does not panic when condition is true.
func TestThat_Pass(t *testing.T) {
	require.NotPanics(t, func() {
		That(true, "should not panic")
	})
}

// TestThat_Panic verifies That panics when condition is false.
func TestThat_Panic(t *testing.T) {
	require.Panics(t, func() {
		That(false, "should panic")
	})
}

// TestThat_PanicMessage verifies the panic message contains the expected content.
func TestThat_PanicMessage(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic")
		msg := r.(string)
		require.Contains(t, msg, "assertion failed:")
		require.Contains(t, msg, "test message")
		require.Contains(t, msg, "key1=value1")
		require.Contains(t, msg, "key2=42")
		require.Contains(t, msg, "stack trace:")
	}()
	That(false, "test message", "key1", "value1", "key2", 42)
}

// TestNotNil_Pass verifies NotNil does not panic for non-nil values.
func TestNotNil_Pass(t *testing.T) {
	require.NotPanics(t, func() {
		NotNil("hello", "string should not be nil")
	})
	require.NotPanics(t, func() {
		NotNil(42, "int should not be nil")
	})
	require.NotPanics(t, func() {
		x := new(int)
		NotNil(x, "pointer should not be nil")
	})
	require.NotPanics(t, func() {
		s := []int{1, 2, 3}
		NotNil(s, "slice should not be nil")
	})
	require.NotPanics(t, func() {
		m := map[string]int{"a": 1}
		NotNil(m, "map should not be nil")
	})
}

// TestNotNil_Panic verifies NotNil panics for nil values.
func TestNotNil_Panic(t *testing.T) {
	require.Panics(t, func() {
		NotNil(nil, "should panic for nil")
	})
}

// TestNotNil_TypedNil verifies NotNil correctly handles typed nil.
// A typed nil is when an interface holds a nil pointer of a concrete type.
func TestNotNil_TypedNil(t *testing.T) {
	var ptr *int = nil
	var iface interface{} = ptr // typed nil: interface is not nil, but value is

	require.Panics(t, func() {
		NotNil(iface, "should panic for typed nil")
	})
}

// TestNotNil_TypedNilSlice verifies NotNil handles typed nil slices.
func TestNotNil_TypedNilSlice(t *testing.T) {
	var s []int = nil
	var iface interface{} = s

	require.Panics(t, func() {
		NotNil(iface, "should panic for typed nil slice")
	})
}

// TestNotNil_TypedNilMap verifies NotNil handles typed nil maps.
func TestNotNil_TypedNilMap(t *testing.T) {
	var m map[string]int = nil
	var iface interface{} = m

	require.Panics(t, func() {
		NotNil(iface, "should panic for typed nil map")
	})
}

// TestNotNil_TypedNilChan verifies NotNil handles typed nil channels.
func TestNotNil_TypedNilChan(t *testing.T) {
	var ch chan int = nil
	var iface interface{} = ch

	require.Panics(t, func() {
		NotNil(iface, "should panic for typed nil channel")
	})
}

// TestNotNil_TypedNilFunc verifies NotNil handles typed nil functions.
func TestNotNil_TypedNilFunc(t *testing.T) {
	var fn func() = nil
	var iface interface{} = fn

	require.Panics(t, func() {
		NotNil(iface, "should panic for typed nil function")
	})
}

// TestNotEmpty_Pass verifies NotEmpty does not panic for non-empty strings.
func TestNotEmpty_Pass(t *testing.T) {
	require.NotPanics(t, func() {
		NotEmpty("hello", "should not panic")
	})
	require.NotPanics(t, func() {
		NotEmpty(" ", "whitespace is not empty")
	})
}

// TestNotEmpty_Panic verifies NotEmpty panics for empty strings.
func TestNotEmpty_Panic(t *testing.T) {
	require.Panics(t, func() {
		NotEmpty("", "should panic for empty string")
	})
}

// TestNoError_Pass verifies NoError does not panic when error is nil.
func TestNoError_Pass(t *testing.T) {
	require.NotPanics(t, func() {
		NoError(nil, "should not panic")
	})
}

// TestNoError_Panic verifies NoError panics when error is not nil.
func TestNoError_Panic(t *testing.T) {
	err := errors.New("test error")
	require.Panics(t, func() {
		NoError(err, "should panic")
	})
}

// TestNoError_PanicMessageContainsError verifies the error message and type are included in panic.
func TestNoError_PanicMessageContainsError(t *testing.T) {
	err := errors.New("specific test error")
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic")
		msg := r.(string)
		require.Contains(t, msg, "assertion failed:")
		require.Contains(t, msg, "operation failed")
		require.Contains(t, msg, "error=specific test error")
		require.Contains(t, msg, "error_type=*errors.errorString")
		require.Contains(t, msg, "context_key=context_value")
	}()
	NoError(err, "operation failed", "context_key", "context_value")
}

// TestNever_AlwaysPanics verifies Never always panics.
func TestNever_AlwaysPanics(t *testing.T) {
	require.Panics(t, func() {
		Never("unreachable code reached")
	})
}

// TestNever_PanicMessage verifies Never includes message and context.
func TestNever_PanicMessage(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic")
		msg := r.(string)
		require.Contains(t, msg, "assertion failed:")
		require.Contains(t, msg, "unreachable")
		require.Contains(t, msg, "state=invalid")
	}()
	Never("unreachable", "state", "invalid")
}

// TestOddKeyValuePairs verifies handling of odd number of key-value pairs.
func TestOddKeyValuePairs(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic")
		msg := r.(string)
		require.Contains(t, msg, "key1=value1")
		require.Contains(t, msg, "key2=MISSING_VALUE")
	}()
	That(false, "test", "key1", "value1", "key2")
}

// TestStackTraceIncluded verifies stack trace is present in panic message.
func TestStackTraceIncluded(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic")
		msg := r.(string)
		require.Contains(t, msg, "stack trace:")
		require.Contains(t, msg, "goroutine")
	}()
	That(false, "test")
}

// TestPositive tests the Positive predicate.
func TestPositive(t *testing.T) {
	tests := []struct {
		name     string
		n        int64
		expected bool
	}{
		{"positive", 1, true},
		{"large positive", 1000000, true},
		{"max int64", math.MaxInt64, true},
		{"zero", 0, false},
		{"negative", -1, false},
		{"large negative", -1000000, false},
		{"min int64", math.MinInt64, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, Positive(tt.n))
		})
	}
}

// TestNonNegative tests the NonNegative predicate.
func TestNonNegative(t *testing.T) {
	tests := []struct {
		name     string
		n        int64
		expected bool
	}{
		{"positive", 1, true},
		{"max int64", math.MaxInt64, true},
		{"zero", 0, true},
		{"negative", -1, false},
		{"large negative", -1000000, false},
		{"min int64", math.MinInt64, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, NonNegative(tt.n))
		})
	}
}

// TestNotZero tests the NotZero predicate.
func TestNotZero(t *testing.T) {
	tests := []struct {
		name     string
		n        int64
		expected bool
	}{
		{"positive", 1, true},
		{"negative", -1, true},
		{"zero", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, NotZero(tt.n))
		})
	}
}

// TestInRange tests the InRange predicate.
func TestInRange(t *testing.T) {
	tests := []struct {
		name     string
		n        int64
		min      int64
		max      int64
		expected bool
	}{
		{"in range", 5, 1, 10, true},
		{"at min", 1, 1, 10, true},
		{"at max", 10, 1, 10, true},
		{"below min", 0, 1, 10, false},
		{"above max", 11, 1, 10, false},
		{"negative range", -5, -10, -1, true},
		{"single value range", 5, 5, 5, true},
		{"inverted range always false", 5, 10, 1, false}, // min > max, fail-safe behavior
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, InRange(tt.n, tt.min, tt.max))
		})
	}
}

// TestValidUUID tests the ValidUUID predicate.
func TestValidUUID(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		expected bool
	}{
		{"valid uuid v4", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid uuid v1", "6ba7b810-9dad-11d1-80b4-00c04fd430c8", true},
		{"valid nil uuid", "00000000-0000-0000-0000-000000000000", true},
		{"empty string", "", false},
		{"invalid format", "not-a-uuid", false},
		{"missing hyphens", "550e8400e29b41d4a716446655440000", true}, // uuid package accepts this
		{"too short", "550e8400-e29b-41d4-a716", false},
		{"too long", "550e8400-e29b-41d4-a716-446655440000-extra", false},
		{"invalid characters", "550e8400-e29b-41d4-a716-44665544ZZZZ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, ValidUUID(tt.s))
		})
	}
}

// TestValidAmount tests the ValidAmount predicate.
func TestValidAmount(t *testing.T) {
	tests := []struct {
		name     string
		amount   decimal.Decimal
		expected bool
	}{
		{"zero", decimal.Zero, true},
		{"positive integer", decimal.NewFromInt(100), true},
		{"negative integer", decimal.NewFromInt(-100), true},
		{"two decimal places", decimal.NewFromFloat(123.45), true},
		{"max valid exponent", decimal.New(1, 18), true},
		{"min valid exponent", decimal.New(1, -18), true},
		{"exponent too large", decimal.New(1, 19), false},
		{"exponent too small", decimal.New(1, -19), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, ValidAmount(tt.amount))
		})
	}
}

// TestValidScale tests the ValidScale predicate.
func TestValidScale(t *testing.T) {
	tests := []struct {
		name     string
		scale    int
		expected bool
	}{
		{"zero", 0, true},
		{"two", 2, true},
		{"max valid", 18, true},
		{"negative", -1, false},
		{"too large", 19, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, ValidScale(tt.scale))
		})
	}
}

// TestPositiveDecimal tests the PositiveDecimal predicate.
func TestPositiveDecimal(t *testing.T) {
	tests := []struct {
		name     string
		amount   decimal.Decimal
		expected bool
	}{
		{"positive", decimal.NewFromFloat(1.5), true},
		{"small positive", decimal.NewFromFloat(0.001), true},
		{"zero", decimal.Zero, false},
		{"negative", decimal.NewFromFloat(-1.5), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, PositiveDecimal(tt.amount))
		})
	}
}

// TestNonNegativeDecimal tests the NonNegativeDecimal predicate.
func TestNonNegativeDecimal(t *testing.T) {
	tests := []struct {
		name     string
		amount   decimal.Decimal
		expected bool
	}{
		{"positive", decimal.NewFromFloat(1.5), true},
		{"zero", decimal.Zero, true},
		{"negative", decimal.NewFromFloat(-1.5), false},
		{"small negative", decimal.NewFromFloat(-0.001), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, NonNegativeDecimal(tt.amount))
		})
	}
}

// TestIsNil tests the isNil helper function directly.
func TestIsNil(t *testing.T) {
	t.Run("untyped nil", func(t *testing.T) {
		require.True(t, isNil(nil))
	})

	t.Run("typed nil pointer", func(t *testing.T) {
		var ptr *int = nil
		require.True(t, isNil(ptr))
	})

	t.Run("typed nil slice", func(t *testing.T) {
		var s []int = nil
		require.True(t, isNil(s))
	})

	t.Run("typed nil map", func(t *testing.T) {
		var m map[string]int = nil
		require.True(t, isNil(m))
	})

	t.Run("typed nil channel", func(t *testing.T) {
		var ch chan int = nil
		require.True(t, isNil(ch))
	})

	t.Run("typed nil func", func(t *testing.T) {
		var fn func() = nil
		require.True(t, isNil(fn))
	})

	t.Run("non-nil pointer", func(t *testing.T) {
		x := 42
		require.False(t, isNil(&x))
	})

	t.Run("non-nil slice", func(t *testing.T) {
		s := []int{1, 2, 3}
		require.False(t, isNil(s))
	})

	t.Run("empty but non-nil slice", func(t *testing.T) {
		s := []int{}
		require.False(t, isNil(s))
	})

	t.Run("non-nil map", func(t *testing.T) {
		m := map[string]int{}
		require.False(t, isNil(m))
	})

	t.Run("non-nil channel", func(t *testing.T) {
		ch := make(chan int)
		require.False(t, isNil(ch))
	})

	t.Run("non-nil func", func(t *testing.T) {
		fn := func() {}
		require.False(t, isNil(fn))
	})

	t.Run("value types are never nil", func(t *testing.T) {
		require.False(t, isNil(42))
		require.False(t, isNil("hello"))
		require.False(t, isNil(3.14))
		require.False(t, isNil(true))
		require.False(t, isNil(struct{}{}))
	})
}

// TestTruncateValue tests the truncateValue function for various input sizes.
func TestTruncateValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "short string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "long string",
			input:    strings.Repeat("a", 300),
			expected: strings.Repeat("a", 200) + "... (truncated 100 chars)",
		},
		{
			name:     "exactly max length",
			input:    strings.Repeat("b", 200),
			expected: strings.Repeat("b", 200),
		},
		{
			name:     "one over max length",
			input:    strings.Repeat("c", 201),
			expected: strings.Repeat("c", 200) + "... (truncated 1 chars)",
		},
		{
			name:     "integer value",
			input:    42,
			expected: "42",
		},
		{
			name:     "nil value",
			input:    nil,
			expected: "<nil>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := truncateValue(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

// TestTruncateValueInPanicContext verifies truncation is applied in panic messages.
func TestTruncateValueInPanicContext(t *testing.T) {
	longValue := strings.Repeat("x", 300)

	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic")
		msg := r.(string)
		require.Contains(t, msg, "... (truncated 100 chars)")
		require.NotContains(t, msg, strings.Repeat("x", 300))
	}()

	That(false, "test with long value", "long_key", longValue)
}

// TestPanicWithContext_FormatOutput verifies the formatting of panic messages.
func TestPanicWithContext_FormatOutput(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic")
		msg := r.(string)

		// Verify structure
		require.True(t, strings.HasPrefix(msg, "assertion failed: "))

		// Verify key-value formatting
		require.Contains(t, msg, "    string_key=string_value")
		require.Contains(t, msg, "    int_key=123")
		require.Contains(t, msg, "    bool_key=true")

		// Verify stack trace is present
		require.Contains(t, msg, "stack trace:")
	}()

	panicWithContext("test message",
		"string_key", "string_value",
		"int_key", 123,
		"bool_key", true,
	)
}
