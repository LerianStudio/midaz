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

	"github.com/LerianStudio/midaz/v4/tests/reporter/e2e/shared"
)

// ############################################################################
// Template Report Validation Tests (TC-TPL-VAL-001 to TC-TPL-VAL-004)
//
// These tests validate that specific business templates produce correctly
// structured reports with the expected internal data.
// ############################################################################

// TC-TPL-VAL-001: account_pdf.tpl — Full PDF pipeline with account data validation.
// Creates a PDF report from the account_pdf template, downloads it, and validates
// the PDF structure and that it was generated from account data.
func TestTemplateReport_AccountPDF(t *testing.T) {
	// Not parallel — these tests are resource-intensive (PDF rendering, multi-datasource XML)
	// and cause 500 errors on template creation when competing with other parallel tests.

	timeout := shared.DefaultPollTimeout + shared.PDFExtraTimeout

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Step 1: Create template with PDF output format.
	tplID := createTemplateForFormat(t, ctx, shared.FormatPDF, shared.FixtureAccountPDF)

	// Step 2: Create report (no filters — all accounts).
	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")

	// Step 3: Wait for report to complete.
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, timeout)

	// Step 4: Download and validate PDF.
	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertContentType(t, headers, "application/pdf")
	shared.AssertPDFContent(t, data)
	shared.SaveReport(t, data, shared.FormatPDF)

	// PDF should be non-trivial in size (accounts table with 7 rows + styling).
	assert.Greater(t, len(data), 1000, "PDF should be larger than 1KB (contains styled account table)")
}

