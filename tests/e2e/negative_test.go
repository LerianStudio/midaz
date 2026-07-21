// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

// TestNegativeTransactionContracts asserts the error contracts the transaction
// API must honor — the wrong status code here is a regression that silently
// changes how every client handles failures.
func TestNegativeTransactionContracts(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	src := createAccount(t, f, "@src")
	dst := createAccount(t, f, "@dst")
	fund(t, f, src, "1000")

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{
			name: "insufficient funds",
			body: transferBody(src, dst, "1000000000", nil),
			want: http.StatusUnprocessableEntity, // 422 — ErrInsufficientFunds
		},
		{
			name: "unknown source alias",
			body: transferBody("@nonexistent", dst, "1", nil),
			want: http.StatusUnprocessableEntity, // 422 — account ineligibility
		},
		{
			name: "unbalanced source/destination totals",
			body: map[string]any{
				"description": "unbalanced",
				"send": map[string]any{
					"asset": "USD", "value": "10",
					"source":     map[string]any{"from": []any{map[string]any{"accountAlias": src, "amount": map[string]any{"asset": "USD", "value": "10"}}}},
					"distribute": map[string]any{"to": []any{map[string]any{"accountAlias": dst, "amount": map[string]any{"asset": "USD", "value": "5"}}}},
				},
			},
			want: http.StatusUnprocessableEntity, // 422 — transaction value mismatch
		},
		{
			name: "unknown field in body",
			body: map[string]any{
				"description": "bogus",
				"bogusField":  "x",
				"send":        transferBody(src, dst, "1", nil)["send"],
			},
			want: http.StatusBadRequest, // 400 — unexpected fields (proves F1 fix didn't over-relax string fields)
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := call(t, http.MethodPost, f.ledgers()+"/transactions/json", tc.body)
			if r.status != tc.want {
				t.Errorf("want %d, got %d\nbody: %s", tc.want, r.status, r.body)
			}
		})
	}
}

// TestNegativeAccountAndPackageContracts covers the onboarding/fees error paths.
func TestNegativeAccountAndPackageContracts(t *testing.T) {
	requireStack(t)

	f := newFixture(t, false)
	createAccount(t, f, "@dup")

	t.Run("duplicate alias is a conflict", func(t *testing.T) {
		r := call(t, http.MethodPost, f.ledgers()+"/accounts", map[string]any{
			"name": "dup2", "assetCode": "USD", "type": "deposit", "alias": "@dup",
		})
		if r.status != http.StatusConflict { // 409 — alias unavailable
			t.Errorf("duplicate alias: want 409, got %d\nbody: %s", r.status, r.body)
		}
	})

	t.Run("holder-owned account requires type", func(t *testing.T) {
		holderID := createHolder(t, f.orgID)
		r := call(t, http.MethodPost, f.ledgers()+"/holders/"+holderID+"/accounts", map[string]any{
			"assetCode": "USD", // type omitted
		})
		if r.status != http.StatusBadRequest { // 400 — missing required field
			t.Errorf("holder account without type: want 400, got %d\nbody: %s", r.status, r.body)
		}
	})

	t.Run("fee package minimum greater than maximum", func(t *testing.T) {
		r := call(t, http.MethodPost, ledgerURL()+"/v1/organizations/"+f.orgID+"/packages", map[string]any{
			"feeGroupLabel": "bad", "ledgerId": f.ledgerID,
			"minimumAmount": "1000", "maximumAmount": "10", "enable": true,
			"fees": map[string]any{"f": map[string]any{
				"feeLabel":         "x",
				"calculationModel": map[string]any{"applicationRule": "flatFee", "calculations": []any{map[string]any{"type": "flat", "value": "1"}}},
				"referenceAmount":  "originalAmount", "priority": 1, "isDeductibleFrom": false, "creditAccount": "@dup",
			}},
		})
		if r.status != http.StatusUnprocessableEntity { // 422 — min>max
			t.Errorf("fee min>max: want 422, got %d\nbody: %s", r.status, r.body)
		}
	})
}

// TestNegativeLedgerSettings guards the settings-validation sharp edge: a
// partial settings object leaves tracer.mode="" which the API rejects, rather
// than defaulting the omitted sub-fields.
func TestNegativeLedgerSettings(t *testing.T) {
	requireStack(t)

	orgID := createOrg(t)
	r := call(t, http.MethodPost, ledgerURL()+"/v1/organizations/"+orgID+"/ledgers", map[string]any{
		"name":     "partial-settings",
		"settings": map[string]any{"overrides": map[string]any{"allowFeeSkip": true}}, // tracer/accounting omitted
	})
	if r.status != http.StatusBadRequest { // 400 — invalid settings field value (tracer.mode)
		t.Errorf("partial settings: want 400, got %d\nbody: %s", r.status, r.body)
	}
}
