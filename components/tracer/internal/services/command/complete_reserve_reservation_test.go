// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestReservationCompletionUsesAuthenticatedOwner(t *testing.T) {
	for _, scenario := range []string{"complete", "absent identity", "missing", "foreign owner", "wrong reservation", "wrong evaluation", "conflict", "invalid status", "missing tenant"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			locator := mocks.NewMockReservationOperationLocator(ctrl)
			reporter := mocks.NewMockReserveOperationReporter(ctrl)
			cmd, err := NewCompleteReserveReservationCommand(locator, reporter, scenario != "missing tenant")
			require.NoError(t, err)
			ctx := completionAuth(t.Context())
			reservation := testutil.MustDeterministicUUID(88911)
			transaction := testutil.MustDeterministicUUID(88912)
			evaluation := testutil.MustDeterministicUUID(88913)
			owner := &model.ReserveReservationOwner{ReservationID: reservation, EvaluationID: evaluation, Operation: model.ReserveOperationIdentity{IntegrationID: "verified-producer", TransactionID: transaction}}
			outcome := model.OperationConfirmed
			expected := constant.ErrInternalServer
			switch scenario {
			case "absent identity":
				ctx = t.Context()
				expected = constant.ErrInsufficientPrivileges
			case "invalid status":
				outcome = model.OperationOpen
				expected = constant.ErrInvalidRequestBody
			case "missing tenant":
				expected = constant.ErrReservationTenantRequired
			default:
				if scenario == "foreign owner" {
					owner.Operation.IntegrationID = "other"
				}
				if scenario == "wrong reservation" {
					owner.ReservationID = uuid.Nil
				}
				if scenario == "missing" {
					owner = nil
					expected = constant.ErrReservationNotFound
				}
				locator.EXPECT().GetReservationOwner(gomock.Any(), "verified-producer", reservation).Return(owner, nil)
				if scenario == "complete" || scenario == "wrong evaluation" || scenario == "conflict" {
					result := &tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transaction, Status: "CONFIRMED", EvaluationID: &evaluation}
					if scenario == "wrong evaluation" {
						result.EvaluationID = &transaction
					}
					var completeErr error
					if scenario == "conflict" {
						completeErr = constant.ErrReserveOperationConflict
						expected = completeErr
					}
					reporter.EXPECT().ExecuteReport(gomock.Any(), transaction, model.OperationConfirmed).Return(result, completeErr)
				}
			}
			result, err := cmd.Execute(ctx, reservation, outcome)
			if scenario == "complete" {
				require.NoError(t, err)
				require.NoError(t, result.Validate())
				require.Equal(t, reservation, result.ReservationID)
				require.Equal(t, transaction, result.TransactionID)
			} else {
				require.ErrorIs(t, err, expected)
				require.Nil(t, result)
			}
		})
	}
}
