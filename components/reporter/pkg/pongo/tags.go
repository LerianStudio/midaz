// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/reporter/pkg/constant"

	"github.com/flosch/pongo2/v6"
	"github.com/shopspring/decimal"
)

// aggregateTagNode represents a data structure to hold information for custom aggregation operations in templates.
// It includes the operation type, collection expression, field expression, filter expression, and scaling expression.
type aggregateTagNode struct {
	op             string            // Aggregation type ("count", "sum", "avg", "min", "max")
	collectionExpr pongo2.IEvaluator // Expression representing the data collection
	fieldExpr      pongo2.IEvaluator // Field expression for selecting specific fields (e.g., "by field")
	filterExpr     pongo2.IEvaluator // Condition to filter items in the dataset (e.g., "if condition" in the template tag)
}

// dateNowNode represents a data structure to hold information for the dateNow template tag.
// It includes the expression representing the date format (e.g., "YYYY-MM-dd").
type dateNowNode struct {
	formatExpr pongo2.IEvaluator // Expression representing the date format (e.g., "YYYY-MM-dd")
}

// calcTagNode represents a calc tag that evaluates arithmetic expressions
type calcTagNode struct {
	expression string
}

const aggregateOpCount = "count"

// makeAggregateTag returns a pongo2.TagParser for creating custom aggregate template tags based on the specified operation.
func makeAggregateTag(op string) pongo2.TagParser {
	return func(doc *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
		collectionExpr, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}

		var fieldExpr pongo2.IEvaluator

		if op != aggregateOpCount { // "count" operation doesn't need a specific field to operate on (simply counts the number of elements)
			if t := args.Match(pongo2.TokenIdentifier, "by"); t == nil {
				return nil, args.Error("Expected 'by' keyword", nil)
			}

			fieldExpr, err = args.ParseExpression()
			if err != nil {
				return nil, err
			}
		}

		var filterExpr pongo2.IEvaluator
		if t := args.Match(pongo2.TokenIdentifier, "if"); t != nil {
			filterExpr, err = args.ParseExpression()
			if err != nil {
				return nil, err
			}
		}

		return &aggregateTagNode{
			op:             op,
			collectionExpr: collectionExpr,
			fieldExpr:      fieldExpr,
			filterExpr:     filterExpr,
		}, nil
	}
}

// makeDateNowTag creates a custom Pongo2 template tag that outputs the current date formatted according to the provided expression.
func makeDateNowTag() pongo2.TagParser {
	return func(doc *pongo2.Parser, start *pongo2.Token, args *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
		formatExpr, err := args.ParseExpression()
		if err != nil {
			return nil, err
		}

		return &dateNowNode{
			formatExpr: formatExpr,
		}, nil
	}
}

// Execute renders the current date in a specified format and writes it using the provided template writer.
func (node *dateNowNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	formatVal, err := node.formatExpr.Evaluate(ctx)
	if err != nil {
		return err
	}

	format := formatVal.String()
	goLayout := convertToGoDateLayout(format)
	output := time.Now().Format(goLayout)

	_, err2 := writer.WriteString(output)
	if err2 != nil {
		return ctx.Error("Failed to write date", nil)
	}

	return nil
}

// Execute processes a template tag by evaluating a collection and performing aggregation, then writes the result to the output.
func (node *aggregateTagNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	list, err := evaluateCollection(ctx, node.collectionExpr)
	if err != nil {
		return err
	}

	result, err := aggregateResult(ctx, list, node)
	if err != nil {
		return err
	}

	_, err2 := writer.WriteString(result)
	if err2 != nil {
		return ctx.Error("Error writing output", nil)
	}

	return nil
}

// evaluateCollection evaluates the given expression in the provided context and extracts a []map[string]any collection.
// Returns the evaluated collection or an error if the type assertion fails or evaluation encounters an issue.
func evaluateCollection(ctx *pongo2.ExecutionContext, expr pongo2.IEvaluator) ([]map[string]any, *pongo2.Error) {
	val, err := expr.Evaluate(ctx)
	if err != nil {
		return nil, err
	}

	list, ok := val.Interface().([]map[string]any)
	if !ok {
		return nil, ctx.Error("Expected []map[string]any for collection", nil)
	}

	return list, nil
}

