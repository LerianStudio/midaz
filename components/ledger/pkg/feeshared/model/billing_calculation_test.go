// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"reflect"
	"testing"

	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestTransactionPayload builds a minimal transaction.Transaction using Midaz types directly.
func newTestTransactionPayload() transaction.Transaction {
	return transaction.Transaction{
		Description: "billing charge",
		Code:        "BLC-001",
		Metadata: map[string]any{
			"billingPackageId":   "pkg-001",
			"billingPeriod":      "2026-01",
			"totalAccounts":      10,
			"totalCharged":       8,
			"totalSkipped":       2,
			"unitPrice":          "1.50",
			"discountPercentage": "0",
		},
		Send: transaction.Send{
			Asset: "BRL",
			Value: decimal.NewFromFloat(12.00),
			Source: transaction.Source{
				From: []transaction.FromTo{
					{
						AccountAlias:    "@debit-account",
						ChartOfAccounts: "1000",
						Metadata:        map[string]any{},
					},
				},
			},
			Distribute: transaction.Distribute{
				To: []transaction.FromTo{
					{
						AccountAlias:    "@credit-account",
						ChartOfAccounts: "2000",
						Metadata:        map[string]any{},
					},
				},
			},
		},
	}
}

// TestBillingCalculateResponse_JSONRoundtrip verifies that a full response with
// volume and maintenance results can be marshalled to JSON and unmarshalled back
// with all fields preserved.
func TestBillingCalculateResponse_JSONRoundtrip(t *testing.T) {
	t.Parallel()

	volTxBytes, err := json.Marshal(newTestTransactionPayload())
	require.NoError(t, err)

	volumeResult := BillingCalculationResult{
		BillingPackageID:    "pkg-001",
		BillingPackageLabel: "Volume Package",
		BillingType:         "volume",
		Period:              "2026-01",
		TotalAccounts:       10,
		TotalCharged:        8,
		TotalSkipped:        2,
		TotalNetAmount:      decimal.NewFromFloat(12.00),
		TransactionPayload:  json.RawMessage(volTxBytes),
	}

	maintenanceTx := newTestTransactionPayload()
	maintenanceTx.Description = "maintenance charge"
	maintenanceTx.Code = "MNT-001"
	maintenanceTx.Send.Value = decimal.NewFromFloat(50.00)

	mntTxBytes, err := json.Marshal(maintenanceTx)
	require.NoError(t, err)

	maintenanceResult := BillingCalculationResult{
		BillingPackageID:    "pkg-002",
		BillingPackageLabel: "Maintenance Package",
		BillingType:         "maintenance",
		Period:              "2026-01",
		TotalAccounts:       5,
		TotalCharged:        5,
		TotalSkipped:        0,
		TotalNetAmount:      decimal.NewFromFloat(250.00),
		TransactionPayload:  json.RawMessage(mntTxBytes),
	}

	response := BillingCalculateResponse{
		Results: []BillingCalculationResult{volumeResult, maintenanceResult},
		Summary: BillingCalculateSummary{
			TotalResults:     2,
			TotalVolume:      1,
			TotalMaintenance: 1,
			TotalNetAmount:   decimal.NewFromFloat(262.00),
		},
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	var got BillingCalculateResponse

	err = json.Unmarshal(data, &got)
	require.NoError(t, err)

	// Verify Summary fields.
	assert.Equal(t, response.Summary.TotalResults, got.Summary.TotalResults)
	assert.Equal(t, response.Summary.TotalVolume, got.Summary.TotalVolume)
	assert.Equal(t, response.Summary.TotalMaintenance, got.Summary.TotalMaintenance)
	assert.True(t, response.Summary.TotalNetAmount.Equal(got.Summary.TotalNetAmount))

	// Verify Results slice length.
	require.Len(t, got.Results, 2)

	// Verify first result (volume).
	gotVol := got.Results[0]

	assert.Equal(t, "pkg-001", gotVol.BillingPackageID)
	assert.Equal(t, "Volume Package", gotVol.BillingPackageLabel)
	assert.Equal(t, "volume", gotVol.BillingType)
	assert.Equal(t, "2026-01", gotVol.Period)
	assert.Equal(t, 10, gotVol.TotalAccounts)
	assert.Equal(t, 8, gotVol.TotalCharged)
	assert.Equal(t, 2, gotVol.TotalSkipped)
	assert.True(t, decimal.NewFromFloat(12.00).Equal(gotVol.TotalNetAmount))

	// Verify TransactionPayload round-trips correctly via json.RawMessage.
	require.NotEmpty(t, gotVol.TransactionPayload)
	var gotVolTx transaction.Transaction
	require.NoError(t, json.Unmarshal(gotVol.TransactionPayload, &gotVolTx))
	assert.Equal(t, "billing charge", gotVolTx.Description)
	assert.Equal(t, "BLC-001", gotVolTx.Code)
	assert.Equal(t, "BRL", gotVolTx.Send.Asset)
	require.Len(t, gotVolTx.Send.Source.From, 1)
	assert.Equal(t, "@debit-account", gotVolTx.Send.Source.From[0].AccountAlias)
	require.Len(t, gotVolTx.Send.Distribute.To, 1)
	assert.Equal(t, "@credit-account", gotVolTx.Send.Distribute.To[0].AccountAlias)

	// Verify second result (maintenance).
	gotMnt := got.Results[1]

	assert.Equal(t, "pkg-002", gotMnt.BillingPackageID)
	assert.Equal(t, "maintenance", gotMnt.BillingType)
	assert.True(t, decimal.NewFromFloat(250.00).Equal(gotMnt.TotalNetAmount))
}

// TestBillingCalculationResult_TransactionPayloadIsRawMessage verifies that the
// TransactionPayload field is json.RawMessage, allowing {} for zero-amount results
// and full transaction JSON for chargeable results.
func TestBillingCalculationResult_TransactionPayloadIsRawMessage(t *testing.T) {
	t.Parallel()

	result := BillingCalculationResult{}

	fieldType := reflect.TypeOf(result).Field(getFieldIndex(t, reflect.TypeOf(result), "TransactionPayload")).Type

	assert.Equal(t, reflect.TypeOf(json.RawMessage{}), fieldType,
		"TransactionPayload must be json.RawMessage to support {} for zero-amount results")

	// Zero-value must serialize as null (not an invalid transaction object).
	emptyResult := BillingCalculationResult{TransactionPayload: json.RawMessage("{}")}
	data, err := json.Marshal(emptyResult)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"transactionPayload":{}`,
		"zero-amount result must serialize transactionPayload as {}")
}

// getFieldIndex returns the index of a struct field by name, failing the test if not found.
func getFieldIndex(t *testing.T, structType reflect.Type, fieldName string) int {
	t.Helper()

	for i := range structType.NumField() {
		if structType.Field(i).Name == fieldName {
			return i
		}
	}

	t.Fatalf("field %q not found in struct %s", fieldName, structType.Name())

	return -1
}

// TestBillingCalculateRequest_Validation verifies that the required fields of
// BillingCalculateRequest are correctly tagged and that optional fields are optional.
func TestBillingCalculateRequest_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request BillingCalculateRequest
		// wantJSON checks that the JSON key is present in the serialised form.
		wantJSONKeys    []string
		wantOmitJSONKey string
	}{
		{
			name: "all fields present",
			request: BillingCalculateRequest{
				OrganizationID: "org-001",
				LedgerID:       "ledger-001",
				Period:         "2026-01",
				Type:           "volume",
			},
			wantJSONKeys:    []string{"ledgerId", "period", "type"},
			wantOmitJSONKey: "organizationId",
		},
		{
			name: "type omitted when empty",
			request: BillingCalculateRequest{
				OrganizationID: "org-001",
				LedgerID:       "ledger-001",
				Period:         "2026-01",
			},
			wantJSONKeys:    []string{"ledgerId", "period"},
			wantOmitJSONKey: "type",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.request)
			require.NoError(t, err)

			jsonStr := string(data)

			for _, key := range tt.wantJSONKeys {
				assert.Contains(t, jsonStr, `"`+key+`"`)
			}

			if tt.wantOmitJSONKey != "" {
				assert.NotContains(t, jsonStr, `"`+tt.wantOmitJSONKey+`"`)
			}
		})
	}
}

// TestBillingCalculateRequest_RequiredFieldValidateTags verifies that the struct tags
// on BillingCalculateRequest are correct for required fields.
func TestBillingCalculateRequest_RequiredFieldValidateTags(t *testing.T) {
	t.Parallel()

	reqType := reflect.TypeOf(BillingCalculateRequest{})

	requiredFields := []string{"LedgerID", "Period"}

	for _, fieldName := range requiredFields {
		field, ok := reqType.FieldByName(fieldName)
		require.True(t, ok, "field %s not found", fieldName)
		assert.Contains(t, field.Tag.Get("validate"), "required",
			"field %s must have validate:\"required\" tag", fieldName)
	}

	// Type field must NOT have required tag.
	typeField, ok := reqType.FieldByName("Type")
	require.True(t, ok, "field Type not found")
	assert.NotContains(t, typeField.Tag.Get("validate"), "required",
		"field Type must not be required")
}

// TestParseWeeklyPeriod verifies the ISO 8601 weekly period parser across all boundary cases.
func TestParseWeeklyPeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		period    string
		wantOk    bool
		wantStart string // "2006-01-02T15:04:05Z"
		wantEnd   string
	}{
		// Valid cases
		{
			name:      "W01 starts in previous calendar year",
			period:    "2026-W01",
			wantOk:    true,
			wantStart: "2025-12-29T00:00:00Z",
			wantEnd:   "2026-01-05T00:00:00Z",
		},
		{
			name:      "Mid-year week",
			period:    "2026-W13",
			wantOk:    true,
			wantStart: "2026-03-23T00:00:00Z",
			wantEnd:   "2026-03-30T00:00:00Z",
		},
		{
			name:      "Last full week of regular year (W52)",
			period:    "2026-W52",
			wantOk:    true,
			wantStart: "2026-12-21T00:00:00Z",
			wantEnd:   "2026-12-28T00:00:00Z",
		},
		{
			name:      "W53 on year with 53 weeks (2020)",
			period:    "2020-W53",
			wantOk:    true,
			wantStart: "2020-12-28T00:00:00Z",
			wantEnd:   "2021-01-04T00:00:00Z",
		},
		{
			name:      "W53 on year with 53 weeks (2026)",
			period:    "2026-W53",
			wantOk:    true,
			wantStart: "2026-12-28T00:00:00Z",
			wantEnd:   "2027-01-04T00:00:00Z",
		},
		// Invalid cases
		{
			name:   "W53 on year with only 52 weeks",
			period: "2025-W53",
			wantOk: false,
		},
		{
			name:   "W00 below minimum",
			period: "2026-W00",
			wantOk: false,
		},
		{
			name:   "W54 above maximum",
			period: "2026-W54",
			wantOk: false,
		},
		{
			name:   "Single-digit week not ISO 8601",
			period: "2026-W1",
			wantOk: false,
		},
		{
			name:   "Year with fewer than 4 digits",
			period: "202-W01",
			wantOk: false,
		},
		{
			name:   "Non-numeric year",
			period: "202X-W01",
			wantOk: false,
		},
		{
			name:   "Non-numeric week body",
			period: "2026-WXX",
			wantOk: false,
		},
		{
			name:   "Monthly format not matched",
			period: "2026-01",
			wantOk: false,
		},
		{
			name:   "Empty string",
			period: "",
			wantOk: false,
		},
		{
			name:   "Garbage input",
			period: "not-a-period",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			start, end, ok := ParseWeeklyPeriod(tt.period)

			assert.Equal(t, tt.wantOk, ok, "ParseWeeklyPeriod(%q) ok mismatch", tt.period)

			if tt.wantOk {
				assert.Equal(t, tt.wantStart, start.Format("2006-01-02T15:04:05Z"),
					"start mismatch for %q", tt.period)
				assert.Equal(t, tt.wantEnd, end.Format("2006-01-02T15:04:05Z"),
					"end mismatch for %q", tt.period)
			} else {
				assert.True(t, start.IsZero(), "start must be zero on failure for %q", tt.period)
				assert.True(t, end.IsZero(), "end must be zero on failure for %q", tt.period)
			}
		})
	}
}

// TestLooksLikeWeeklyPeriod verifies the structural-only weekly period matcher.
// It must accept valid-format strings regardless of ISO week existence, and reject
// anything that does not structurally match "YYYY-Www".
func TestLooksLikeWeeklyPeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		period string
		want   bool
	}{
		// Accepted: structurally valid regardless of week existence
		{"Valid W01", "2026-W01", true},
		{"Valid W13", "2026-W13", true},
		{"Valid W52", "2026-W52", true},
		{"Valid W53", "2026-W53", true},
		// Non-existent but structurally valid — LooksLike returns true
		{"Non-existent W53 on 52-week year", "2025-W53", true},
		// Rejected: structural failures
		{"Single-digit week W1", "2026-W1", false},
		{"W00 below range", "2026-W00", false},
		{"W54 above range", "2026-W54", false},
		{"Year too short", "202-W01", false},
		{"Year too long", "20260-W01", false},
		{"Non-numeric year", "202X-W01", false},
		{"Non-numeric week", "2026-WXX", false},
		{"Monthly format", "2026-01", false},
		{"Daily format", "2026-01-15", false},
		{"Empty string", "", false},
		{"Garbage", "not-a-period", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := LooksLikeWeeklyPeriod(tt.period)
			assert.Equal(t, tt.want, got, "LooksLikeWeeklyPeriod(%q)", tt.period)
		})
	}
}

// TestDiscountDetail_JSONRoundtrip verifies the DiscountDetail struct serialises correctly.
func TestDiscountDetail_JSONRoundtrip(t *testing.T) {
	t.Parallel()

	detail := DiscountDetail{
		DiscountPercentage: decimal.NewFromFloat(15.5),
		DiscountAmount:     decimal.NewFromFloat(23.25),
		MinQuantity:        100,
	}

	data, err := json.Marshal(detail)
	require.NoError(t, err)

	var got DiscountDetail

	err = json.Unmarshal(data, &got)
	require.NoError(t, err)

	assert.True(t, detail.DiscountPercentage.Equal(got.DiscountPercentage))
	assert.True(t, detail.DiscountAmount.Equal(got.DiscountAmount))
	assert.Equal(t, detail.MinQuantity, got.MinQuantity)
}

// TestBillingCalculateSummary_JSONRoundtrip verifies the summary struct serialises correctly.
func TestBillingCalculateSummary_JSONRoundtrip(t *testing.T) {
	t.Parallel()

	summary := BillingCalculateSummary{
		TotalResults:     5,
		TotalVolume:      3,
		TotalMaintenance: 2,
		TotalNetAmount:   decimal.NewFromFloat(1500.75),
	}

	data, err := json.Marshal(summary)
	require.NoError(t, err)

	var got BillingCalculateSummary

	err = json.Unmarshal(data, &got)
	require.NoError(t, err)

	assert.Equal(t, summary.TotalResults, got.TotalResults)
	assert.Equal(t, summary.TotalVolume, got.TotalVolume)
	assert.Equal(t, summary.TotalMaintenance, got.TotalMaintenance)
	assert.True(t, summary.TotalNetAmount.Equal(got.TotalNetAmount))
}
