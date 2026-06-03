//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fuzzy

import (
	"context"
	"strings"
	"testing"
	"time"

	h "github.com/LerianStudio/reporter/tests/utils"
)

func fuzzAssertNoServerError(t *testing.T, label string, code int, body []byte, templateContent string) {
	t.Helper()

	if code >= 500 {
		t.Fatalf("SERVER ERROR on %s: code=%d body=%s template=%q", label, code, string(body), templateContent)
	}
}

func fuzzTryReportGeneration(t *testing.T, ctx context.Context, cli *h.HTTPClient, headers map[string]string, templateID, testOrgID, templateContent string) {
	t.Helper()

	payload := map[string]any{
		"templateId": templateID,
		"filters": map[string]any{
			"midaz_onboarding": map[string]any{
				"organization": map[string]any{
					"id": map[string]any{
						"eq": []string{testOrgID},
					},
				},
			},
		},
	}

	reportCode, reportBody, reportErr := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
	if reportErr != nil {
		t.Logf("Report generation failed (expected): %v", reportErr)
		return
	}

	fuzzAssertNoServerError(t, "report generation", reportCode, reportBody, templateContent)

	if reportCode != 200 && reportCode != 201 {
		return
	}

	reportID := unmarshalID(reportBody)
	if reportID == "" {
		return
	}

	time.Sleep(2 * time.Second)

	statusCode, statusBody, _ := cli.Request(ctx, "GET", "/v1/reports/"+reportID, headers, nil)
	fuzzAssertNoServerError(t, "status check", statusCode, statusBody, templateContent)

	t.Logf("Report generated or failed gracefully: %s", reportID)
}

// FuzzTemplate_InvalidTags tests templates with non-existent or malformed tags
// Expected: Should return 4xx errors, never 5xx (server errors)
func FuzzTemplate_InvalidTags(f *testing.F) {
	// Seed corpus with various malformed template patterns
	f.Add("{{ nonexistent.field }}")
	f.Add("{% for x in fake.table %}{{ x.id }}{% endfor %}")
	f.Add("{{ database.table.nonexistent_column }}")
	f.Add("{% if missing.field > 10 %}error{% endif %}")
	f.Add("{{ midaz_onboarding.account.999999999 }}")
	f.Add("{% with x = filter(fake.data, 'id', 'value') %}{{ x.field }}{% endwith %}")
	f.Add("{{ ...invalid... }}")
	f.Add("{% calc invalid.field + 10 %}")
	f.Add("{{ %illegal syntax% }}")
	f.Add("{% for %}{% endfor %}")
	f.Add("{{ }}")
	f.Add("{% %}")

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	testOrgID := "00000000-0000-0000-0000-000000000001"

	f.Fuzz(func(t *testing.T, templateContent string) {
		if len(templateContent) > 10000 {
			templateContent = templateContent[:10000]
		}

		if strings.TrimSpace(templateContent) == "" {
			return
		}

		files := map[string][]byte{
			"template": []byte(templateContent),
		}

		formData := map[string]string{
			"outputFormat": "TXT",
			"description":  "Fuzz test invalid tags",
		}

		code, body, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", headers, formData, files)
		if err != nil {
			t.Logf("Request error (acceptable): %v", err)
			return
		}

		fuzzAssertNoServerError(t, "invalid template", code, body, templateContent)

		if code != 200 && code != 201 {
			return
		}

		templateID := unmarshalID(body)
		if templateID == "" {
			return
		}

		t.Logf("Template accepted: %s", templateID)
		fuzzTryReportGeneration(t, ctx, cli, headers, templateID, testOrgID, templateContent)
	})
}
