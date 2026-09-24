// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model_test

import (
	"crypto/sha256"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func storedDecision() model.ReserveDecision {
	return model.ReserveDecision{
		Key:       model.ReserveOperationKey{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(701), RequestID: testutil.MustDeterministicUUID(702)},
		ContextID: "ledger", Fingerprint: sha256.Sum256([]byte("frozen request")), ValidationMode: tracercontract.ValidationLimits, CreatedAt: testutil.FixedTime(),
		Result: tracercontract.ReserveResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: testutil.MustDeterministicUUID(701), EvaluationID: testutil.MustDeterministicUUID(703), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesNotRequested, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}},
	}
}

func TestReserveDecisionValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*model.ReserveDecision)
	}{
		{"missing integration", func(d *model.ReserveDecision) { d.Key.IntegrationID = "" }},
		{"noncanonical integration", func(d *model.ReserveDecision) { d.Key.IntegrationID = " producer" }},
		{"missing request", func(d *model.ReserveDecision) { d.Key.RequestID = uuid.Nil }},
		{"response transaction mismatch", func(d *model.ReserveDecision) { d.Result.TransactionID = testutil.MustDeterministicUUID(999) }},
		{"missing context", func(d *model.ReserveDecision) { d.ContextID = "" }},
		{"invalid mode", func(d *model.ReserveDecision) { d.ValidationMode = "" }},
		{"rules without policy", func(d *model.ReserveDecision) {
			d.ValidationMode = tracercontract.ValidationRulesAndLimits
			d.Result.Controls.Rules = tracercontract.RulesEvaluated
		}},
		{"limits with policy", func(d *model.ReserveDecision) {
			d.Policy = &model.ReserveDecisionPolicy{ID: testutil.MustDeterministicUUID(704), Revision: 1, BindingVersion: 1}
		}},
	}
	require.NoError(t, storedDecision().Validate(10, 10))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := storedDecision()
			tt.change(&d)
			require.ErrorIs(t, d.Validate(10, 10), constant.ErrInvalidRequestBody)
		})
	}
}

func TestReserveDecisionRulesSnapshot(t *testing.T) {
	t.Parallel()
	d := storedDecision()
	d.ValidationMode = tracercontract.ValidationRulesAndLimits
	d.Result.Controls.Rules = tracercontract.RulesEvaluated
	rule := model.RuleRevision{ID: testutil.MustDeterministicUUID(705), Revision: 2}
	d.Policy = &model.ReserveDecisionPolicy{ID: testutil.MustDeterministicUUID(704), Revision: 3, BindingVersion: 8, EvaluatedRules: []model.RuleRevision{rule}, MatchedRules: []model.RuleRevision{rule}}
	require.NoError(t, d.Validate(10, 10))
	d.Policy.MatchedRules[0].Revision++
	require.ErrorIs(t, d.Validate(10, 10), constant.ErrInvalidRequestBody)
	d.Policy.MatchedRules = []model.RuleRevision{}
	require.ErrorIs(t, d.Validate(10, 10), constant.ErrInvalidRequestBody, "default use must be recorded")
	d.Policy.DefaultUsed = true
	require.NoError(t, d.Validate(10, 10))
	d.Policy.EvaluatedRules = append(d.Policy.EvaluatedRules, rule)
	require.ErrorIs(t, d.Validate(10, 10), constant.ErrInvalidRequestBody, "rule list cannot contain duplicates")
}
