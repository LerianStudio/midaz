// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query_test

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func replayConfig() query.ReserveReplayConfig {
	return query.ReserveReplayConfig{Limits: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxRules: 10, MaxReservations: 100}
}

func replayRequest() tracercontract.ReserveRequest {
	longLived := false
	return tracercontract.ReserveRequest{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: testutil.MustDeterministicUUID(71001), RequestID: testutil.MustDeterministicUUID(71002), ContextID: "ledger", ValidationMode: tracercontract.ValidationLimits, TransactionTimestamp: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), LongLived: &longLived, Amount: "0.00000001", Asset: tracercontract.AssetRef{Namespace: "producer", ID: "asset-1", Code: "BTC"}, Context: policyFacts()}
}

func replayContext() context.Context {
	return tmcore.ContextWithTenantID(contextutil.WithIntegrationIdentity(context.Background(), contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "producer"}), "tenant-a")
}

func replayDecision(t *testing.T) model.ReserveDecision {
	t.Helper()
	r := replayRequest()
	fingerprint, err := r.Fingerprint(t.Context(), tracercontract.ReserveScope{TenantID: "tenant-a", IntegrationID: "producer", AssetNamespace: "producer"}, replayConfig().Limits)
	require.NoError(t, err)
	return model.ReserveDecision{Key: model.ReserveOperationKey{IntegrationID: "producer", TransactionID: r.TransactionID, RequestID: r.RequestID}, ContextID: r.ContextID, Fingerprint: fingerprint, ValidationMode: r.ValidationMode, CreatedAt: testutil.FixedTime(), Result: tracercontract.ReserveResult{ContractRevision: r.ContractRevision, TransactionID: r.TransactionID, EvaluationID: testutil.MustDeterministicUUID(71003), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesNotRequested, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}}
}

func TestLookupReserveDecisionReplaysFrozenResponse(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockReserveDecisionReader(gomock.NewController(t))
	q, err := query.NewLookupReserveDecisionQuery(repo, replayConfig())
	require.NoError(t, err)
	d := replayDecision(t)
	repo.EXPECT().Get(gomock.Any(), d.Key).DoAndReturn(func(ctx context.Context, _ model.ReserveOperationKey) (*model.ReserveDecision, error) {
		require.Equal(t, "tenant-a", tmcore.GetTenantIDContext(ctx))
		return &d, nil
	})
	r := replayRequest()
	r.Amount += "00"
	got, err := q.Execute(replayContext(), r)
	require.NoError(t, err)
	require.Equal(t, d, *got)
	got.Result.Reasons[0] = "MUTATED"
	require.Equal(t, tracercontract.ReasonLimitsSatisfied, d.Result.Reasons[0])
}

func TestLookupReserveDecisionConflictsAndMisses(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		change func(*tracercontract.ReserveRequest)
	}{
		{"amount", func(r *tracercontract.ReserveRequest) { r.Amount = "1" }},
		{"context", func(r *tracercontract.ReserveRequest) { r.ContextID = "other-ledger" }},
		{"account fact", func(r *tracercontract.ReserveRequest) { *r.Context.Accounts[0].Blocked = true }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewMockReserveDecisionReader(gomock.NewController(t))
			q, err := query.NewLookupReserveDecisionQuery(repo, replayConfig())
			require.NoError(t, err)
			d := replayDecision(t)
			repo.EXPECT().Get(gomock.Any(), d.Key).Return(&d, nil)
			r := replayRequest()
			tt.change(&r)
			got, err := q.Execute(replayContext(), r)
			require.ErrorIs(t, err, constant.ErrReserveDecisionConflict)
			require.Nil(t, got)
		})
	}
	for _, want := range []error{nil, constant.ErrInternalServer, constant.ErrReserveDecisionConflict} {
		repo := mocks.NewMockReserveDecisionReader(gomock.NewController(t))
		q, err := query.NewLookupReserveDecisionQuery(repo, replayConfig())
		require.NoError(t, err)
		repo.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, want)
		got, err := q.Execute(replayContext(), replayRequest())
		require.ErrorIs(t, err, want)
		require.Nil(t, got)
	}
}

func TestLookupReserveDecisionRejectsBeforeRead(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(replayContext())
	cancel()
	for _, tt := range []struct {
		name   string
		ctx    context.Context
		change func(*tracercontract.ReserveRequest)
		want   error
	}{
		{"no producer", context.Background(), nil, constant.ErrInsufficientPrivileges},
		{"no tenant", contextutil.WithIntegrationIdentity(context.Background(), contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "producer"}), nil, constant.ErrReservationTenantRequired},
		{"canceled", canceled, nil, context.Canceled},
		{"invalid request", replayContext(), func(r *tracercontract.ReserveRequest) { r.LongLived = nil }, constant.ErrInvalidRequestBody},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewMockReserveDecisionReader(gomock.NewController(t))
			q, err := query.NewLookupReserveDecisionQuery(repo, replayConfig())
			require.NoError(t, err)
			r := replayRequest()
			if tt.change != nil {
				tt.change(&r)
			}
			got, err := q.Execute(tt.ctx, r)
			require.ErrorIs(t, err, tt.want)
			require.Nil(t, got)
		})
	}
}

func TestLookupReserveDecisionSingleTenantAndCancellation(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockReserveDecisionReader(gomock.NewController(t))
	config := replayConfig()
	config.SingleTenant = true
	q, err := query.NewLookupReserveDecisionQuery(repo, config)
	require.NoError(t, err)
	ctx := contextutil.WithIntegrationIdentity(context.Background(), contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "producer"})
	repo.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil)
	got, err := q.Execute(ctx, replayRequest())
	require.NoError(t, err)
	require.Nil(t, got)
	canceled, cancel := context.WithCancel(replayContext())
	defer cancel()
	d := replayDecision(t)
	repo.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, model.ReserveOperationKey) (*model.ReserveDecision, error) {
		cancel()
		return &d, nil
	})
	got, err = q.Execute(canceled, replayRequest())
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)
}

func TestLookupReserveDecisionRejectsCorruptRecord(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockReserveDecisionReader(gomock.NewController(t))
	q, err := query.NewLookupReserveDecisionQuery(repo, replayConfig())
	require.NoError(t, err)
	d := replayDecision(t)
	d.Result.ReservationIDs = nil
	repo.EXPECT().Get(gomock.Any(), d.Key).Return(&d, nil)
	got, err := q.Execute(replayContext(), replayRequest())
	require.ErrorIs(t, err, constant.ErrInternalServer)
	require.Nil(t, got)
}
