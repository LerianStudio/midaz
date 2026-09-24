// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextReservationByID(t *testing.T) {
	for _, scenario := range []string{"confirm", "release", "missing identity", "unsupported revision", "foreign reservation", "invalid response"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			completer := mocks.NewMockContextReserveIDCompleter(ctrl)
			config := ContextReservationConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxBodyBytes: 65536, MaxReservations: 100}
			server, err := NewContextReservationServer(mocks.NewMockReservationService(ctrl), testutil.NewDefaultMockClock(), mocks.NewMockContextReserveAdmitter(ctrl), mocks.NewMockContextReserveCompleter(ctrl), completer, config)
			require.NoError(t, err)
			ctx := contextutil.WithIntegrationIdentity(t.Context(), contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "official"})
			id := testutil.MustDeterministicUUID(88951)
			transaction := testutil.MustDeterministicUUID(88952)
			evaluation := testutil.MustDeterministicUUID(88953)
			revision := tracercontract.ReserveContractRevision
			expected := codes.OK
			outcome := model.OperationConfirmed
			if scenario == "release" {
				outcome = model.OperationReleased
			}
			switch scenario {
			case "missing identity":
				ctx = t.Context()
				expected = codes.PermissionDenied
			case "unsupported revision":
				revision = "unsupported"
				expected = codes.InvalidArgument
			default:
				result := &tracercontract.ReservationCompletionResult{ContractRevision: revision, ReservationID: id, TransactionID: transaction, Status: string(outcome), EvaluationID: &evaluation}
				var commandErr error
				if scenario == "foreign reservation" {
					commandErr = constant.ErrReservationNotFound
					expected = codes.NotFound
				}
				if scenario == "invalid response" {
					result.ReservationID = uuid.Nil
					expected = codes.Internal
				}
				completer.EXPECT().Execute(gomock.Any(), id, outcome).Return(result, commandErr)
			}
			if scenario == "release" {
				result, err := server.ReleaseById(ctx, &reservationv1.ReleaseByIdRequest{ContractRevision: revision, ReservationId: id.String()})
				require.NoError(t, err)
				require.Equal(t, id.String(), result.GetReservationId())
				require.Equal(t, transaction.String(), result.GetTransactionId())
				require.Equal(t, string(outcome), result.GetStatus())
			} else {
				result, err := server.ConfirmById(ctx, &reservationv1.ConfirmByIdRequest{ContractRevision: revision, ReservationId: id.String()})
				require.Equal(t, expected, status.Code(err))
				if expected == codes.OK {
					require.Equal(t, id.String(), result.GetReservationId())
					require.Equal(t, evaluation.String(), result.GetEvaluationId())
				} else {
					require.Nil(t, result)
				}
			}
		})
	}
}
