// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package protobuf

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestReserveRESTProtobufEquivalence(t *testing.T) {
	raw, err := os.ReadFile("../testdata/reserve_request.json")
	require.NoError(t, err)
	limits := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	expected, err := tracercontract.DecodeReserveJSON(t.Context(), raw, 65536, limits)
	require.NoError(t, err)
	encoded, err := EncodeReserve(t.Context(), expected, limits)
	require.NoError(t, err)
	require.Equal(t, "BTC", encoded.Asset)
	require.Equal(t, "BTC", encoded.Context.Accounts[0].Asset)
	require.Equal(t, "BTC", encoded.Context.Entries[0].Asset)
	wire, err := proto.Marshal(encoded)
	require.NoError(t, err)
	var received reservationv1.ReserveRequest
	require.NoError(t, proto.Unmarshal(wire, &received))
	actual, err := DecodeReserve(t.Context(), &received, limits, 65536)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	require.Equal(t, "BTC", actual.Asset)
	require.Equal(t, "BTC", actual.Context.Accounts[0].Asset)
	for _, entry := range actual.Context.Entries {
		require.Equal(t, "BTC", entry.Asset)
	}
	scope := tracercontract.ReserveScope{IntegrationID: "producer", SingleTenant: true}
	before, err := expected.Fingerprint(t.Context(), scope, limits)
	require.NoError(t, err)
	after, err := actual.Fingerprint(t.Context(), scope, limits)
	require.NoError(t, err)
	require.Equal(t, before, after)
	for _, scenario := range []string{"missing presence", "unknown field", "legacy field", "nested unknown", "empty asset", "entry asset nul", "entry asset over hundred characters", "oversize"} {
		t.Run(scenario, func(t *testing.T) {
			request := proto.Clone(encoded).(*reservationv1.ReserveRequest)
			bound := 65536
			switch scenario {
			case "missing presence":
				request.Context.Accounts[0].Blocked = nil
			case "unknown field":
				request.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 99, protowire.VarintType), 1))
			case "legacy field":
				request.ProtoReflect().SetUnknown(protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), "legacy"))
			case "nested unknown":
				request.Context.Entries[0].ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 99, protowire.VarintType), 1))
			case "empty asset":
				request.Asset = ""
			case "entry asset nul":
				request.Context.Entries[1].Asset = "US\x00D"
			case "entry asset over hundred characters":
				request.Context.Entries[1].Asset = strings.Repeat("A", 101)
			case "oversize":
				bound = 1
			}
			result, err := DecodeReserve(t.Context(), request, limits, bound)
			require.Error(t, err)
			require.Equal(t, tracercontract.ReserveRequest{}, result)
		})
	}
}

func TestEncodeReserveCarriesStoredAssetCode(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../testdata/reserve_request.json")
	require.NoError(t, err)
	limits := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	request, err := tracercontract.DecodeReserveJSON(t.Context(), raw, 65536, limits)
	require.NoError(t, err)
	request.Context.Entries[1].Asset = "usd"
	encoded, err := EncodeReserve(t.Context(), request, limits)
	require.NoError(t, err, "a stored ledger code outside the uppercase rule is still a fact")
	decoded, err := DecodeReserve(t.Context(), encoded, limits, 65536)
	require.NoError(t, err)
	require.Equal(t, "usd", decoded.Context.Entries[1].Asset)

	request.Context.Entries[1].Asset = ""
	encoded, err = EncodeReserve(t.Context(), request, limits)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	require.Nil(t, encoded)
}

func TestResultCodecRejectsIncompleteDecisions(t *testing.T) {
	transaction := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	evaluation := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	reservation := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	for _, decision := range []tracercontract.Decision{tracercontract.DecisionAllow, tracercontract.DecisionDeny, tracercontract.DecisionReview} {
		t.Run(string(decision), func(t *testing.T) {
			result := &tracercontract.ReserveResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transaction, EvaluationID: evaluation, Decision: decision, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}
			if decision == tracercontract.DecisionAllow {
				result.ReservationIDs = []uuid.UUID{reservation}
			}
			encoded, err := EncodeResult(result, 100)
			require.NoError(t, err)
			actual, err := DecodeResult(encoded, 100)
			require.NoError(t, err)
			require.Equal(t, result, actual)
			for _, scenario := range []string{"missing controls", "unknown decision", "missing evaluation", "unknown nested", "excess capacity", "unknown root"} {
				t.Run(scenario, func(t *testing.T) {
					input := proto.Clone(encoded).(*reservationv1.ReserveResult)
					switch scenario {
					case "missing controls":
						input.Controls = nil
					case "unknown decision":
						input.Decision = "SKIPPED"
					case "missing evaluation":
						input.EvaluationId = ""
					case "unknown nested":
						input.Controls.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 99, protowire.VarintType), 1))
					case "unknown root":
						input.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 99, protowire.VarintType), 1))
					case "excess capacity":
						input.ReservationIds = []string{reservation.String(), reservation.String()}
					}
					actual, err := DecodeResult(input, 1)
					require.Error(t, err)
					require.Nil(t, actual)
				})
			}
		})
	}
}