// aggregateResult performs aggregation operations (sum, avg, min, max, count) on a filtered list within a template context.
func aggregateResult(ctx *pongo2.ExecutionContext, list []map[string]any, node *aggregateTagNode) (string, *pongo2.Error) {
	accumulator := newAggregator(node.op)

	for _, item := range list {
		if !passesFilter(ctx, item, node.filterExpr) {
			continue
		}

		if err := accumulator.processItem(ctx, item, node); err != nil {
			return "", err
		}
	}

	return accumulator.result(), nil
}

// aggregator holds state for different aggregation operations
type aggregator struct {
	op     string
	total  decimal.Decimal
	count  int
	minVal *decimal.Decimal
	maxVal *decimal.Decimal
}

// newAggregator creates a new aggregator for the specified operation
func newAggregator(op string) *aggregator {
	return &aggregator{
		op:    op,
		total: decimal.Zero,
	}
}

// processItem processes a single item for aggregation
func (a *aggregator) processItem(ctx *pongo2.ExecutionContext, item map[string]any, node *aggregateTagNode) *pongo2.Error {
	if a.op == aggregateOpCount {
		a.count++
		return nil
	}

	vDec, skip, err := extractDecimalValue(ctx, item, node.fieldExpr)
	if err != nil {
		return err
	}

	if skip {
		return nil
	}

	return a.accumulate(vDec)
}

// accumulate adds a value to the appropriate accumulator
func (a *aggregator) accumulate(vDec decimal.Decimal) *pongo2.Error {
	switch a.op {
	case "sum", "avg":
		a.total = a.total.Add(vDec)
		a.count++
	case "min":
		if a.minVal == nil || vDec.LessThan(*a.minVal) {
			tmp := vDec
			a.minVal = &tmp
		}
	case "max":
		if a.maxVal == nil || vDec.GreaterThan(*a.maxVal) {
			tmp := vDec
			a.maxVal = &tmp
		}
	}

	return nil
}

// result returns the final aggregation result
func (a *aggregator) result() string {
	switch a.op {
	case aggregateOpCount:
		return fmt.Sprintf("%d", a.count)
	case "sum":
		return a.total.String()
	case "avg":
		if a.count == 0 {
			return "0"
		}

		avg := a.total.Div(decimal.NewFromInt(int64(a.count)))

		return avg.String()
	case "min":
		if a.minVal == nil {
			return "0"
		}

		return a.minVal.String()
	case "max":
		if a.maxVal == nil {
			return "0"
		}

		return a.maxVal.String()
	default:
		return "NaN"
	}
}

// passesFilter evaluates a filter expression on an item within a given execution context and returns true if the condition is met.
func passesFilter(ctx *pongo2.ExecutionContext, item map[string]any, filterExpr pongo2.IEvaluator) bool {
	if filterExpr == nil {
		return true
	}

	localCtx := pongo2.NewChildExecutionContext(ctx)
	for k, v := range item {
		localCtx.Private[k] = v
	}

	cond, err := filterExpr.Evaluate(localCtx)

	return err == nil && cond.IsTrue()
}

// extractIntValue retrieves an integer value from a nested map field specified by a field expression in the given context.
// It evaluates the field expression, extracts the field value, and converts it into int64 if possible.
// Returns the integer value, a boolean indicating if the field should be skipped, and an optional error.
func extractDecimalValue(ctx *pongo2.ExecutionContext, item map[string]any, fieldExpr pongo2.IEvaluator) (decimal.Decimal, bool, *pongo2.Error) {
	fieldNameVal, err := fieldExpr.Evaluate(ctx)
	if err != nil {
		return decimal.Zero, false, err
	}

	fieldName := fieldNameVal.String()

	value, ok := getNestedField(item, fieldName)
	if !ok {
		return decimal.Zero, true, nil
	}

	switch v := value.(type) {
	case int:
		return decimal.NewFromInt(int64(v)), false, nil
	case int64:
		return decimal.NewFromInt(v), false, nil
	case float64:
		return decimal.NewFromFloat(v), false, nil
	case string:
		if parsed, err := decimal.NewFromString(v); err == nil {
			return parsed, false, nil
		}
	}

	return decimal.Zero, true, nil
}

