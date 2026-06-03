// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/LerianStudio/reporter/pkg/constant"

	"github.com/flosch/pongo2/v6"
	"github.com/shopspring/decimal"
)

// formatNumber formats a float64 as a string without scientific notation
func formatNumber(num float64) string {
	// Always use %f to avoid scientific notation completely
	return fmt.Sprintf("%.10f", num)
}

// stripZerosFilter formats a numeric value without trailing zeros and without rounding.
// Accepts int, int64, float64 or numeric strings.
func stripZerosFilter(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	v := in.Interface()

	var dec decimal.Decimal

	switch t := v.(type) {
	case int:
		dec = decimal.NewFromInt(int64(t))
	case int64:
		dec = decimal.NewFromInt(t)
	case float64:
		dec = decimal.NewFromFloat(t)
	case string:
		d, err := decimal.NewFromString(t)
		if err != nil {
			return pongo2.AsSafeValue("NaN"), &pongo2.Error{Sender: "strip_zeros", OrigError: err}
		}

		dec = d
	default:
		// Fallback to string formatting
		s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%v", v), "0"), ".")
		return pongo2.AsValue(s), nil
	}

	// decimal.String() already removes trailing zeros when possible
	out := dec.String()

	return pongo2.AsValue(out), nil
}

// percentOfFilter calculates the percentage of `in` relative to `param` and returns it as a formatted string.
// Returns "NaN" with an error if inputs are invalid or the denominator is zero.
func percentOfFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	toDec := func(v any) (decimal.Decimal, error) {
		switch t := v.(type) {
		case int:
			return decimal.NewFromInt(int64(t)), nil
		case int64:
			return decimal.NewFromInt(t), nil
		case float64:
			return decimal.NewFromFloat(t), nil
		case string:
			return decimal.NewFromString(t)
		default:
			return decimal.Zero, fmt.Errorf("unsupported type %T", v)
		}
	}

	num, err1 := toDec(in.Interface())
	den, err2 := toDec(param.Interface())

	if err1 != nil || err2 != nil || den.IsZero() {
		return pongo2.AsSafeValue("NaN"), &pongo2.Error{
			Sender:    "percentOfFilter",
			OrigError: errors.New("invalid input or denominator is zero"),
		}
	}

	hundred := decimal.NewFromInt(constant.PercentBase)
	pct := num.Mul(hundred).Div(den)

	return pongo2.AsValue(pct.StringFixed(constant.DecimalPrecisionPercent) + "%"), nil
}

// sliceFilter extracts a substring from the input string based on the specified "start:end" slice format in the parameter.
// Returns the sliced string or an error if the format is invalid or indices are out of bounds.
func sliceFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	s := in.String()

	parts := strings.Split(param.String(), ":")
	if len(parts) != constant.SliceFormatParts {
		return nil, &pongo2.Error{
			Sender:    "slice",
			OrigError: fmt.Errorf("invalid slice format, expected 'start:end'"),
		}
	}

	start, err1 := strconv.Atoi(parts[0])
	end, err2 := strconv.Atoi(parts[1])

	if err1 != nil || err2 != nil {
		return nil, &pongo2.Error{
			Sender:    "slice",
			OrigError: fmt.Errorf("invalid start or end in slice"),
		}
	}

	if start < 0 {
		start = 0
	}

	if end > len(s) {
		end = len(s)
	}

	if start > end {
		start = end
	}

	return pongo2.AsValue(s[start:end]), nil
}

// evaluateArithmeticExpression evaluates a mathematical expression string.
// Supports +, -, *, /, ** operators and parentheses, including negative numbers.
func evaluateArithmeticExpression(expression string) (float64, error) {
	expression = strings.ReplaceAll(expression, " ", "")

	if expression == "" {
		return 0, fmt.Errorf("empty expression")
	}

	if strings.Contains(expression, "(") {
		return evaluateWithParentheses(expression)
	}

	// Tokenize the expression to properly handle negative numbers
	tokens, err := tokenizeExpression(expression)
	if err != nil {
		return 0, err
	}

	return evaluateTokens(tokens)
}

// token represents a number or operator in the expression
type token struct {
	isOperator bool
	operator   string
	value      float64
}

