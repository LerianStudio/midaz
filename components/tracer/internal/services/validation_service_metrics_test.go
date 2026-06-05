// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"sync"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	commandMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	queryMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// recordingMTMetrics is a test double that implements metrics.MultiTenantMetrics
// via structural typing. Lives in the services package so tests can check
// the emitMessageProcessed hook without reaching across packages.
type recordingMTMetrics struct {
	mu         sync.Mutex
	messages   []struct{ TenantID, Module, Result string }
	connTotals int
}

func (r *recordingMTMetrics) IncConnectionsTotal(_ context.Context, _, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.connTotals++
}

func (r *recordingMTMetrics) IncConnectionErrors(_ context.Context, _, _, _ string) {}

func (r *recordingMTMetrics) IncConsumersActive(_ context.Context, _, _ string) {}

func (r *recordingMTMetrics) DecConsumersActive(_ context.Context, _, _ string) {}

func (r *recordingMTMetrics) IncMessagesProcessed(_ context.Context, tenantID, module, result string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.messages = append(r.messages, struct{ TenantID, Module, Result string }{tenantID, module, result})
}

func (r *recordingMTMetrics) snapshot() []struct{ TenantID, Module, Result string } {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]struct{ TenantID, Module, Result string }, len(r.messages))
	copy(out, r.messages)

	return out
}

// newValidationServiceForMetricsTest returns a ValidationService wired ONLY
// with the fields emitMessageProcessed touches: mtMetrics. every other
// production dependency is deliberately zero-valued — the test exercises
// a single method that does not reach through any other field.
//
// Replaces the raw struct literal &ValidationService{mtMetrics: rec}
// which implicitly assumed that emitMessageProcessed would never read
// another field. The helper makes that invariant explicit: adding a new
// dependency to emitMessageProcessed that is not wired here will fail
// immediately.
//
// Passing a nil sink returns a ValidationService whose mtMetrics interface is
// the untyped-nil — matching the "forgot to wire the sink" failure mode the
// companion test guards against.
func newValidationServiceForMetricsTest(t *testing.T, sink *recordingMTMetrics) *ValidationService {
	t.Helper()

	svc := &ValidationService{}

	if sink != nil {
		svc.mtMetrics = sink
	}

	return svc
}

// TestValidationService_EmitMessageProcessed_ResultMapping verifies the
// decision → result mapping used for tenant_messages_processed_total.
// This is the hook point operators watch on the SLO dashboard; any drift
// here silently reshapes the "% ERROR by tenant" panels.
func TestValidationService_EmitMessageProcessed_ResultMapping(t *testing.T) {
	t.Parallel()

	// Deterministic UUIDs keep the test stable across runs — the project
	// forbids uuid.New() in unit tests for exactly this reason.
	validationID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)

	tests := []struct {
		name         string
		result       *ValidateResult
		retErr       error
		ctx          context.Context
		wantTenantID string
		wantResult   string
	}{
		{
			name:         "error path maps to ERROR",
			result:       nil,
			retErr:       errors.New("boom"),
			ctx:          tmcore.ContextWithTenantID(context.Background(), "tenant-a"),
			wantTenantID: "tenant-a",
			wantResult:   "ERROR",
		},
		{
			name:         "nil response maps to ERROR",
			result:       &ValidateResult{Response: nil},
			retErr:       nil,
			ctx:          tmcore.ContextWithTenantID(context.Background(), "tenant-b"),
			wantTenantID: "tenant-b",
			wantResult:   "ERROR",
		},
		{
			name: "ALLOW decision",
			result: &ValidateResult{
				Response: &model.ValidationResponse{
					ValidationID:     validationID,
					RequestID:        requestID,
					EvaluationResult: model.EvaluationResult{Decision: model.DecisionAllow},
				},
			},
			retErr:       nil,
			ctx:          context.Background(), // single-tenant path
			wantTenantID: "",
			wantResult:   "ALLOW",
		},
		{
			name: "DENY decision",
			result: &ValidateResult{
				Response: &model.ValidationResponse{
					ValidationID:     validationID,
					RequestID:        requestID,
					EvaluationResult: model.EvaluationResult{Decision: model.DecisionDeny},
				},
			},
			retErr:       nil,
			ctx:          tmcore.ContextWithTenantID(context.Background(), "tenant-c"),
			wantTenantID: "tenant-c",
			wantResult:   "DENY",
		},
		{
			name: "REVIEW decision",
			result: &ValidateResult{
				Response: &model.ValidationResponse{
					ValidationID:     validationID,
					RequestID:        requestID,
					EvaluationResult: model.EvaluationResult{Decision: model.DecisionReview},
				},
			},
			retErr:       nil,
			ctx:          tmcore.ContextWithTenantID(context.Background(), "tenant-d"),
			wantTenantID: "tenant-d",
			wantResult:   "REVIEW",
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recordingMTMetrics{}
			svc := newValidationServiceForMetricsTest(t, rec)

			svc.emitMessageProcessed(tc.ctx, tc.result, tc.retErr)

			entries := rec.snapshot()
			require.Len(t, entries, 1, "exactly one emission expected")

			got := entries[0]
			assert.Equal(t, tc.wantTenantID, got.TenantID)
			assert.Equal(t, constant.ModuleName, got.Module)
			assert.Equal(t, tc.wantResult, got.Result)
		})
	}
}

