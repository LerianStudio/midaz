// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// contextRecorder keeps the request context the reservation services see.
type contextRecorder struct {
	mu  sync.Mutex
	got context.Context
}

func (r *contextRecorder) record(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.got = ctx
}

func (r *contextRecorder) context() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.got
}

// TestNewRoutes_MultiTenantReservationReachesTheHandlerUnderTheTokenTenant
// drives every reservation path through NewRoutes under multi-tenancy with a
// tenant-manager application token for an associated tenant. The real
// lib-commons tenant middleware resolves the tracer pool, and the reservation
// service runs with the token tenant and its pool bound.
func TestNewRoutes_MultiTenantReservationReachesTheHandlerUnderTheTokenTenant(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../../../../../pkg/tracercontract/testdata/reserve_request.json")
	require.NoError(t, err)

	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	request, err := tracercontract.DecodeReserveJSON(t.Context(), raw, 65536, bounds)
	require.NoError(t, err)

	completionBody := `{"contractRevision":"` + tracercontract.ReserveContractRevision + `"}`
	evaluation := testutil.MustDeterministicUUID(88911)

	for name, tc := range map[string]struct {
		path   string
		body   string
		status int
		expect func(*mocks.MockContextReserveAdmitter, *mocks.MockContextReserveCompleter, *mocks.MockContextReserveIDCompleter, *contextRecorder)
	}{
		"reserve": {
			path: "/v1/reservations", body: string(raw), status: http.StatusCreated,
			expect: func(admission *mocks.MockContextReserveAdmitter, _ *mocks.MockContextReserveCompleter, _ *mocks.MockContextReserveIDCompleter, rec *contextRecorder) {
				admission.EXPECT().Execute(gomock.Any(), request).DoAndReturn(func(ctx context.Context, input tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
					rec.record(ctx)

					return &tracercontract.ReserveResult{
						ContractRevision: input.ContractRevision, TransactionID: input.TransactionID, EvaluationID: evaluation,
						Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated},
						ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied},
					}, nil
				})
			},
		},
		"confirm by transaction": {
			path: "/v1/reservations/transaction/" + request.TransactionID.String() + "/confirm", body: completionBody, status: http.StatusOK,
			expect: func(_ *mocks.MockContextReserveAdmitter, completion *mocks.MockContextReserveCompleter, _ *mocks.MockContextReserveIDCompleter, rec *contextRecorder) {
				completion.EXPECT().ExecuteReport(gomock.Any(), request.TransactionID, model.OperationConfirmed).DoAndReturn(transactionCompletion(rec, "CONFIRMED"))
			},
		},
		"release by transaction": {
			path: "/v1/reservations/transaction/" + request.TransactionID.String() + "/release", body: completionBody, status: http.StatusOK,
			expect: func(_ *mocks.MockContextReserveAdmitter, completion *mocks.MockContextReserveCompleter, _ *mocks.MockContextReserveIDCompleter, rec *contextRecorder) {
				completion.EXPECT().ExecuteReport(gomock.Any(), request.TransactionID, model.OperationReleased).DoAndReturn(transactionCompletion(rec, "RELEASED"))
			},
		},
		"confirm by reservation": {
			path: "/v1/reservations/" + request.RequestID.String() + "/confirm", body: completionBody, status: http.StatusOK,
			expect: func(_ *mocks.MockContextReserveAdmitter, _ *mocks.MockContextReserveCompleter, byID *mocks.MockContextReserveIDCompleter, rec *contextRecorder) {
				byID.EXPECT().Execute(gomock.Any(), request.RequestID, model.OperationConfirmed).DoAndReturn(reservationCompletion(rec, request.TransactionID, evaluation, "CONFIRMED"))
			},
		},
		"release by reservation": {
			path: "/v1/reservations/" + request.RequestID.String() + "/release", body: completionBody, status: http.StatusOK,
			expect: func(_ *mocks.MockContextReserveAdmitter, _ *mocks.MockContextReserveCompleter, byID *mocks.MockContextReserveIDCompleter, rec *contextRecorder) {
				byID.EXPECT().Execute(gomock.Any(), request.RequestID, model.OperationReleased).DoAndReturn(reservationCompletion(rec, request.TransactionID, evaluation, "RELEASED"))
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			admission := mocks.NewMockContextReserveAdmitter(ctrl)
			completion := mocks.NewMockContextReserveCompleter(ctrl)
			byID := mocks.NewMockContextReserveIDCompleter(ctrl)
			rec := &contextRecorder{}
			tc.expect(admission, completion, byID, rec)

			handler, err := NewContextReservationHandler(admission, completion, byID, bounds, 65536, 100)
			require.NoError(t, err)

			connections := startTracerPoolFake(t)

			var (
				mu      sync.Mutex
				lookups []string
			)

			app := multiTenantRoutes(t, startAccessManagerFake(t), connections, func(_ context.Context, tenantID, service string) error {
				mu.Lock()
				defer mu.Unlock()

				lookups = append(lookups, service+":"+tenantID)

				return nil
			}, func(deps *RoutesDeps) { deps.ContextReservation = handler })

			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+tenantProducerToken(t, nil))

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			response := readProducerAuthResponse(t, resp)
			require.Equal(t, tc.status, response.status, string(response.body))

			ctx := rec.context()
			require.NotNil(t, ctx, "the reservation service ran")
			requireLedgerProducer(t, ctx)
			require.Equal(t, mtTokenTenant, tmcore.GetTenantIDContext(ctx), "the service runs under the token tenant")
			require.NotNil(t, tmcore.GetPGContext(ctx), "the token tenant's tracer pool is bound")
			mu.Lock()
			require.Equal(t, []string{producerauth.ServiceLedger + ":" + mtTokenTenant}, lookups, "the ledger association is checked once")
			mu.Unlock()

			paths := connections.recorded()
			require.NotEmpty(t, paths, "the tenant middleware resolved the tracer pool")

			for _, path := range paths {
				require.Contains(t, path, "/tenants/"+mtTokenTenant+"/", "the pool is resolved for the token tenant")
			}
		})
	}
}

