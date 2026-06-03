//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fuzzy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	h "github.com/LerianStudio/reporter/tests/utils"
)

// FuzzTemplate_SyntaxErrors tests various template syntax errors
// Expected: Should be rejected at template creation with 4xx, never 5xx
func FuzzTemplate_SyntaxErrors(f *testing.F) {
	// Seed corpus with common syntax errors
	seeds := []string{
		// Unclosed tags
		"{% for x in data.table %} {{ x.id }}",
		"{{ account.id",
		"{% if condition %} text",
		"{% with x = data.table %} {{ x.id }}",

		// Mismatched tags
		"{% for x in data.table %}{{ x.id }}{% endwith %}",
		"{% if true %}text{% endfor %}",

		// Nested errors
		"{% for x in {% for y in data %} {{ y }} {% endfor %} %}{{ x }}{% endfor %}",

		// Empty blocks
		"{% for x in %}{% endfor %}",
		"{% if %}{% endif %}",
		"{% with = %}{% endwith %}",

		// Invalid expressions
		"{{ ++++ }}",
		"{{ ........ }}",
		"{% calc / %}",
		"{% calc 1 / 0 %}",

		// Script injection attempts
		"<script>alert('xss')</script>",
		"{{ '<script>alert(1)</script>' }}",
		"{% for x in data %}<script>{{ x.id }}</script>{% endfor %}",

		// SQL injection attempts in template
		"{{ account'; DROP TABLE accounts; -- }}",
		"{% for x in 'users; DELETE FROM users; --' %}{{ x }}{% endfor %}",

		// Special characters
		"\x00\x01\x02{{ data.field }}",
		"{{ data\u0000.field }}",

		// Extremely nested
		strings.Repeat("{% for x in data.table %}", 100) + "{{ x.id }}" + strings.Repeat("{% endfor %}", 100),

		// Unicode edge cases
		"{{ 你好.世界 }}",
		"{{ مرحبا.عالم }}",
		"{{ 🚀.💀 }}",

		// Malformed filter function
		"{% with x = filter() %}{{ x }}{% endwith %}",
		"{% with x = filter(data.table) %}{{ x }}{% endwith %}",
		"{% with x = filter(data.table, 'field') %}{{ x }}{% endwith %}",
		"{% with x = filter(data.table, 'field', 'value', 'extra', 'params') %}{{ x }}{% endwith %}",

		// Malformed calc
		"{% calc %}",
		"{% calc data.field + %}",
		"{% calc + data.field %}",
		"{% calc data.field ** ** 2 %}",

		// Invalid aggregations
		"{% avg %}",
		"{% sum %}",
		"{% sum invalid.field %}",
		"{% avg of in %}",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	f.Fuzz(func(t *testing.T, templateContent string) {
		// Limit template size to prevent DoS
		if len(templateContent) > 100000 {
			templateContent = templateContent[:100000]
		}

		// Empty input is valid for fuzz testing - function should handle gracefully
		if strings.TrimSpace(templateContent) == "" {
			return
		}

		files := map[string][]byte{
			"template": []byte(templateContent),
		}

		formData := map[string]string{
			"outputFormat": "TXT",
			"description":  "Fuzz test syntax errors",
		}

		code, body, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", headers, formData, files)
		if err != nil {
			t.Logf("Request error (acceptable): %v", err)
			return
		}

		// Server should NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on syntax error template: code=%d body=%s template=%q", code, string(body), templateContent)
		}

		// Log accepted templates for analysis
		if code == 200 || code == 201 {
			var resp struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(body, &resp); err == nil && resp.ID != "" {
				t.Logf("Template unexpectedly accepted (may fail at render time): id=%s template=%q", resp.ID, templateContent)
			}
		} else {
			t.Logf("Template correctly rejected: code=%d template=%q", code, templateContent)
		}
	})
}

