// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextTracerDecisionsPreserveLedgerPosture(t *testing.T) {
	for _, mode := range []string{"advisory", "enforce"} {
		for _, posture := range []string{"open", "closed"} {
			for _, decision := range []tracercontract.Decision{tracercontract.DecisionAllow, tracercontract.DecisionDeny, tracercontract.DecisionReview} {
				t.Run(mode+"/"+posture+"/"+string(decision), func(t *testing.T) {
					settings := mmodel.TracerSettings{Mode: mode, FailPosture: posture}
					attempt := ContextTracerAttempt{IntentAttempted: true, Frozen: true, Result: &tracercontract.ReserveResult{Decision: decision}}
					got := contextTracerDisposition(settings, attempt, nil)
					if mode == "advisory" || decision == tracercontract.DecisionAllow {
						require.Equal(t, reservationProceed, got.Kind)
						require.NoError(t, got.Err)
					} else {
						require.Equal(t, reservationReject, got.Kind)
						var rejected pkg.UnprocessableOperationError
						require.ErrorAs(t, got.Err, &rejected)
						if decision == tracercontract.DecisionReview {
							require.Equal(t, "0534", rejected.Code)
						} else {
							require.Equal(t, "0177", rejected.Code)
						}
					}
					attempt.Result = nil
					got = contextTracerDisposition(settings, attempt, fmt.Errorf("response lost: %w", traceradapter.ErrTracerUnavailable))
					require.Equal(t, mode == "advisory" || posture == "open", got.Kind == reservationProceed)
					attempt.Frozen = false
					got = contextTracerDisposition(settings, attempt, errors.New("journal outcome unknown"))
					require.Equal(t, reservationReject, got.Kind, "fail-open never bypasses uncertain dispatch ownership")
				})
			}
		}
	}
}

func TestContextTracerRejectsDeterministicFailuresInEveryPosture(t *testing.T) {
	for _, mode := range []string{mmodel.TracerModeAdvisory, mmodel.TracerModeEnforce} {
		for _, posture := range []string{mmodel.TracerFailPostureOpen, mmodel.TracerFailPostureClosed} {
			for _, cause := range []error{constant.ErrInvalidRequestBody, constant.ErrPayloadTooLarge, constant.ErrTracerFactsUnavailable, constant.ErrContextPolicyUnavailable, constant.ErrContextLimitsUnavailable, constant.ErrExpressionCostExceeded, constant.ErrExpressionEvaluation, constant.ErrTracerContractUnavailable, errors.New("unclassified failure")} {
				t.Run(mode+"/"+posture+"/"+cause.Error(), func(t *testing.T) {
					settings := mmodel.TracerSettings{Mode: mode, FailPosture: posture}
					for _, attempt := range []ContextTracerAttempt{{}, {IntentAttempted: true, Frozen: true}} {
						outcome := contextTracerDisposition(settings, attempt, fmt.Errorf("admission: %w", cause))
						require.Equal(t, reservationReject, outcome.Kind)
						require.Error(t, outcome.Err)
						require.Equal(t, "context_invalid", tracerAdmissionMetric(attempt, outcome, cause))
					}
				})
			}
		}
	}
}

func TestContextTracerDeadlineStillFollowsPosture(t *testing.T) {
	for _, posture := range []string{mmodel.TracerFailPostureOpen, mmodel.TracerFailPostureClosed} {
		outcome := contextTracerDisposition(mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: posture}, ContextTracerAttempt{IntentAttempted: true, Frozen: true}, context.DeadlineExceeded)
		require.Equal(t, posture == mmodel.TracerFailPostureOpen, outcome.Kind == reservationProceed)
	}
}
