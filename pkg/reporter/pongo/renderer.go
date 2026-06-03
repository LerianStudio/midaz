// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/LerianStudio/lib-observability/log"

	"github.com/flosch/pongo2/v6"
	"github.com/shopspring/decimal"
)

// Pre-compiled regexes for hot-path functions (avoid re-compiling on every render).
var (
	schemaRefPattern     = regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*):([a-zA-Z_][a-zA-Z0-9_]*)\.([a-zA-Z_][a-zA-Z0-9_]*)`)
	xmlDeclarationRegex  = regexp.MustCompile(`<\?xml[^>]*version="[^"]*"[^>]*\?>`)
	trailingZerosPattern = regexp.MustCompile(`(^|[^.\d])(\d+\.\d*0+)`)
)

// TemplateRenderer handles rendering templates using pongo2
type TemplateRenderer struct{}

// NewTemplateRenderer creates a new TemplateRenderer
func NewTemplateRenderer() *TemplateRenderer {
	return &TemplateRenderer{}
}

// RenderFromBytes renders a template from bytes using the provided data context
func (r *TemplateRenderer) RenderFromBytes(ctx context.Context, templateBytes []byte, data map[string]map[string][]map[string]any, logger log.Logger) (string, error) {
	// Pre-process template to convert schema syntax (database:schema.table) to Pongo2 compatible syntax
	processedTemplate := preprocessSchemaReferences(string(templateBytes))

	// Create a per-call TemplateSet to avoid a race condition on pongo2's
	// shared DefaultSet.  TemplateSet.FromString() writes to the unsynchronized
	// field firstTemplateCreated, so concurrent renders through the global
	// DefaultSet trigger the Go race detector.  A fresh set per render is cheap
	// (no caching benefit is lost because each template string is unique) and
	// eliminates the shared mutable state entirely.
	ts := newSafeTemplateSet("render")

	tpl, err := ts.FromString(processedTemplate)
	if err != nil {
		logger.Log(ctx, log.LevelError, "Error parsing template", log.Err(err))
		return "", err
	}

	pongoCtx := pongo2.Context{
		// Counter storage scoped to this render (prevents race conditions between concurrent renders)
		CounterContextKey: NewCounterStorage(),
		"filter": func(collection any, field string, value any) []map[string]any {
			var result []map[string]any

			items, ok := collection.([]map[string]any)
			if !ok {
				return result
			}

			for _, item := range items {
				if v, ok := item[field]; ok && fmt.Sprintf("%v", v) == fmt.Sprintf("%v", value) {
					result = append(result, item)
				}
			}

			return result
		},
		"contains": func(str1 any, str2 any) bool {
			s1 := strings.ToUpper(fmt.Sprintf("%v", str1))
			s2 := strings.ToUpper(fmt.Sprintf("%v", str2))

			return strings.Contains(s1, s2)
		},
	}
	for k, v := range data {
		pongoCtx[k] = v
	}

	out, err := tpl.Execute(pongoCtx)
	if err != nil {
		logger.Log(ctx, log.LevelError, "Error executing template", log.Err(err))
		return "", err
	}

	cleaned := cleanNumericOutput(out)

	return cleaned, nil
}

// preprocessSchemaReferences converts explicit schema syntax (database:schema.table) to Pongo2 dot notation.
// Example: "mydb:sales.orders" becomes "mydb.sales__orders"
// The schema.table is converted to schema__table (double underscore) to create a valid Pongo2 identifier.
// This requires the data storage to use the same key format (schema__table).
func preprocessSchemaReferences(template string) string {
	// Replace database:schema.table with database.schema__table
	// Note: Use ${n} syntax to avoid Go interpreting $2__ as a named group
	return schemaRefPattern.ReplaceAllString(template, `${1}.${2}__${3}`)
}

// cleanNumericOutput removes trailing zeros from numeric values in the output.
// Multi-dot strings (e.g., COSIF accounting codes like "1.1.2.00.000") are skipped
// by checking the trailing context manually: if the character right after a candidate
// number is a dot or digit, the candidate is part of a larger multi-dot token and is
// left untouched.
func cleanNumericOutput(output string) string {
	// First, protect XML declarations from being modified
	xmlDeclarations := xmlDeclarationRegex.FindAllString(output, -1)

	protectedOutput := output

	for i, declaration := range xmlDeclarations {
		placeholder := fmt.Sprintf("__XML_DECLARATION_%d__", i)
		protectedOutput = strings.Replace(protectedOutput, declaration, placeholder, 1)
	}

	matches := trailingZerosPattern.FindAllStringSubmatchIndex(protectedOutput, -1)

	var b strings.Builder

	b.Grow(len(protectedOutput))

	last := 0

	for _, m := range matches {
		// Match indices: m[0]=fullStart, m[1]=fullEnd,
		// m[2]=g1Start, m[3]=g1End, m[4]=g2Start, m[5]=g2End
		numEnd := m[5]

		// Check trailing context manually (RE2 has no lookahead): if the character
		// immediately after the number is a dot or digit, the candidate is part of
		// a multi-dot token (e.g., a COSIF code) and must be preserved verbatim.
		if numEnd < len(protectedOutput) {
			next := protectedOutput[numEnd]
			if next == '.' || (next >= '0' && next <= '9') {
				b.WriteString(protectedOutput[last:m[1]])
				last = m[1]

				continue
			}
		}

		b.WriteString(protectedOutput[last:m[2]])
		b.WriteString(protectedOutput[m[2]:m[3]])
		b.WriteString(cleanNumericString(protectedOutput[m[4]:m[5]]))

		last = m[1]
	}

	b.WriteString(protectedOutput[last:])

	cleaned := b.String()

	for i, declaration := range xmlDeclarations {
		placeholder := fmt.Sprintf("__XML_DECLARATION_%d__", i)
		cleaned = strings.Replace(cleaned, placeholder, declaration, 1)
	}

	return cleaned
}

// cleanNumericString removes trailing zeros from a numeric string
func cleanNumericString(s string) string {
	s = strings.TrimSpace(s)

	if dec, err := decimal.NewFromString(s); err == nil {
		return dec.String()
	}

	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}

	return s
}
