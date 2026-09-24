// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
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
							require.Equal(t, "0526", rejected.Code)
						} else {
							require.Equal(t, "0177", rejected.Code)
						}
					}
					attempt.Result = nil
					got = contextTracerDisposition(settings, attempt, errors.New("response lost"))
					require.Equal(t, mode == "advisory" || posture == "open", got.Kind == reservationProceed)
					attempt.Frozen = false
					got = contextTracerDisposition(settings, attempt, errors.New("journal outcome unknown"))
					require.Equal(t, reservationReject, got.Kind, "fail-open never bypasses uncertain dispatch ownership")
				})
			}
		}
	}
}