// TC-TPL-VAL-002: ACCS005.tpl — Brazilian CCS XML report with holder/account/alias data.
// Creates an XML report using the ACCS005 regulatory template, downloads it,
// and validates the XML structure and business data (holders, accounts, banking details).
func TestTemplateReport_ACCS005(t *testing.T) {
	// Not parallel — these tests are resource-intensive (PDF rendering, multi-datasource XML)
	// and cause 500 errors on template creation when competing with other parallel tests.

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	// ACCS005 requires crm (MongoDB) datasource.
	dsStatus, _, _ := apiClient.GetDataSourceByID(ctx, shared.DSCRM)
	if dsStatus != http.StatusOK {
		t.Skipf("crm datasource unavailable (status %d) — skipping ACCS005 test", dsStatus)
	}

	// Step 1: Create template with XML output format.
	tplID := createTemplateForFormat(t, ctx, shared.FormatXML, shared.FixtureACCS005)

	// Step 2: Create report (no filters — all data).
	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")

	// Step 3: Wait for report to complete.
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	// Step 4: Download and validate XML.
	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertContentType(t, headers, "application/xml")
	shared.AssertXMLContent(t, data)
	shared.SaveReport(t, data, shared.FormatXML)

	content := string(data)

	// ── XML structure ──
	assert.Contains(t, content, `xmlns="http://www.bcb.gov.br/ccs/ACCS005.xsd"`,
		"should contain BCBS CCS namespace")
	assert.Contains(t, content, "<BCARQ>", "should contain BCARQ header section")
	assert.Contains(t, content, "<NomArq>ACCS005</NomArq>", "should contain file name ACCS005")
	assert.Contains(t, content, "<IdentdEmissor>12345678</IdentdEmissor>",
		"should contain issuer identifier from template")
	assert.Contains(t, content, "<IdentdDestinatario>00000001</IdentdDestinatario>",
		"should contain recipient identifier from template")
	assert.Contains(t, content, "<SISARQ>", "should contain SISARQ data section")
	assert.Contains(t, content, "<CCSArqInfDettRelctPessoa>", "should contain person detail section")

	// ── Responsible entity (first holder document prefix) ──
	assert.Contains(t, content, "<CNPJBaseEntRespons>12345678</CNPJBaseEntRespons>",
		"should contain first holder document prefix (first 8 chars of 12345678901)")

	// ── Holder 1: Alice Johnson (holder-001, NATURAL_PERSON, deposit account) ──
	assert.Contains(t, content, "<NomPessoa>Alice Johnson</NomPessoa>",
		"should contain holder-001 name (decrypted)")
	assert.Contains(t, content, "<CNPJBasePart>12345678</CNPJBasePart>",
		"should contain holder-001 document prefix as participant CNPJ")
	assert.Contains(t, content, "<TpBDV>1</TpBDV>",
		"deposit account (holder-001 via alias-001) should map to TpBDV 1")
	assert.Contains(t, content, "<AgIF>0001</AgIF>",
		"should contain branch 0001 from alias-001 banking details")
	assert.Contains(t, content, "<CtCli>123456</CtCli>",
		"should contain account number 123456 from alias-001 (CACC type)")
	assert.Contains(t, content, "<DtIni>2025-01-25</DtIni>",
		"should contain opening date from alias-001")
	assert.Contains(t, content, "<TpVincBDV>1</TpVincBDV>",
		"should contain relationship type 1")

	// ── Holder 1: Natural person linked data (mother) ──
	assert.Contains(t, content, "<TpVinc>3</TpVinc>",
		"should contain relationship type 3 (mother) for natural person")
	assert.Contains(t, content, "<CNPJ_CPFPessoaVincd>12345678901</CNPJ_CPFPessoaVincd>",
		"should contain full document of holder-001 in linked person section")
	assert.Contains(t, content, "<NomPessoaVincd>Maria Johnson</NomPessoaVincd>",
		"should contain mother name for holder-001 (decrypted)")
	assert.Contains(t, content, "<DtIniVinc>2025-01-25</DtIniVinc>",
		"should contain linked person start date matching alias-001 opening date")

	// ── Holder 2: Charlie Brown (holder-003, NATURAL_PERSON, savings account) ──
	assert.Contains(t, content, "<NomPessoa>Charlie Brown</NomPessoa>",
		"should contain holder-003 name (decrypted)")
	assert.Contains(t, content, "<CNPJBasePart>11122233</CNPJBasePart>",
		"should contain holder-003 document prefix (first 8 chars of 11122233344)")
	assert.Contains(t, content, "<TpBDV>2</TpBDV>",
		"savings account (holder-003 via alias-002) should map to TpBDV 2")
	assert.Contains(t, content, "<AgIF>0002</AgIF>",
		"should contain branch 0002 from alias-002 banking details")
	assert.Contains(t, content, "<CtCli>654321</CtCli>",
		"should contain account number 654321 from alias-002 (CACC type)")
	assert.Contains(t, content, "<DtIni>2025-02-05</DtIni>",
		"should contain opening date from alias-002")

	// ── Holder 2: Natural person linked data (mother) ──
	assert.Contains(t, content, "<CNPJ_CPFPessoaVincd>11122233344</CNPJ_CPFPessoaVincd>",
		"should contain full document of holder-003 in linked person section")
	assert.Contains(t, content, "<NomPessoaVincd>Diana Brown</NomPessoaVincd>",
		"should contain mother name for holder-003 (decrypted)")
	assert.Contains(t, content, "<DtIniVinc>2025-02-05</DtIniVinc>",
		"should contain linked person start date matching alias-002 opening date")

	// ── Structural counts ──
	bdvCount := strings.Count(content, "<Grupo_CCS0005_BDV>")
	assert.Equal(t, 2, bdvCount,
		"should contain exactly 2 BDV groups (one per holder-alias-account match)")
	vincdCount := strings.Count(content, "<Repet_CCS0005_Vincd>")
	assert.Equal(t, 2, vincdCount,
		"should contain exactly 2 linked person sections (both holders are NATURAL_PERSON)")

	// ── Movement date (dynamic) ──
	assert.Contains(t, content, "<DtMovto>", "should contain movement date element")
}

