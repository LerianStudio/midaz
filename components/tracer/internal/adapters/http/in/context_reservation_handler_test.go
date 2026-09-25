// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextReservationTransportStrictRequest(t *testing.T) {
	for _, scenario := range []string{"valid", "duplicate", "identity absent", "namespace", "service unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			admission := mocks.NewMockContextReserveAdmitter(ctrl)
			completion := mocks.NewMockContextReserveCompleter(ctrl)
			bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
			handler, err := NewContextReservationHandler(admission, completion, mocks.NewMockContextReserveIDCompleter(ctrl), bounds, 65536, 100)
			require.NoError(t, err)
			blocked, longLived := false, false
			account := testutil.MustDeterministicUUID(89801)
			asset := tracercontract.AssetRef{Namespace: "official", ID: "asset", Code: "TOKEN"}
			r := tracercontract.ReserveRequest{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: testutil.MustDeterministicUUID(89802), RequestID: testutil.MustDeterministicUUID(89803), ContextID: "context", ValidationMode: tracercontract.ValidationLimits, TransactionTimestamp: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), LongLived: &longLived, Amount: "10.125", Asset: asset, Context: tracercontract.Context{Accounts: []tracercontract.Account{{ID: account, Type: "native", Status: "ACTIVE", Blocked: &blocked, Asset: asset}}, Entries: []tracercontract.Entry{{AccountID: account, Direction: tracercontract.Debit, Amount: "10.125", Asset: asset}}}}
			raw, err := json.Marshal(r)
			require.NoError(t, err)
			ctx := contextutil.WithIntegrationIdentity(t.Context(), contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "official"})
			switch scenario {
			case "duplicate":
				raw = append(raw[:len(raw)-1], []byte(`,"longLived":true}`)...)
			case "identity absent":
				ctx = t.Context()
			case "namespace":
				ctx = contextutil.WithIntegrationIdentity(t.Context(), contextutil.IntegrationIdentity{ID: "other", AssetNamespace: "other"})
			default:
				result := &tracercontract.ReserveResult{ContractRevision: r.ContractRevision, TransactionID: r.TransactionID, EvaluationID: testutil.MustDeterministicUUID(89804), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesNotRequested, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}
				var serviceErr error
				if scenario == "service unavailable" {
					serviceErr = context.DeadlineExceeded
				}
				admission.EXPECT().Execute(gomock.Any(), r).Return(result, serviceErr)
			}
			result, err := handler.Reserve(ctx, &ReserveInputHuma{RawBody: raw})
			if scenario == "valid" {
				require.NoError(t, err)
				require.Equal(t, r.TransactionID, result.Body.TransactionID)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
			}
		})
	}
}

func TestContextReservationSchemaPresence(t *testing.T) {
	_, api := contextPolicyTestApp(t, NewMockContextPolicyAdminService(gomock.NewController(t)), nil)
	RegisterContextReservationRoutes(api, &ContextReservationHandler{maxBodyBytes: 65536}, nil)
	registry := api.OpenAPI().Components.Schemas
	for _, tc := range []struct {
		typ   reflect.Type
		field string
	}{
		{reflect.TypeFor[tracercontract.ReserveRequest](), "longLived"},
		{reflect.TypeFor[tracercontract.Account](), "blocked"},
		{reflect.TypeFor[tracercontract.Context](), "accounts"},
		{reflect.TypeFor[tracercontract.Context](), "entries"},
		{reflect.TypeFor[tracercontract.ReserveResult](), "reservationIds"},
		{reflect.TypeFor[tracercontract.ReserveResult](), "reasons"},
	} {
		schema := registry.Schema(tc.typ, false, "")
		require.Contains(t, schema.Required, tc.field)
		require.False(t, schema.Properties[tc.field].Nullable, tc.field)
	}
	schema := registry.Schema(reflect.TypeFor[tracercontract.ReserveRequest](), false, "")
	require.Equal(t, []any{tracercontract.ReserveContractRevision}, schema.Properties["contractRevision"].Enum)
	require.Equal(t, []any{string(tracercontract.ValidationLimits), string(tracercontract.ValidationRulesAndLimits)}, schema.Properties["validationMode"].Enum)
	require.Equal(t, huma.TypeString, schema.Properties["amount"].Type)
}

func TestContextReservationPreservesPolicyEvaluationError(t *testing.T) {
	var failure pkg.InternalServerError
	require.ErrorAs(t, canonicalContextReservationError(constant.ErrExpressionEvaluation), &failure)
	require.Equal(t, constant.ErrExpressionEvaluation.Error(), failure.Code)
}

func TestContextReservationReportsCompilationSaturation(t *testing.T) {
	var failure huma.StatusError
	require.ErrorAs(t, canonicalContextReservationError(query.ErrContextPolicyCompilationBusy), &failure)
	require.Equal(t, http.StatusTooManyRequests, failure.GetStatus())
}
