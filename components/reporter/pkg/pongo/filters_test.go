// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"testing"

	"github.com/flosch/pongo2/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPercentOfFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		num      any
		total    any
		expect   string
		hasError bool
	}{
		{"basic", 25, 100, "25.00%", false},
		{"fraction", 1, 4, "25.00%", false},
		{"string_inputs", "500", "1000", "50.00%", false},
		{"zero_denominator", 10, 0, "NaN", true},
		{"invalid_input", "abc", 100, "NaN", true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			val, err := percentOfFilter(pongo2.AsValue(test.num), pongo2.AsValue(test.total))
			t.Logf("num=%v, total=%v → output=%s, err=%v", test.num, test.total, val.String(), err)

			if test.hasError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			assert.Equal(t, test.expect, val.String())
		})
	}
}

func TestStripZerosFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"integer", 100, "100"},
		{"int64", int64(200), "200"},
		{"float_with_zeros", 3.14000, "3.14"},
		{"float_whole_number", 5.0, "5"},
		{"string_decimal", "123.45000", "123.45"},
		{"string_integer", "100", "100"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			val, err := stripZerosFilter(pongo2.AsValue(test.input), pongo2.AsValue(""))
			assert.Nil(t, err)
			assert.Equal(t, test.expected, val.String())
		})
	}
}

func TestStripZerosFilter_InvalidString(t *testing.T) {
	t.Parallel()
	val, err := stripZerosFilter(pongo2.AsValue("not_a_number"), pongo2.AsValue(""))
	assert.NotNil(t, err)
	assert.Equal(t, "strip_zeros", err.Sender)
	assert.NotNil(t, err.OrigError)
	assert.Equal(t, "NaN", val.String())
}

func TestSliceFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		param    string
		expected string
		hasError bool
	}{
		{"basic_slice", "Hello World", "0:5", "Hello", false},
		{"middle_slice", "Hello World", "6:11", "World", false},
		{"full_string", "Test", "0:4", "Test", false},
		{"empty_result", "Test", "0:0", "", false},
		{"out_of_bounds_end", "Test", "0:100", "Test", false},
		{"invalid_format", "Test", "0-5", "", true},
		{"invalid_start", "Test", "abc:5", "", true},
		{"invalid_end", "Test", "0:xyz", "", true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			val, err := sliceFilter(pongo2.AsValue(test.input), pongo2.AsValue(test.param))

			if test.hasError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, test.expected, val.String())
			}
		})
	}
}

func TestSliceFilter_NegativeStart(t *testing.T) {
	t.Parallel()
	val, err := sliceFilter(pongo2.AsValue("Hello"), pongo2.AsValue("-5:5"))
	assert.Nil(t, err)
	assert.Equal(t, "Hello", val.String())
}

func TestSliceFilter_StartGreaterThanEnd(t *testing.T) {
	t.Parallel()
	val, err := sliceFilter(pongo2.AsValue("Hello"), pongo2.AsValue("5:2"))
	assert.Nil(t, err)
	assert.Equal(t, "", val.String())
}

func TestEvaluateArithmeticExpression_BasicOperations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		expression string
		expected   float64
		wantErr    bool
	}{
		{"addition", "5+3", 8, false},
		{"subtraction", "10-4", 6, false},
		{"multiplication", "6*7", 42, false},
		{"division", "20/4", 5, false},
		{"power", "2**3", 8, false},
		{"complex_expression", "2+3*4", 14, false},
		{"parentheses", "(2+3)*4", 20, false},
		{"negative_number", "-5+10", 5, false},
		{"decimal_numbers", "3.5*2", 7, false},
		{"division_decimal_result", "7/2", 3.5, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := evaluateArithmeticExpression(tt.expression)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.0001)
			}
		})
	}
}