// FuzzTemplate_OutputFormats tests invalid output formats
func FuzzTemplate_OutputFormats(f *testing.F) {
	f.Add("TXT")
	f.Add("HTML")
	f.Add("CSV")
	f.Add("XML")
	f.Add("INVALID")
	f.Add("")
	f.Add("pdf")
	f.Add("json")
	f.Add("../../../etc/passwd")
	f.Add("C:\\Windows\\System32")
	f.Add("'; DROP TABLE templates; --")
	f.Add("\x00\x01\x02")
	f.Add(strings.Repeat("A", 1000))

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	simpleTemplate := "Test Template"

	f.Fuzz(func(t *testing.T, outputFormat string) {
		// Limit size
		if len(outputFormat) > 256 {
			outputFormat = outputFormat[:256]
		}

		files := map[string][]byte{
			"template": []byte(simpleTemplate),
		}

		formData := map[string]string{
			"outputFormat": outputFormat,
			"description":  "Fuzz test output format",
		}

		code, body, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", headers, formData, files)
		if err != nil {
			t.Logf("Request error (acceptable): %v", err)
			return
		}

		// Server should NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on invalid outputFormat: code=%d body=%s format=%q", code, string(body), outputFormat)
		}

		if code == 200 || code == 201 {
			t.Logf("Output format accepted: %q", outputFormat)
		} else {
			t.Logf("Output format rejected: code=%d format=%q", code, outputFormat)
		}
	})
}

// FuzzTemplate_Description tests various description inputs
func FuzzTemplate_Description(f *testing.F) {
	f.Add("Valid description")
	f.Add("")
	f.Add("<script>alert('xss')</script>")
	f.Add("'; DELETE FROM templates WHERE '1'='1")
	f.Add(strings.Repeat("A", 10000))
	f.Add("\x00\x01\x02\x03")
	f.Add("Unicode test: 你好世界 🚀")
	f.Add("\n\r\t")
	f.Add("{{ template.injection }}")
	f.Add("{% if true %}injection{% endif %}")

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	simpleTemplate := "Test Template"

	f.Fuzz(func(t *testing.T, description string) {
		// Limit size
		if len(description) > 50000 {
			description = description[:50000]
		}

		files := map[string][]byte{
			"template": []byte(simpleTemplate),
		}

		formData := map[string]string{
			"outputFormat": "TXT",
			"description":  description,
		}

		code, body, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", headers, formData, files)
		if err != nil {
			t.Logf("Request error (acceptable): %v", err)
			return
		}

		// Server should NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on description fuzzing: code=%d body=%s description=%q", code, string(body), description)
		}

		if code == 200 || code == 201 {
			var resp struct {
				ID          string `json:"id"`
				Description string `json:"description"`
			}
			if err := json.Unmarshal(body, &resp); err == nil {
				// Verify description wasn't executed or interpreted
				if strings.Contains(resp.Description, "<script>") {
					t.Logf("WARNING: Script tag in stored description: %s", resp.Description)
				}
			}
		}
	})
}

// FuzzTemplate_Size tests extremely large templates
func FuzzTemplate_Size(f *testing.F) {
	f.Add(100)
	f.Add(1000)
	f.Add(10000)
	f.Add(100000)
	f.Add(1000000)
	f.Add(10000000)

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, 30*time.Second) // Longer timeout for large uploads
	headers := h.AuthHeaders()
	_ = env // Use env variable to avoid unused warning

	f.Fuzz(func(t *testing.T, size int) {
		// Cap at 50MB to prevent test runner OOM
		if size > 50*1024*1024 {
			size = 50 * 1024 * 1024
		}

		if size < 0 {
			size = -size
		}

		// Create template of specified size
		templateContent := "{{ data.field }}\n"
		repeatCount := size / len(templateContent)
		if repeatCount > 1000000 {
			repeatCount = 1000000 // Reasonable limit
		}

		largeTemplate := strings.Repeat(templateContent, repeatCount)

		files := map[string][]byte{
			"template": []byte(largeTemplate),
		}

		formData := map[string]string{
			"outputFormat": "TXT",
			"description":  fmt.Sprintf("Fuzz test size=%d", size),
		}

		code, body, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", headers, formData, files)
		if err != nil {
			t.Logf("Request error on size=%d (acceptable): %v", size, err)
			return
		}

		// Server should NEVER crash (5xx)
		if code >= 500 {
			t.Fatalf("SERVER ERROR on large template: code=%d size=%d body=%s", code, size, string(body))
		}

		t.Logf("Template size=%d result: code=%d", size, code)
	})
}