// TestValidationService_EmitMessageProcessed_NilMetrics_NoPanic protects
// against a call site that forgets to wire the sink. The service must stay
// usable — it is on the hot path.
func TestValidationService_EmitMessageProcessed_NilMetrics_NoPanic(t *testing.T) {
	t.Parallel()

	svc := newValidationServiceForMetricsTest(t, nil)

	assert.NotPanics(t, func() {
		svc.emitMessageProcessed(context.Background(), nil, errors.New("x"))
	})
}

// TestValidationService_SetMultiTenantMetrics_NilRestoresNoop verifies the
// setter accepts nil and swaps back to the no-op. This guards the MT=false
// path in bootstrap.
func TestValidationService_SetMultiTenantMetrics_NilRestoresNoop(t *testing.T) {
	t.Parallel()

	rec := &recordingMTMetrics{}
	svc := newValidationServiceForMetricsTest(t, rec)

	svc.SetMultiTenantMetrics(nil)

	// No panic and no emission — the no-op swallows the call.
	svc.emitMessageProcessed(context.Background(), &ValidateResult{
		Response: &model.ValidationResponse{EvaluationResult: model.EvaluationResult{Decision: model.DecisionAllow}},
	}, nil)

	assert.Empty(t, rec.snapshot())
}

// TestValidationService_Validate_EmitsErrorMetric_OnContextCancelled covers
// the early-return guard where ctx is already cancelled. The metric defer
// must be registered BEFORE the guard so SLO dashboards see the failure as
// result="ERROR" instead of silently dropping the sample.
func TestValidationService_Validate_EmitsErrorMetric_OnContextCancelled(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	svc, err := NewValidationService(
		pgdbMocks.NewMockTxBeginner(ctrl),
		mocks.NewMockRuleEvaluator(ctrl),
		mocks.NewMockLimitChecker(ctrl),
		commandMocks.NewMockTransactionValidationRepository(ctrl),
		queryMocks.NewMockTransactionValidationRepository(ctrl),
		mocks.NewMockAuditWriter(ctrl),
		nil,
	)
	require.NoError(t, err)

	rec := &recordingMTMetrics{}
	svc.SetMultiTenantMetrics(rec)

	ctx, cancel := context.WithCancel(tmcore.ContextWithTenantID(context.Background(), "tenant-cancel"))
	cancel() // cancel BEFORE Validate so ctx.Err() returns immediately

	req := &model.ValidationRequest{} // value irrelevant — ctx guard fires first
	result, validateErr := svc.Validate(ctx, req)

	require.Error(t, validateErr)
	assert.Nil(t, result)

	entries := rec.snapshot()
	require.Len(t, entries, 1, "early-return path must still emit one metric")
	assert.Equal(t, "tenant-cancel", entries[0].TenantID)
	assert.Equal(t, constant.ModuleName, entries[0].Module)
	assert.Equal(t, "ERROR", entries[0].Result)
}

// TestValidationService_Validate_EmitsErrorMetric_OnNilRequest covers the
// early-return guard where the request is nil. Same reasoning as the
// context-cancelled case: the metric must fire so SLO dashboards capture
// the failure.
func TestValidationService_Validate_EmitsErrorMetric_OnNilRequest(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	svc, err := NewValidationService(
		pgdbMocks.NewMockTxBeginner(ctrl),
		mocks.NewMockRuleEvaluator(ctrl),
		mocks.NewMockLimitChecker(ctrl),
		commandMocks.NewMockTransactionValidationRepository(ctrl),
		queryMocks.NewMockTransactionValidationRepository(ctrl),
		mocks.NewMockAuditWriter(ctrl),
		nil,
	)
	require.NoError(t, err)

	rec := &recordingMTMetrics{}
	svc.SetMultiTenantMetrics(rec)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-nilreq")

	result, validateErr := svc.Validate(ctx, nil)

	require.Error(t, validateErr)
	assert.Contains(t, validateErr.Error(), "validation request cannot be nil")
	assert.Nil(t, result)

	entries := rec.snapshot()
	require.Len(t, entries, 1, "nil-request guard must still emit one metric")
	assert.Equal(t, "tenant-nilreq", entries[0].TenantID)
	assert.Equal(t, constant.ModuleName, entries[0].Module)
	assert.Equal(t, "ERROR", entries[0].Result)
}