func TestEvaluateArithmeticExpression_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		expression  string
		errContains string
	}{
		{"empty_expression", "", "empty expression"},
		{"division_by_zero", "10/0", "division by zero"},
		{"invalid_character", "5+abc", "unexpected character"},
		{"unmatched_parentheses", "(5+3", "unmatched parentheses"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := evaluateArithmeticExpression(tt.expression)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestEvaluateArithmeticExpression_Precedence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		expression string
		expected   float64
	}{
		{"mult_before_add", "2+3*4", 14},
		{"div_before_sub", "10-6/2", 7},
		{"power_before_mult", "2*3**2", 18},
		{"nested_parentheses", "((2+3)*4)+1", 21},
		{"multiple_operations", "1+2*3-4/2+5", 10},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := evaluateArithmeticExpression(tt.expression)
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}

func TestParseNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		input          string
		expectedStr    string
		expectedLength int
	}{
		{"simple_integer", "123", "123", 3},
		{"decimal_number", "3.14", "3.14", 4},
		{"negative_number", "-42", "-42", 3},
		{"number_followed_by_operator", "123+", "123", 3},
		{"empty_string", "", "", 0},
		{"starts_with_non_digit", "abc", "", 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			str, length := parseNumber(tt.input)
			assert.Equal(t, tt.expectedStr, str)
			assert.Equal(t, tt.expectedLength, length)
		})
	}
}

func TestIsDigit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		char     byte
		expected bool
	}{
		{'0', true},
		{'5', true},
		{'9', true},
		{'a', false},
		{'z', false},
		{'.', false},
		{'-', false},
		{'+', false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(string(tt.char), func(t *testing.T) {
			t.Parallel()
			result := isDigit(tt.char)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    float64
		expected string
	}{
		{"integer_as_float", 42.0, "42.0000000000"},
		{"decimal", 3.14, "3.1400000000"},
		{"negative", -5.5, "-5.5000000000"},
		{"zero", 0.0, "0.0000000000"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := formatNumber(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTokenizeExpression(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		expression string
		wantErr    bool
		numTokens  int
	}{
		{"simple_addition", "2+3", false, 3},
		{"multiple_operations", "1+2*3", false, 5},
		{"negative_number_at_start", "-5+3", false, 3},
		{"power_operator", "2**3", false, 3},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tokens, err := tokenizeExpression(tt.expression)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, tokens, tt.numTokens)
			}
		})
	}
}

func TestEvaluateTokens_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		tokens  []token
		want    float64
		wantErr bool
	}{
		{"empty_tokens", []token{}, 0, true},
		{"single_number", []token{{isOperator: false, value: 42}}, 42, false},
		{"single_operator", []token{{isOperator: true, operator: "+"}}, 0, true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := evaluateTokens(tt.tokens)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.want, result, 0.0001)
			}
		})
	}
}

func TestEvaluateWithParentheses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		expression string
		expected   float64
		wantErr    bool
	}{
		{"simple_parentheses", "(5+3)", 8, false},
		{"nested_parentheses", "((2+3)*4)", 20, false},
		{"parentheses_change_precedence", "(2+3)*4", 20, false},
		{"multiple_parentheses_groups", "(2+3)*(4+1)", 25, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := evaluateWithParentheses(tt.expression)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.0001)
			}
		})
	}
}

func TestToken_Struct(t *testing.T) {
	t.Parallel()
	numToken := token{isOperator: false, value: 42.5}
	assert.False(t, numToken.isOperator)
	assert.Equal(t, 42.5, numToken.value)

	opToken := token{isOperator: true, operator: "+"}
	assert.True(t, opToken.isOperator)
	assert.Equal(t, "+", opToken.operator)
}

// ---------------------------------------------------------------------------
// evaluatePowerTokens error paths
// ---------------------------------------------------------------------------

func TestEvaluatePowerTokens_ErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tokens      []token
		errContains string
	}{
		{
			name: "power_operator_at_position_zero",
			tokens: []token{
				{isOperator: true, operator: "**"},
				{isOperator: false, value: 3},
			},
			errContains: "invalid ** operator position",
		},
		{
			name: "power_operator_at_last_position",
			tokens: []token{
				{isOperator: false, value: 2},
				{isOperator: true, operator: "**"},
			},
			errContains: "invalid ** operator position",
		},
		{
			name: "adjacent_power_operators",
			tokens: []token{
				{isOperator: false, value: 2},
				{isOperator: true, operator: "**"},
				{isOperator: true, operator: "**"},
				{isOperator: false, value: 3},
			},
			errContains: "missing operand for ** operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := evaluatePowerTokens(tt.tokens)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// ---------------------------------------------------------------------------
// parseMinusToken edge cases
// ---------------------------------------------------------------------------

func TestParseMinusToken_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expression  string
		expected    float64
		wantErr     bool
		errContains string
	}{
		{
			name:       "minus_as_operator_between_numbers",
			expression: "10-3",
			expected:   7,
		},
		{
			name:       "negative_sign_at_start",
			expression: "-7+2",
			expected:   -5,
		},
		{
			name:       "negative_after_operator",
			expression: "5*-2",
			expected:   -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := evaluateArithmeticExpression(tt.expression)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.0001)
			}
		})
	}
}