// convertToGoDateLayout converts a date format string from a custom format (e.g., "YYYY-MM-dd") to Go's date layout format.
func convertToGoDateLayout(layout string) string {
	replacer := strings.NewReplacer(
		"YYYY", "2006",
		"MM", "01",
		"dd", "02",
		"HH", "15",
		"mm", "04",
		"ss", "05",
	)

	return replacer.Replace(layout)
}

func makeCalcTag(_ *pongo2.Parser, _ *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	calcNode := &calcTagNode{}

	// Get the raw expression as string
	// We need to collect all remaining tokens to build the full expression
	var tokens []string

	for arguments.Remaining() > 0 {
		token := arguments.Current()
		tokens = append(tokens, token.Val)

		arguments.Consume()
	}

	expression := strings.Join(tokens, " ")

	// Remove spaces around dots to fix variable paths
	expression = strings.ReplaceAll(expression, " . ", ".")
	calcNode.expression = expression

	return calcNode, nil
}

func (node *calcTagNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	context := ctx.Public

	expression := node.replaceVariables(node.expression, context, ctx.Private)

	result, err := evaluateArithmeticExpression(expression)
	if err != nil {
		return &pongo2.Error{
			Sender:    "calc",
			OrigError: err,
		}
	}

	// Round to 10 decimal places to avoid artifacts (e.g., ...0000000001)
	rounded := math.Round(result*constant.RoundingPrecision) / constant.RoundingPrecision
	out := formatNumber(rounded)
	out = strings.TrimRight(out, "0")
	out = strings.TrimRight(out, ".")

	if _, err := writer.WriteString(out); err != nil {
		return ctx.Error("Error writing output", nil)
	}

	return nil
}

// shouldSkipMatch determines if a match should be skipped (operators, parentheses, numbers)
func shouldSkipMatch(match string) bool {
	// Skip operators and parentheses
	operators := []string{"+", "-", "*", "/", "**", "(", ")", "[", "]", "{", "}"}
	for _, op := range operators {
		if match == op {
			return true
		}
	}

	// Skip if it's a number (but only if it doesn't contain dots)
	if !strings.Contains(match, ".") {
		if _, err := strconv.ParseFloat(match, 64); err == nil {
			return true
		}
	}

	return false
}

// resolveVariableFromContext attempts to resolve a variable from a given context
func resolveVariableFromContext(match string, context pongo2.Context) (string, bool) {
	templateStr := fmt.Sprintf("{{ %s }}", match)

	// Use a per-call TemplateSet to avoid a race on pongo2's shared DefaultSet.
	// See renderer.go for the detailed explanation.
	ts := newSafeTemplateSet("resolve")

	template, err := ts.FromString(templateStr)
	if err != nil {
		return "", false
	}

	result, err := template.ExecuteBytes(context)
	if err != nil {
		return "", false
	}

	value := strings.TrimSpace(string(result))
	if value == "" {
		return "", false
	}

	// Try to parse as number to validate it's a valid numeric value
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return "", false
	}

	return value, true
}

// replaceVariables replaces variables in the expression with their values
func (node *calcTagNode) replaceVariables(expression string, context pongo2.Context, privateContext pongo2.Context) string {
	// Find all variable patterns in the expression (words with dots)
	// This regex matches patterns like: balance.available, midaz_transaction.balance.0.initial_balance, etc.
	re := regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z0-9_]+)*\b`)

	expression = re.ReplaceAllStringFunc(expression, func(match string) string {
		if shouldSkipMatch(match) {
			return match
		}

		if value, ok := resolveVariableFromContext(match, privateContext); ok {
			return value
		}

		if value, ok := resolveVariableFromContext(match, context); ok {
			return value
		}

		return "0"
	})

	return expression
}
