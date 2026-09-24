// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func reserveResult() tracercontract.ReserveResult {
	return tracercontract.ReserveResult{
		ContractRevision: tracercontract.ReserveContractRevision,
		TransactionID:    uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		EvaluationID:     uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		Decision:         tracercontract.DecisionAllow,
		Controls:         tracercontract.ReserveControls{Rules: tracercontract.RulesNotRequested, Limits: tracercontract.LimitsEvaluated},
		ReservationIDs:   []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied},
	}
}

func TestReserveResultMatchesRequestedControls(t *testing.T) {
	for _, scenario := range []string{"limits", "combined", "missing rules", "unexpected rules", "other transaction", "other revision", "unknown mode"} {
		t.Run(scenario, func(t *testing.T) {
			result := reserveResult()
			request := tracercontract.ReserveRequest{ContractRevision: result.ContractRevision, TransactionID: result.TransactionID, ValidationMode: tracercontract.ValidationLimits}
			switch scenario {
			case "combined":
				request.ValidationMode = tracercontract.ValidationRulesAndLimits
				result.Controls.Rules = tracercontract.RulesEvaluated
			case "missing rules":
				request.ValidationMode = tracercontract.ValidationRulesAndLimits
			case "unexpected rules":
				result.Controls.Rules = tracercontract.RulesEvaluated
			case "other transaction":
				request.TransactionID = result.EvaluationID
			case "other revision":
				request.ContractRevision = "unknown"
			case "unknown mode":
				request.ValidationMode = ""
			}
			err := result.ValidateFor(request, 10)
			if scenario == "limits" || scenario == "combined" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
			}
		})
	}
}

func TestReserveResultPresenceAndControls(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		change  func(*tracercontract.ReserveResult)
		invalid bool
	}{
		{"allow without limits", func(*tracercontract.ReserveResult) {}, false},
		{"review without capacity", func(r *tracercontract.ReserveResult) {
			r.Decision = tracercontract.DecisionReview
			r.Controls.Rules = tracercontract.RulesEvaluated
			r.Reasons = append(r.Reasons, tracercontract.ReasonRuleReview)
		}, false},
		{"rule deny skips limits", func(r *tracercontract.ReserveResult) {
			r.Decision = tracercontract.DecisionDeny
			r.Controls.Rules = tracercontract.RulesEvaluated
			r.Controls.Limits = tracercontract.LimitsSkippedRuleDeny
			r.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonRuleDeny}
		}, false},
		{"missing revision", func(r *tracercontract.ReserveResult) { r.ContractRevision = "" }, true},
		{"missing transaction", func(r *tracercontract.ReserveResult) { r.TransactionID = uuid.Nil }, true},
		{"missing evaluation", func(r *tracercontract.ReserveResult) { r.EvaluationID = uuid.Nil }, true},
		{"missing decision", func(r *tracercontract.ReserveResult) { r.Decision = "" }, true},
		{"missing rules control", func(r *tracercontract.ReserveResult) { r.Controls.Rules = "" }, true},
		{"missing limits control", func(r *tracercontract.ReserveResult) { r.Controls.Limits = "" }, true},
		{"allow without evaluated limits", func(r *tracercontract.ReserveResult) { r.Controls.Limits = tracercontract.LimitsSkippedRuleDeny }, true},
		{"review requires rules", func(r *tracercontract.ReserveResult) { r.Decision = tracercontract.DecisionReview }, true},
		{"absent reservation array", func(r *tracercontract.ReserveResult) { r.ReservationIDs = nil }, true},
		{"zero reservation", func(r *tracercontract.ReserveResult) { r.ReservationIDs = []uuid.UUID{uuid.Nil} }, true},
		{"duplicate reservation", func(r *tracercontract.ReserveResult) { r.ReservationIDs = []uuid.UUID{r.EvaluationID, r.EvaluationID} }, true},
		{"denial cannot hold capacity", func(r *tracercontract.ReserveResult) {
			r.Decision = tracercontract.DecisionDeny
			r.ReservationIDs = []uuid.UUID{r.EvaluationID}
		}, true},
		{"missing reasons", func(r *tracercontract.ReserveResult) { r.Reasons = nil }, true},
		{"unknown reason", func(r *tracercontract.ReserveResult) { r.Reasons = []tracercontract.ReserveReason{"CUSTOM"} }, true},
		{"duplicate reason", func(r *tracercontract.ReserveResult) { r.Reasons = append(r.Reasons, r.Reasons[0]) }, true},
		{"unsorted reasons", func(r *tracercontract.ReserveResult) {
			r.Reasons = append([]tracercontract.ReserveReason{tracercontract.ReasonRuleAllow}, r.Reasons...)
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reserveResult()
			tt.change(&r)
			err := r.Validate(10)
			if tt.invalid {
				require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
			} else {
				require.NoError(t, err)
			}
		})
	}
	r := reserveResult()
	r.ReservationIDs = []uuid.UUID{r.EvaluationID, r.TransactionID}
	require.ErrorIs(t, r.Validate(1), constant.ErrInvalidRequestBody)
	require.ErrorIs(t, r.Validate(0), constant.ErrInvalidRequestBody)
	data, err := json.Marshal(reserveResult())
	require.NoError(t, err)
	require.Contains(t, string(data), `"reservationIds":[]`)
}