func TestParseMinusToken_NegativeSignWithoutDigits(t *testing.T) {
	t.Parallel()
	// A bare minus at the end of the expression: negative sign with no following digits
	_, err := evaluateArithmeticExpression("5+-")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid number")
}

// ---------------------------------------------------------------------------
// evaluateAddSubTokens error paths
// ---------------------------------------------------------------------------

func TestEvaluateAddSubTokens_ErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tokens      []token
		errContains string
	}{
		{
			name:        "empty_expression",
			tokens:      []token{},
			errContains: "empty expression",
		},
		{
			name: "operator_at_start",
			tokens: []token{
				{isOperator: true, operator: "+"},
				{isOperator: false, value: 5},
			},
			errContains: "expression cannot start with operator",
		},
		{
			name: "missing_operand_after_operator",
			tokens: []token{
				{isOperator: false, value: 5},
				{isOperator: true, operator: "+"},
			},
			errContains: "missing operand after operator",
		},
		{
			name: "two_consecutive_operators",
			tokens: []token{
				{isOperator: false, value: 5},
				{isOperator: true, operator: "+"},
				{isOperator: true, operator: "-"},
				{isOperator: false, value: 3},
			},
			errContains: "expected number, got operator",
		},
		{
			name: "two_consecutive_numbers_without_operator",
			tokens: []token{
				{isOperator: false, value: 5},
				{isOperator: false, value: 3},
			},
			errContains: "expected operator at position",
		},
		{
			name: "single_operator_token",
			tokens: []token{
				{isOperator: true, operator: "-"},
			},
			errContains: "expected number, got operator",
		},
		{
			name: "unexpected_operator_in_addsub_pass",
			tokens: []token{
				{isOperator: false, value: 5},
				{isOperator: true, operator: "*"},
				{isOperator: false, value: 3},
			},
			errContains: "unexpected operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := evaluateAddSubTokens(tt.tokens)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// ---------------------------------------------------------------------------
// evaluateMulDivTokens error paths
// ---------------------------------------------------------------------------

func TestEvaluateMulDivTokens_ErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tokens      []token
		errContains string
	}{
		{
			name: "multiply_at_position_zero",
			tokens: []token{
				{isOperator: true, operator: "*"},
				{isOperator: false, value: 3},
			},
			errContains: "invalid * operator position",
		},
		{
			name: "divide_at_last_position",
			tokens: []token{
				{isOperator: false, value: 5},
				{isOperator: true, operator: "/"},
			},
			errContains: "invalid / operator position",
		},
		{
			name: "missing_operand_for_multiply",
			tokens: []token{
				{isOperator: false, value: 5},
				{isOperator: true, operator: "*"},
				{isOperator: true, operator: "+"},
				{isOperator: false, value: 3},
			},
			errContains: "missing operand for * operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := evaluateMulDivTokens(tt.tokens)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// ---------------------------------------------------------------------------
// stripZerosFilter - default (fallback) type branch
// ---------------------------------------------------------------------------

func TestStripZerosFilter_FallbackType(t *testing.T) {
	t.Parallel()
	// Pass a bool value, which hits the default branch (fallback to string formatting)
	val, err := stripZerosFilter(pongo2.AsValue(true), pongo2.AsValue(""))
	assert.Nil(t, err)
	assert.Equal(t, "true", val.String())
}

func TestStripZerosFilter_FallbackStructType(t *testing.T) {
	t.Parallel()
	type custom struct{ X int }
	val, err := stripZerosFilter(pongo2.AsValue(custom{X: 42}), pongo2.AsValue(""))
	assert.Nil(t, err)
	assert.Equal(t, "{42}", val.String())
}

// ---------------------------------------------------------------------------
// percentOfFilter - additional type coverage
// ---------------------------------------------------------------------------

func TestPercentOfFilter_Int64Inputs(t *testing.T) {
	t.Parallel()
	val, err := percentOfFilter(pongo2.AsValue(int64(50)), pongo2.AsValue(int64(200)))
	assert.Nil(t, err)
	assert.Equal(t, "25.00%", val.String())
}

func TestPercentOfFilter_Float64Inputs(t *testing.T) {
	t.Parallel()
	val, err := percentOfFilter(pongo2.AsValue(25.0), pongo2.AsValue(50.0))
	assert.Nil(t, err)
	assert.Equal(t, "50.00%", val.String())
}

func TestPercentOfFilter_UnsupportedType(t *testing.T) {
	t.Parallel()
	// bool is not a supported type in toDec -> triggers unsupported type error
	val, err := percentOfFilter(pongo2.AsValue(true), pongo2.AsValue(100))
	assert.NotNil(t, err)
	assert.Equal(t, "NaN", val.String())
}

func TestPercentOfFilter_UnsupportedDenominatorType(t *testing.T) {
	t.Parallel()
	// unsupported type for denominator
	val, err := percentOfFilter(pongo2.AsValue(50), pongo2.AsValue(true))
	assert.NotNil(t, err)
	assert.Equal(t, "NaN", val.String())
}

// ---------------------------------------------------------------------------
// parseNumberToken error path
// ---------------------------------------------------------------------------

func TestParseNumberToken_OnlyDecimalPoint(t *testing.T) {
	t.Parallel()
	// A lone decimal point should fail to produce a valid number (hasDigit=false)
	numStr, length := parseNumber(".")
	assert.Equal(t, "", numStr)
	assert.Equal(t, 0, length)
}

func TestParseNumberToken_LoneDotInExpression(t *testing.T) {
	t.Parallel()
	// A lone "." triggers parseNumberToken with length=0, exercising the error branch
	_, err := evaluateArithmeticExpression("5+.")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid number")
}

func TestParseNumberToken_DirectCall(t *testing.T) {
	t.Parallel()
	// Directly call parseNumberToken with a lone dot to hit length==0 error path
	_, _, err := parseNumberToken(".", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid number at position 0")
}

func TestParseNumber_NegativeWithoutDigits(t *testing.T) {
	t.Parallel()
	// Just a negative sign with no digits after it
	numStr, length := parseNumber("-")
	assert.Equal(t, "", numStr)
	assert.Equal(t, 0, length)
}

func TestParseNumber_NegativeFollowedByNonDigit(t *testing.T) {
	t.Parallel()
	// Negative sign followed by a non-digit, non-dot character
	numStr, length := parseNumber("-abc")
	assert.Equal(t, "", numStr)
	assert.Equal(t, 0, length)
}

// ---------------------------------------------------------------------------
// evaluateWithParentheses error paths
// ---------------------------------------------------------------------------

func TestEvaluateWithParentheses_InnerExpressionError(t *testing.T) {
	t.Parallel()
	// Inner expression contains an error (empty parens)
	_, err := evaluateWithParentheses("()")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty expression")
}

func TestEvaluateWithParentheses_UnmatchedOpenParen(t *testing.T) {
	t.Parallel()
	_, err := evaluateWithParentheses("(5+3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmatched parentheses")
}

func TestEvaluateWithParentheses_NoParentheses(t *testing.T) {
	t.Parallel()
	// Expression without parentheses goes through the no-paren branch
	result, err := evaluateWithParentheses("5+3*2")
	require.NoError(t, err)
	assert.InDelta(t, 11.0, result, 0.0001)
}

// ---------------------------------------------------------------------------
// Integration-level tests exercising error paths through evaluateArithmeticExpression
// ---------------------------------------------------------------------------

func TestEvaluateArithmeticExpression_AdditionalErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expression  string
		errContains string
	}{
		{
			name:        "only_operator",
			expression:  "+",
			errContains: "expected number, got operator",
		},
		{
			name:        "only_power_operator",
			expression:  "**",
			errContains: "invalid ** operator position",
		},
		{
			name:        "parentheses_with_error_inside",
			expression:  "(5+)",
			errContains: "missing operand after operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := evaluateArithmeticExpression(tt.expression)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}