// tokenizeExpression splits an expression into tokens, properly handling negative numbers
func tokenizeExpression(expression string) ([]token, error) {
	var tokens []token

	i := 0

	for i < len(expression) {
		tok, consumed, err := parseNextToken(expression, i, tokens)
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, tok)
		i += consumed
	}

	return tokens, nil
}

// parseNextToken parses the next token from expression starting at position i
func parseNextToken(expression string, i int, prevTokens []token) (token, int, error) {
	ch := expression[i]

	// Check for ** operator (must check before single *)
	if ch == '*' && i+1 < len(expression) && expression[i+1] == '*' {
		return token{isOperator: true, operator: "**"}, constant.PowerOperatorTokenLength, nil
	}

	// Check for single character operators (+, *, /)
	if ch == '+' || ch == '*' || ch == '/' {
		return token{isOperator: true, operator: string(ch)}, 1, nil
	}

	// Handle minus sign - could be operator or negative sign
	if ch == '-' {
		return parseMinusToken(expression, i, prevTokens)
	}

	// Parse a number (digits and decimal point)
	if isDigit(ch) || ch == '.' {
		return parseNumberToken(expression, i)
	}

	return token{}, 0, fmt.Errorf("unexpected character '%c' at position %d", ch, i)
}

// parseMinusToken handles minus sign which could be operator or negative sign
func parseMinusToken(expression string, i int, prevTokens []token) (token, int, error) {
	// It's a negative sign if at start or after an operator
	isNegativeSign := len(prevTokens) == 0 || prevTokens[len(prevTokens)-1].isOperator

	if !isNegativeSign {
		return token{isOperator: true, operator: "-"}, 1, nil
	}

	// Parse the negative number
	numStr, length := parseNumber(expression[i:])
	if length == 0 {
		return token{}, 0, fmt.Errorf("invalid number at position %d", i)
	}

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return token{}, 0, err
	}

	return token{isOperator: false, value: val}, length, nil
}

// parseNumberToken parses a number token from expression
func parseNumberToken(expression string, i int) (token, int, error) {
	numStr, length := parseNumber(expression[i:])
	if length == 0 {
		return token{}, 0, fmt.Errorf("invalid number at position %d", i)
	}

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return token{}, 0, err
	}

	return token{isOperator: false, value: val}, length, nil
}

// parseNumber extracts a number (including negative) from the start of the string
func parseNumber(s string) (string, int) {
	if len(s) == 0 {
		return "", 0
	}

	i := 0
	// Handle negative sign
	if s[i] == '-' {
		i++
	}

	// Must have at least one digit
	if i >= len(s) || (!isDigit(s[i]) && s[i] != '.') {
		return "", 0
	}

	hasDigit := false
	hasDecimal := false

	for i < len(s) {
		ch := s[i]
		if isDigit(ch) {
			hasDigit = true
			i++
		} else if ch == '.' && !hasDecimal {
			hasDecimal = true
			i++
		} else {
			break
		}
	}

	if !hasDigit {
		return "", 0
	}

	return s[:i], i
}

// isDigit checks if a character is a digit
func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// evaluateTokens evaluates a list of tokens respecting operator precedence
func evaluateTokens(tokens []token) (float64, error) {
	if len(tokens) == 0 {
		return 0, fmt.Errorf("empty expression")
	}

	// First pass: handle ** (highest precedence, right-to-left)
	tokens, err := evaluatePowerTokens(tokens)
	if err != nil {
		return 0, err
	}

	// Second pass: handle * and / (left-to-right)
	tokens, err = evaluateMulDivTokens(tokens)
	if err != nil {
		return 0, err
	}

	// Third pass: handle + and - (left-to-right)
	return evaluateAddSubTokens(tokens)
}