// TC-TPL-VAL-003: cadoc-4111.tpl — CADOC 4111 XML report with operation balances.
// Creates an XML report using the CADOC 4111 template, downloads it,
// and validates the XML structure, organization data, and aggregated balances.
// Requires the midaz_transaction datasource to be configured.
func TestTemplateReport_CADOC4111(t *testing.T) {
	// Not parallel — these tests are resource-intensive (PDF rendering, multi-datasource XML)
	// and cause 500 errors on template creation when competing with other parallel tests.

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	// cadoc-4111 requires midaz_transaction datasource. Skip if unavailable.
	dsStatus, _, _ := apiClient.GetDataSourceByID(ctx, shared.DSMidazTransaction)
	if dsStatus != http.StatusOK {
		t.Skipf("midaz_transaction datasource unavailable (status %d) — skipping CADOC 4111 test", dsStatus)
	}

	// Step 1: Create template with XML output format.
	tplID := createTemplateForFormat(t, ctx, shared.FormatXML, shared.FixtureCadoc4111)

	// Step 2: Create report (no filters — all data).
	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")

	// Step 3: Wait for report to complete.
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	// Step 4: Download and validate XML.
	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertContentType(t, headers, "application/xml")
	shared.AssertXMLContent(t, data)
	shared.SaveReport(t, data, shared.FormatXML)

	content := string(data)

	// ── Document root attributes ──
	assert.Contains(t, content, `codigoDocumento="4111"`,
		"should contain CADOC document code 4111")
	assert.Contains(t, content, `cnpj="12345678"`,
		"should contain organization CNPJ prefix (first 8 chars of 12345678000101)")
	assert.Contains(t, content, `tipoRemessa="I"`,
		"should contain remittance type I (initial)")
	assert.Contains(t, content, `dataBase="`,
		"should contain dataBase date attribute (dynamic)")

	// ── Registros section ──
	assert.Contains(t, content, "<registros>",
		"should contain registros section")

	// ── Route 4111001: last_item_by_group keeps latest operation (150000) ──
	// Seed: operation e001 (100000, 2025-06-01) and e002 (150000, 2025-06-02) for route d001.
	// last_item_by_group by "account_id,route" order_by "created_at" keeps e002.
	// sum_by on "available_balance_after" for route d001 = 150000.
	assert.Contains(t, content, "<conta>4111001</conta>",
		"should contain operation route code 4111001")
	assert.Contains(t, content, "<saldoDia>150000</saldoDia>",
		"route 4111001 balance should be 150000 (latest operation after last_item_by_group)")

	// ── Route 4111002: single operation (500000) ──
	// Seed: operation e003 (500000, 2025-06-01) for route d002.
	// sum_by on "available_balance_after" for route d002 = 500000.
	assert.Contains(t, content, "<conta>4111002</conta>",
		"should contain operation route code 4111002")
	assert.Contains(t, content, "<saldoDia>500000</saldoDia>",
		"route 4111002 balance should be 500000 (single operation)")

	// ── Structural counts ──
	registroCount := strings.Count(content, "<registro>")
	assert.Equal(t, 2, registroCount,
		"should contain exactly 2 registro elements (one per operation route with a code)")
	saldoCount := strings.Count(content, "<saldoDia>")
	assert.Equal(t, 2, saldoCount,
		"should contain exactly 2 saldoDia elements (one per operation route)")
}

