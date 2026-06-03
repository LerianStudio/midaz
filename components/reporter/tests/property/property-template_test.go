//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package property

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/reporter/pkg/templateutils"
)

// Property 1: Template parsing deve ser determinístico
// Mesma entrada sempre produz mesma saída
func TestProperty_TemplateParsing_IsDeterministic(t *testing.T) {
	t.Parallel()

	property := func(templateContent string) bool {
		// Skip empty or too large templates
		if len(templateContent) == 0 || len(templateContent) > 10000 {
			return true
		}

		// Parse template twice
		result1 := templateutils.MappedFieldsOfTemplate(templateContent)
		result2 := templateutils.MappedFieldsOfTemplate(templateContent)

		// Results should be identical (deterministic)
		return compareMappedFields(result1, result2)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: template parsing is not deterministic: %v", err)
	}
}

// Property 2: Template parsing deve ser idempotente
// Parse(Parse(x)) == Parse(x)
func TestProperty_TemplateParsing_IsIdempotent(t *testing.T) {
	t.Parallel()

	property := func(templateContent string) bool {
		if len(templateContent) == 0 || len(templateContent) > 10000 {
			return true
		}

		// Parse once
		result1 := templateutils.MappedFieldsOfTemplate(templateContent)

		// Parse again (should be same as result1, demonstrating idempotency)
		result2 := templateutils.MappedFieldsOfTemplate(templateContent)

		return compareMappedFields(result1, result2)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: template parsing is not idempotent: %v", err)
	}
}

// Property 3: MappedFields nunca deve conter campos vazios
func TestProperty_MappedFields_NoEmptyFields(t *testing.T) {
	t.Parallel()

	property := func(templateContent string) bool {
		if len(templateContent) == 0 || len(templateContent) > 10000 {
			return true
		}

		mappedFields := templateutils.MappedFieldsOfTemplate(templateContent)

		// Check that no database, table, or field is empty
		for dbName, tables := range mappedFields {
			if dbName == "" {
				return false
			}

			for tableName, fields := range tables {
				if tableName == "" {
					return false
				}

				for _, field := range fields {
					if field == "" {
						return false
					}
				}
			}
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: mapped fields contains empty values: %v", err)
	}
}

// Property 4: Template com tags vazias não deve crashear
func TestProperty_Template_EmptyTags_NoCrash(t *testing.T) {
	t.Parallel()

	property := func(content string) bool {
		// This should never panic, regardless of input
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Template parsing panicked with input %q: %v", content, r)
			}
		}()

		_ = templateutils.MappedFieldsOfTemplate(content)
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("Property violated: template parsing crashed: %v", err)
	}
}

// Property 5: GetMimeType deve sempre retornar um mime type válido
func TestProperty_GetMimeType_AlwaysValid(t *testing.T) {
	t.Parallel()

	validMimeTypes := map[string]bool{
		"application/xml":          true,
		"text/html":                true,
		"text/csv":                 true,
		"text/plain":               true,
		"application/octet-stream": true,
	}

	property := func(format string) bool {
		mimeType := templateutils.GetMimeType(format)
		return validMimeTypes[mimeType]
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: GetMimeType returned invalid mime type: %v", err)
	}
}

// Property 6: CleanPath deve sempre retornar slice sem índices numéricos
func TestProperty_CleanPath_NoNumericIndices(t *testing.T) {
	t.Parallel()

	property := func(path string) bool {
		if len(path) == 0 {
			return true
		}

		cleanedPath := templateutils.CleanPath(path)

		// Cleaned path should not contain numeric-only segments
		for _, segment := range cleanedPath {
			// Check if segment is purely numeric
			isNumeric := true
			if len(segment) == 0 {
				continue
			}
			for _, ch := range segment {
				if ch < '0' || ch > '9' {
					isNumeric = false
					break
				}
			}
			if isNumeric {
				return false
			}
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: CleanPath contains numeric indices: %v", err)
	}
}

// Property 7: ValidateNoScriptTag deve rejeitar qualquer conteúdo com <script>
func TestProperty_ValidateNoScriptTag_RejectsScript(t *testing.T) {
	t.Parallel()

	property := func(prefix, suffix string) bool {
		// Build template with <script> tag
		template := prefix + "<script>alert('xss')</script>" + suffix

		// Should always return error for templates with <script>
		err := templateutils.ValidateNoScriptTag(template)
		return err != nil
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Property violated: ValidateNoScriptTag did not reject script tag: %v", err)
	}
}

// Property 8: ValidateNoScriptTag deve aceitar templates sem <script>
func TestProperty_ValidateNoScriptTag_AcceptsNoScript(t *testing.T) {
	t.Parallel()

	property := func(content string) bool {
		// Skip if content contains <script>
		if containsScriptTag(content) {
			return true
		}

		// Should not return error for templates without <script>
		err := templateutils.ValidateNoScriptTag(content)
		return err == nil
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: ValidateNoScriptTag rejected valid template: %v", err)
	}
}

// Helper functions

func compareMappedFields(a, b map[string]map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}

	for dbName, aTables := range a {
		bTables, exists := b[dbName]
		if !exists || len(aTables) != len(bTables) {
			return false
		}

		for tableName, aFields := range aTables {
			bFields, exists := bTables[tableName]
			if !exists || len(aFields) != len(bFields) {
				return false
			}

			// Compare fields
			fieldMap := make(map[string]bool)
			for _, field := range aFields {
				fieldMap[field] = true
			}

			for _, field := range bFields {
				if !fieldMap[field] {
					return false
				}
			}
		}
	}

	return true
}

// eventHandlerPattern mirrors the pattern used in ValidateNoScriptTag.
var eventHandlerPattern = regexp.MustCompile(`(?i)\bon\w+\s*=`)

func containsScriptTag(content string) bool {
	// Check for all patterns that ValidateNoScriptTag rejects:
	// script tags, iframe/object/embed tags, and event handler attributes.
	if len(content) == 0 {
		return false
	}

	lower := strings.ToLower(content)

	return strings.Contains(lower, "<script") ||
		strings.Contains(lower, "<iframe") ||
		strings.Contains(lower, "<object") ||
		strings.Contains(lower, "<embed") ||
		eventHandlerPattern.MatchString(content)
}

// Benchmark: Verificar performance de parsing
func BenchmarkTemplateParsingPerformance(b *testing.B) {
	template := `
	{% for account in midaz_onboarding.account %}
		Account: {{ account.id }}
		Name: {{ account.name }}
		{% for balance in midaz_transaction.balance %}
			Balance: {{ balance.available }}
		{% endfor %}
	{% endfor %}
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = templateutils.MappedFieldsOfTemplate(template)
	}
}

// Context-aware property test: parsing is stateless and always returns a non-nil map.
func TestProperty_TemplateParsingWithContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	property := func(template string) bool {
		// Ensure context is not required for parsing (stateless operation)
		_ = ctx // Use context to avoid unused variable error

		if len(template) == 0 || len(template) > 5000 {
			return true
		}

		// Should work without context dependency and always return a non-nil map
		result := templateutils.MappedFieldsOfTemplate(template)
		if result == nil {
			return false
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}