func transactionCompletion(rec *contextRecorder, status string) func(context.Context, uuid.UUID, model.ReserveOperationStatus) (*tracercontract.TransactionCompletionResult, error) {
	return func(ctx context.Context, id uuid.UUID, _ model.ReserveOperationStatus) (*tracercontract.TransactionCompletionResult, error) {
		rec.record(ctx)

		return &tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: id, Status: status}, nil
	}
}

func reservationCompletion(rec *contextRecorder, transactionID, evaluation uuid.UUID, status string) func(context.Context, uuid.UUID, model.ReserveOperationStatus) (*tracercontract.ReservationCompletionResult, error) {
	return func(ctx context.Context, id uuid.UUID, _ model.ReserveOperationStatus) (*tracercontract.ReservationCompletionResult, error) {
		rec.record(ctx)

		return &tracercontract.ReservationCompletionResult{
			ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transactionID, ReservationID: id, Status: status, EvaluationID: &evaluation,
		}, nil
	}
}

// acceptingLimitAudit records nothing and accepts every limit audit event.
type acceptingLimitAudit struct{ command.AuditWriter }

func (acceptingLimitAudit) RecordLimitEventWithTx(context.Context, pgdb.DB, model.AuditEventType, model.AuditAction, uuid.UUID, map[string]any, map[string]any, string) error {
	return nil
}