// TC-TPL-VAL-004: engine-features-showcase_html.tpl — 17 of 19 custom engine features.
// Creates an HTML report that exercises custom tags, filters, and functions
// supported by the template engine. Validates each feature produced correct output.
// Requires both midaz_onboarding and midaz_transaction datasources.
//
// Features exercised:
//
//	Tags    (10): date_time, sum_by, count_by, avg_by, min_by, max_by, calc,
//	              last_item_by_group, counter, counter_show
//	Filters  (5): slice, percent_of, strip_zeros, replace, where
//	Functions(2): filter(), contains()
//
// Not exercised (blocked by TASK-005 — template validator misparses collection-level pipes):
//
//	Filters  (2): sum (pipe), count (pipe)
func TestTemplateReport_EngineFeatureShowcase(t *testing.T) {
	// Not parallel — resource-intensive multi-datasource template that uses all engine features.

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	// Requires midaz_transaction datasource for operation/route data.
	dsStatus, _, _ := apiClient.GetDataSourceByID(ctx, shared.DSMidazTransaction)
	if dsStatus != http.StatusOK {
		t.Skipf("midaz_transaction datasource unavailable (status %d) — skipping feature showcase", dsStatus)
	}

	// Step 1: Create template with HTML output format.
	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureEngineFeatShowcase)

	// Step 2: Create report (no filters — all data).
	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")

	// Step 3: Wait for report to complete.
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	// Step 4: Download and validate HTML.
	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertContentType(t, headers, "text/html")
	shared.SaveReport(t, data, shared.FormatHTML)

	content := string(data)

	// ── HTML structure ──
	assert.Contains(t, content, "<html", "should be valid HTML")
	assert.Contains(t, content, "Feature Showcase", "should contain report title")
	assert.Contains(t, content, "17 of 19", "should note 17 of 19 features exercised")

	// ── Section 1: Source data rendered ──
	assert.Contains(t, content, "Operating Account", "should render account names from seed data")
	assert.Contains(t, content, "Savings Account", "should render savings account")
	assert.Contains(t, content, "Acme Corp", "should render organization names")

	// ── TAG #1: date_time ──
	// Dynamic — just verify the section rendered with a date pattern.
	assert.Contains(t, content, "id=\"feat-date-time\"", "date_time feature section should exist")
	assert.Regexp(t, `\d{2}/\d{2}/\d{4} \d{2}:\d{2}:\d{2}`, content,
		"date_time should render dd/MM/YYYY HH:mm:ss format")
	assert.Regexp(t, `\d{4}-\d{2}-\d{2}`, content,
		"date_time should render YYYY-MM-dd format")
	assert.Regexp(t, `\d{8}`, content,
		"date_time should render YYYYMMdd compact format")

	// ── TAG #2: sum_by ──
	// Operations: 100000 + 150000 + 500000 = 750000
	assert.Contains(t, content, "id=\"feat-sum-by\"", "sum_by feature section should exist")
	assert.Contains(t, content, "750000",
		"sum_by on operation.available_balance_after should be 750000")

	// ── TAG #3: count_by ──
	// Total accounts: 7, Active: 5 (deposit:3 + savings:1 + expense:1 = 5 active of 7)
	assert.Contains(t, content, "id=\"feat-count-by\"", "count_by feature section should exist")
	countBySection := extractSection(content, "feat-count-by", "feat-avg-by")
	assertCounterValue(t, countBySection, "Total accounts:", "7")
	assertCounterValue(t, countBySection, "Active accounts:", "5")
	assertCounterValue(t, countBySection, "Suspended accounts:", "1")

	// ── TAG #4: avg_by ──
	// Average of 7 account balances: 2100000.50 / 7 = 300000.07142857...
	assert.Contains(t, content, "id=\"feat-avg-by\"", "avg_by feature section should exist")
	assert.Contains(t, content, "300000.07",
		"avg_by should compute average starting with 300000.07")

	// ── TAG #5: min_by ──
	// Min balance: 25000 (Reserve Account)
	assert.Contains(t, content, "id=\"feat-min-by\"", "min_by feature section should exist")
	minSection := extractSection(content, "feat-min-by", "feat-max-by")
	assert.Contains(t, minSection, "25000",
		"min_by should find minimum balance 25000")

	// ── TAG #6: max_by ──
	// Max balance: 750000 (Investment Account)
	assert.Contains(t, content, "id=\"feat-max-by\"", "max_by feature section should exist")
	maxSection := extractSection(content, "feat-max-by", "feat-calc")
	assert.Contains(t, maxSection, "750000",
		"max_by should find maximum balance 750000")

	// ── TAG #7: calc ──
	// 250000 * 1.05 = 262500; (250000 + 500000) / 2 = 375000; 100**2 = 10000
	assert.Contains(t, content, "id=\"feat-calc\"", "calc feature section should exist")
	assert.Contains(t, content, "262500",
		"calc should compute 250000 * 1.05 = 262500")
	assert.Contains(t, content, "375000",
		"calc should compute (250000+500000)/2 = 375000")
	assert.Contains(t, content, "10000",
		"calc should compute 100**2 = 10000")

	// ── TAG #8: last_item_by_group ──
	// Groups operations by route, keeps latest by created_at.
	// Route d001: keeps e002 (150000, 2025-06-02). Route d002: keeps e003 (500000).
	assert.Contains(t, content, "id=\"feat-last-item-by-group\"",
		"last_item_by_group feature section should exist")
	assert.Contains(t, content, "150000",
		"last_item_by_group should keep latest operation for route d001 (150000)")
	assert.Contains(t, content, "500000",
		"last_item_by_group should keep single operation for route d002 (500000)")
	// Should have exactly 2 rows in the result table (one per route).
	libgSection := extractSection(content, "feat-last-item-by-group", "feat-counter")
	assert.Equal(t, 2, strings.Count(libgSection, "<tr><td>"),
		"last_item_by_group should produce 2 grouped rows (one per route)")

	// ── TAGS #9-10: counter + counter_show ──
	// Deposit: 3 (c001, c005, c007), Savings: 2 (c002, c006),
	// Expense: 1 (c003), Investment: 1 (c004), All: 7, D+S: 5
	assert.Contains(t, content, "id=\"feat-counter\"", "counter feature section should exist")
	assert.Contains(t, content, "id=\"feat-counter-show\"", "counter_show feature section should exist")
	counterSection := extractSection(content, "feat-counter-show", "feat-slice")
	assertCounterValue(t, counterSection, "Deposit:", "3")
	assertCounterValue(t, counterSection, "Savings:", "2")
	assertCounterValue(t, counterSection, "Expense:", "1")
	assertCounterValue(t, counterSection, "Investment:", "1")
	assertCounterValue(t, counterSection, "All:", "7")
	assertCounterValue(t, counterSection, "Deposit + Savings (combined):", "5")

	// ── FILTER #11: slice ──
	// Organization CNPJ: 12345678000101 → slice :8 = 12345678, slice 4:10 = 567800
	assert.Contains(t, content, "id=\"feat-slice\"", "slice feature section should exist")
	sliceSection := extractSection(content, "feat-slice", "feat-percent-of")
	assert.Contains(t, sliceSection, "12345678000101",
		"slice should show full CNPJ")
	assert.Contains(t, sliceSection, "12345678",
		"slice :8 should extract CNPJ prefix")
	assert.Contains(t, sliceSection, "567800",
		"slice 4:10 should extract middle portion")

	// ── FILTER #12: percent_of ──
	// 250000 / 1000000 * 100 = 25.00%; 500000 / 1000000 * 100 = 50.00%
	assert.Contains(t, content, "id=\"feat-percent-of\"", "percent_of feature section should exist")
	assert.Contains(t, content, "25%",
		"percent_of should compute 250000/1M = 25%%")
	assert.Contains(t, content, "50%",
		"percent_of should compute 500000/1M = 50%%")

	// ── FILTER #13: strip_zeros ──
	// Account c003 balance is 75000.50 → strip_zeros → 75000.5
	assert.Contains(t, content, "id=\"feat-strip-zeros\"", "strip_zeros feature section should exist")
	assert.Contains(t, content, "75000.5",
		"strip_zeros should remove trailing zero from 75000.50")

	// ── FILTER #14: replace ──
	// "Operating Account" → replace "Account:Acct" → "Operating Acct"
	// "Acme Corp" → replace "Corp:Corporation" → "Acme Corporation"
	assert.Contains(t, content, "id=\"feat-replace\"", "replace feature section should exist")
	assert.Contains(t, content, "Operating Acct",
		"replace should transform 'Account' to 'Acct'")
	assert.Contains(t, content, "Acme Corporation",
		"replace should transform 'Corp' to 'Corporation'")

	// ── FILTER #15: where ──
	// Active accounts: 5 rows (deposit×3 + savings×1 + expense×1)
	assert.Contains(t, content, "id=\"feat-where\"", "where feature section should exist")
	whereSection := extractSection(content, "feat-where", "feat-sum-pipe")
	activeRows := strings.Count(whereSection, "<tr><td>")
	assert.Equal(t, 5, activeRows,
		"where status:active should return 5 accounts")
	// Suspended and inactive accounts should NOT appear in the where result.
	assert.NotContains(t, whereSection, "Investment Account",
		"where active should exclude suspended Investment Account")
	assert.NotContains(t, whereSection, "Reserve Account",
		"where active should exclude inactive Reserve Account")

	// ── FILTERS #16-17: sum pipe, count pipe — BLOCKED by TASK-005 ──
	// These features exist as documentation cards but are not exercised.
	// The template validator misparses collection-level pipe filters.
	assert.Contains(t, content, "id=\"feat-sum-pipe\"", "sum pipe section should exist (blocked note)")
	assert.Contains(t, content, "id=\"feat-count-pipe\"", "count pipe section should exist (blocked note)")
	blockedBadges := strings.Count(content, `class="badge b-blocked"`)
	assert.Equal(t, 2, blockedBadges, "should render 2 blocked feature cards")

	// ── FUNCTION #18: filter() ──
	// Operations for first route (d001): e001 (100000) and e002 (150000).
	assert.Contains(t, content, "id=\"feat-filter-func\"", "filter() feature section should exist")
	filterSection := extractSection(content, "feat-filter-func", "feat-contains")
	filterRows := strings.Count(filterSection, "<tr><td>")
	assert.Equal(t, 2, filterRows,
		"filter() for route d001 should return 2 operations")

	// ── FUNCTION #19: contains() ──
	// contains("Acme Corp", "acme") = true (case-insensitive)
	// contains("Acme Corp", "xyz") = false
	assert.Contains(t, content, "id=\"feat-contains\"", "contains() feature section should exist")
	assert.Contains(t, content, "YES",
		"contains() should find 'acme' in 'Acme Corp' (case-insensitive)")
	containsSection := extractSection(content, "feat-contains", "Section 5")
	assert.Contains(t, containsSection, "NO",
		"contains() should not find 'xyz' in org name")

	// ── Section 5: Advanced combinations ──
	// 5.1: contains() in iteration
	assert.Contains(t, content, "Conditional rendering with contains()",
		"should have contains combination section")

	// 5.2: Summary dashboard using tag features (count_by, sum_by, min_by, max_by)
	assert.Contains(t, content, "Summary Dashboard",
		"should have summary dashboard section")

	// ── Feature count: verify feature cards rendered ──
	tagBadges := strings.Count(content, `class="badge b-tag"`)
	filterBadges := strings.Count(content, `class="badge b-filter"`)
	funcBadges := strings.Count(content, `class="badge b-func"`)
	assert.Equal(t, 10, tagBadges, "should render 10 tag feature cards")
	assert.Equal(t, 5, filterBadges, "should render 5 exercised filter feature cards")
	assert.Equal(t, 2, funcBadges, "should render 2 function feature cards")
}

// extractSection returns the substring of content between the start and end anchor IDs.
// Used to scope assertions to a specific feature card without false positives.
func extractSection(content, startID, endID string) string {
	startIdx := strings.Index(content, `id="`+startID+`"`)
	if startIdx == -1 {
		return ""
	}

	endIdx := strings.Index(content[startIdx:], `id="`+endID+`"`)
	if endIdx == -1 {
		return content[startIdx:]
	}

	return content[startIdx : startIdx+endIdx]
}

// assertCounterValue verifies that a counter label is followed by the expected value
// within the given section of HTML content.
func assertCounterValue(t *testing.T, section, label, expectedValue string) {
	t.Helper()

	idx := strings.Index(section, label)
	if idx == -1 {
		t.Errorf("counter label %q not found in section", label)
		return
	}

	// Look at the next ~50 chars after the label for the value.
	end := idx + len(label) + 50
	if end > len(section) {
		end = len(section)
	}

	vicinity := section[idx:end]
	assert.Contains(t, vicinity, expectedValue,
		"counter %q should show value %s", label, expectedValue)
}