// evaluatePowerTokens handles ** operations (right-to-left associativity)
func evaluatePowerTokens(tokens []token) ([]token, error) {
	// Process from right to left for right-to-left associativity
	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i].isOperator && tokens[i].operator == "**" {
			if i == 0 || i == len(tokens)-1 {
				return nil, fmt.Errorf("invalid ** operator position")
			}

			if tokens[i-1].isOperator || tokens[i+1].isOperator {
				return nil, fmt.Errorf("missing operand for ** operator")
			}

			result := math.Pow(tokens[i-1].value, tokens[i+1].value)
			// Replace the three tokens with the result
			newTokens := make([]token, 0, len(tokens)-2)
			newTokens = append(newTokens, tokens[:i-1]...)
			newTokens = append(newTokens, token{isOperator: false, value: result})
			newTokens = append(newTokens, tokens[i+2:]...)
			tokens = newTokens
			// Don't decrement i, as we need to check the same position again
		}
	}

	return tokens, nil
}

// evaluateMulDivTokens handles * and / operations (left-to-right)
func evaluateMulDivTokens(tokens []token) ([]token, error) {
	for {
		found := false

		for i := 0; i < len(tokens); i++ {
			if tokens[i].isOperator && (tokens[i].operator == "*" || tokens[i].operator == "/") {
				if i == 0 || i == len(tokens)-1 {
					return nil, fmt.Errorf("invalid %s operator position", tokens[i].operator)
				}

				if tokens[i-1].isOperator || tokens[i+1].isOperator {
					return nil, fmt.Errorf("missing operand for %s operator", tokens[i].operator)
				}

				var result float64
				if tokens[i].operator == "*" {
					result = tokens[i-1].value * tokens[i+1].value
				} else {
					if tokens[i+1].value == 0 {
						return nil, fmt.Errorf("division by zero")
					}

					result = tokens[i-1].value / tokens[i+1].value
				}

				// Replace the three tokens with the result
				newTokens := make([]token, 0, len(tokens)-2)
				newTokens = append(newTokens, tokens[:i-1]...)
				newTokens = append(newTokens, token{isOperator: false, value: result})
				newTokens = append(newTokens, tokens[i+2:]...)
				tokens = newTokens
				found = true

				break
			}
		}

		if !found {
			break
		}
	}

	return tokens, nil
}

// evaluateAddSubTokens handles + and - operations (left-to-right)
func evaluateAddSubTokens(tokens []token) (float64, error) {
	if len(tokens) == 0 {
		return 0, fmt.Errorf("empty expression")
	}

	if len(tokens) == 1 {
		if tokens[0].isOperator {
			return 0, fmt.Errorf("expected number, got operator")
		}

		return tokens[0].value, nil
	}

	// Process left to right
	result := tokens[0].value
	if tokens[0].isOperator {
		return 0, fmt.Errorf("expression cannot start with operator")
	}

	i := 1
	for i < len(tokens) {
		if !tokens[i].isOperator {
			return 0, fmt.Errorf("expected operator at position %d", i)
		}

		if i+1 >= len(tokens) {
			return 0, fmt.Errorf("missing operand after operator")
		}

		if tokens[i+1].isOperator {
			return 0, fmt.Errorf("expected number, got operator")
		}

		switch tokens[i].operator {
		case "+":
			result += tokens[i+1].value
		case "-":
			result -= tokens[i+1].value
		default:
			return 0, fmt.Errorf("unexpected operator %s", tokens[i].operator)
		}

		i += 2
	}

	return result, nil
}

// evaluateWithParentheses handles expressions with parentheses
func evaluateWithParentheses(expression string) (float64, error) {
	// Find the innermost parentheses
	start := strings.LastIndex(expression, "(")
	if start == -1 {
		// No more parentheses, tokenize and evaluate
		tokens, err := tokenizeExpression(expression)
		if err != nil {
			return 0, err
		}

		return evaluateTokens(tokens)
	}

	end := strings.Index(expression[start:], ")")
	if end == -1 {
		return 0, fmt.Errorf("unmatched parentheses in expression: %s", expression)
	}

	end += start

	innerExpr := expression[start+1 : end]

	innerResult, err := evaluateArithmeticExpression(innerExpr)
	if err != nil {
		return 0, err
	}

	// Build the new expression with the result
	// Handle the case where the result is negative and follows an operator
	resultStr := formatNumber(innerResult)

	// Check if we need to handle negative result after an operator
	// e.g., "5*(-3)" becomes "5*-3.0000000000" which the tokenizer handles correctly
	newExpr := expression[:start] + resultStr + expression[end+1:]

	return evaluateArithmeticExpression(newExpr)
}