// TestNewRoutes_MultiTenantLimitCreateFollowsTheLedgerAssociation posts a
// merchant-scoped limit through NewRoutes under multi-tenancy. The tenant
// comes from the caller's token and the limit command's definition policy asks
// whether that tenant is associated with the ledger: a ledger tenant is
// refused with 422 0531, any other tenant is persisted, and a lookup that
// cannot answer refuses the write with 503 0161.
func TestNewRoutes_MultiTenantLimitCreateFollowsTheLedgerAssociation(t *testing.T) {
	t.Parallel()

	const (
		ledgerTenant     = "0195d3b45a0170008000000000000011"
		validationTenant = "0195d3b45a0170008000000000000012"
		outageTenant     = "0195d3b45a0170008000000000000013"
	)

	body, err := json.Marshal(map[string]any{
		"name": "Merchant Limit", "limitType": "DAILY", "maxAmount": "1000.00", "asset": "BRL",
		"scopes": []map[string]any{{"merchantId": "550e8400-e29b-41d4-a716-446655440000"}},
	})
	require.NoError(t, err)

	for name, tc := range map[string]struct {
		tenant  string
		status  int
		code    string
		persist bool
	}{
		"ledger tenant is refused":            {tenant: ledgerTenant, status: http.StatusUnprocessableEntity, code: constant.ErrContextLimitsUnavailable.Error()},
		"validations-only tenant is accepted": {tenant: validationTenant, status: http.StatusCreated, persist: true},
		"lookup outage refuses the write":     {tenant: outageTenant, status: http.StatusServiceUnavailable, code: constant.ErrTenantServiceUnavailable.Error()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var (
				mu      sync.Mutex
				lookups []string
			)

			lookup := func(_ context.Context, tenantID, service string) error {
				mu.Lock()
				lookups = append(lookups, service+":"+tenantID)
				mu.Unlock()

				switch tenantID {
				case ledgerTenant:
					return nil
				case validationTenant:
					return fmt.Errorf("tenant is not active for %s: %w", service, tmcore.ErrTenantNotFound)
				default:
					return fmt.Errorf("active tenant list for %s unavailable", service)
				}
			}

			ctrl := gomock.NewController(t)
			repo := command.NewMockLimitRepository(ctrl)
			txBeginner := dbmocks.NewMockTxBeginner(ctrl)

			if tc.persist {
				tx := dbmocks.NewMockTx(ctrl)
				gomock.InOrder(
					txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil),
					repo.EXPECT().CreateWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(ctx context.Context, _ pgdb.DB, _ *model.Limit) error {
						require.Equal(t, tc.tenant, tmcore.GetTenantIDContext(ctx), "the limit is persisted under the token tenant")
						require.NotNil(t, tmcore.GetPGContext(ctx), "the token tenant's tracer pool is bound")

						return nil
					}),
					tx.EXPECT().Commit().Return(nil),
				)
			} else {
				txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			}

			createCmd, err := command.NewCreateLimitCommand(repo, testutil.NewDefaultMockClock(), acceptingLimitAudit{}, txBeginner)
			require.NoError(t, err)

			policy, err := command.NewContextLimitDefinitionPolicy(tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, 10, 4096)
			require.NoError(t, err)

			createCmd.ContextLimits = policy.ScopedTo(producerauth.NewServiceTenancy(lookup, producerauth.ServiceLedger))

			limits := NewMockLimitService(ctrl)
			limits.EXPECT().CreateLimit(gomock.Any(), gomock.Any()).DoAndReturn(createCmd.Execute)

			app := multiTenantRoutes(t, startAccessManagerFake(t), startTracerPoolFake(t), func(context.Context, string, string) error { return nil },
				func(deps *RoutesDeps) { deps.LimitService = limits })

			token := limitAdminToken(t, tc.tenant)
			req := httptest.NewRequest(http.MethodPost, "/v1/limits", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			response := readProducerAuthResponse(t, resp)
			require.Equal(t, tc.status, response.status, string(response.body))

			if tc.code != "" {
				require.Equal(t, tc.code, errorCode(t, response.body))
			}

			require.NotContains(t, string(response.body), "active tenant list", "the lookup cause never reaches the response")

			mu.Lock()
			defer mu.Unlock()

			require.Equal(t, []string{producerauth.ServiceLedger + ":" + tc.tenant}, lookups, "the policy asks the ledger association of the token tenant once")
		})
	}
}

// limitAdminToken is a user token carrying tenantID, the claim the lib-commons
// tenant middleware binds the limit routes to.
func limitAdminToken(t *testing.T, tenantID string) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type": "normal-user", "sub": "acme/limit-admin", "owner": "acme", "tenantId": tenantID,
	}).SignedString([]byte("unverified-test-signature"))
	require.NoError(t, err)

	return signed
}
