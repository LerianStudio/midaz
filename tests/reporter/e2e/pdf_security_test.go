// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/tests/reporter/e2e/shared"
)

// ############################################################################
// PDF LFI Security Tests (TC-SEC-PDF-001)
// ############################################################################

// sensitiveFilePatterns contains strings that would appear in the PDF content
// if any of the LFI attack vectors successfully leaked system files.
var sensitiveFilePatterns = []string{
	"root:",         // /etc/passwd first line
	"root:x:0:0:",   // /etc/passwd full root entry
	"/bin/bash",     // common shell in /etc/passwd
	"/bin/sh",       // common shell in /etc/passwd
	"appuser:",      // container user in /etc/passwd
	"nobody:",       // /etc/passwd entry
	"daemon:",       // /etc/passwd entry
	"/etc/shadow",   // shadow file reference
	"/sbin/nologin", // common shell in /etc/passwd
}

// TestSec_PDFLFIAttackVectors generates a PDF report from a template that
// contains multiple LFI attack vectors simultaneously:
//
//   - <meta http-equiv="refresh" content="0;url=file:///etc/passwd"> (top-level nav)
//   - <img src="file:///etc/passwd"> (subresource load)
//   - <link href="file:///etc/shadow"> (stylesheet load)
//   - <style>@import url("file:///etc/hostname")</style> (CSS import)
//
// The test verifies that:
//  1. The template is accepted (these vectors bypass ValidateNoScriptTag by design —
//     the defense-in-depth is at the Chrome rendering layer)
//  2. The PDF is generated successfully (Chrome doesn't crash)
//  3. The PDF does NOT contain any sensitive file contents
//  4. The PDF DOES contain the legitimate report data (proves rendering worked)
func TestSec_PDFLFIAttackVectors(t *testing.T) {
	t.Parallel()

	timeout := shared.DefaultPollTimeout + shared.PDFExtraTimeout

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Step 1: Create template with embedded LFI attack vectors
	tplID := createTemplateForFormat(t, ctx, shared.FormatPDF, shared.FixtureLFIAttackVectors)

	// Step 2: Generate report (triggers PDF rendering in Chrome headless)
	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")

	shared.AssertReportCompleted(t, ctx, apiClient, reportID, timeout)

	// Step 3: Download the generated PDF
	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertContentType(t, headers, "application/pdf")
	shared.AssertPDFContent(t, data)
	shared.SaveReport(t, data, shared.FormatPDF)

	// Step 4: Verify NO sensitive file contents leaked into the PDF.
	// If any LFI vector succeeded, Chrome would navigate to /etc/passwd and render
	// it as the page content. The passwd entries would appear in the PDF metadata
	// or uncompressed text streams.
	pdfContent := strings.ToLower(string(data))

	for _, pattern := range sensitiveFilePatterns {
		assert.NotContains(t, pdfContent, strings.ToLower(pattern),
			"PDF must not contain sensitive file content %q — LFI vector may have succeeded", pattern)
	}

	// Step 5: Verify PDF title proves the original template rendered (not /etc/passwd).
	// PDF metadata (Title field) is stored uncompressed and readable in raw bytes.
	// If Chrome had navigated to file:///etc/passwd, the title would be the file path,
	// not our template title.
	assert.Contains(t, pdfContent, "lfi attack vectors", "PDF metadata should contain the template title — proves Chrome rendered our template, not /etc/passwd")
}
