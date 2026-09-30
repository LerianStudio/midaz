// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"go.opentelemetry.io/otel/codes"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	commandMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	queryMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ruleEvaluationFixture wires a ValidationService whose persistence and limit
// collaborators are mocks, so a test can assert what a validation whose rule
// step failed persists and audits.
type ruleEvaluationFixture struct {
	txBeginner *pgdbMocks.MockTxBeginner
	tx         *pgdbMocks.MockTx
	limits     *mocks.MockLimitChecker
	repo       *commandMocks.MockTransactionValidationRepository
	queryRepo  *queryMocks.MockTransactionValidationRepository
	audit      *mocks.MockAuditWriter
}

func newRuleEvaluationFixture(ctrl *gomock.Controller) *ruleEvaluationFixture {
	f := &ruleEvaluationFixture{
		txBeginner: pgdbMocks.NewMockTxBeginner(ctrl),
		tx:         pgdbMocks.NewMockTx(ctrl),
		limits:     mocks.NewMockLimitChecker(ctrl),
		repo:       commandMocks.NewMockTransactionValidationRepository(ctrl),
		queryRepo:  queryMocks.NewMockTransactionValidationRepository(ctrl),
		audit:      mocks.NewMockAuditWriter(ctrl),
	}

	f.queryRepo.EXPECT().FindByRequestID(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)

	return f
}

// ruleEvaluationRecord is what a validation persisted and audited.
type ruleEvaluationRecord struct {
	persisted *model.TransactionValidation
	audited   *model.EvaluationResult
}

// expectReviewPersisted expects the REVIEW path: limits checked inside the
// transaction, rolled back, then the validation and its audit event persisted
// outside it. Both are captured into record.
func (f *ruleEvaluationFixture) expectReviewPersisted(record *ruleEvaluationRecord) {
	f.txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(f.tx, nil).Times(1)
	f.limits.EXPECT().CheckLimits(gomock.Any(), f.tx, gomock.Any()).
		Return(&model.CheckLimitsOutput{Allowed: true, ExceededLimitIDs: []uuid.UUID{}}, nil).Times(1)
	f.tx.EXPECT().Rollback().Return(nil).Times(1)
	f.expectPersisted(record)
}

// expectPersisted expects the validation and its audit event persisted outside
// any transaction, capturing both into record.
func (f *ruleEvaluationFixture) expectPersisted(record *ruleEvaluationRecord) {
	f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, tv *model.TransactionValidation) error {
			record.persisted = tv
			return nil
		},
	).Times(1)
	f.audit.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ uuid.UUID, _ map[string]any, evalResult model.EvaluationResult, _ model.ValidationResponseContext) error {
			record.audited = &evalResult
			return nil
		},
	).Times(1)
}

func (f *ruleEvaluationFixture) service(t *testing.T, eval RuleEvaluator) *ValidationService {
	t.Helper()

	svc, err := NewValidationService(f.txBeginner, eval, f.limits, f.repo, f.queryRepo, f.audit, nil)
	require.NoError(t, err)

	return svc
}

func ruleEvaluationRequest(metadata map[string]any) *model.ValidationRequest {
	return &model.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(501),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Asset:                "USD",
		TransactionTimestamp: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Account:              model.AccountContext{ID: testutil.MustDeterministicUUID(502)},
		Metadata:             metadata,
	}
}

func assertRuleEvaluationReview(t *testing.T, result *ValidateResult, record *ruleEvaluationRecord, ruleIDs ...uuid.UUID) {
	t.Helper()

	require.NotNil(t, result)
	require.NotNil(t, result.Response)
	assert.False(t, result.IsDuplicate)
	assert.Equal(t, model.DecisionReview, result.Response.Decision)
	assert.Equal(t, reasonRuleEvaluationError, result.Response.Reason)
	assert.Equal(t, ruleIDs, result.Response.MatchedRuleIDs)

	require.NotNil(t, record.persisted, "a REVIEW caused by a rule evaluation failure must be persisted")
	assert.Equal(t, model.DecisionReview, record.persisted.Decision)
	assert.Equal(t, reasonRuleEvaluationError, record.persisted.Reason)
	assert.Equal(t, ruleIDs, record.persisted.MatchedRuleIDs)

	require.NotNil(t, record.audited, "a REVIEW caused by a rule evaluation failure must be audited")
	assert.Equal(t, model.DecisionReview, record.audited.Decision)
	assert.Equal(t, reasonRuleEvaluationError, record.audited.Reason)
	assert.Equal(t, ruleIDs, record.audited.MatchedRuleIDs)
}

// realRuleQuery wires the real rule pipeline (CEL adapter, rule evaluator,
// complete evaluator, rule query) over rules, which the rule store returns in
// the given order: newest first.
func realRuleQuery(t *testing.T, ctrl *gomock.Controller, rules ...*model.Rule) RuleEvaluator {
	t.Helper()

	adapter, err := cel.NewAdapter(cel.AdapterConfig{}, libLog.NewNop())
	require.NoError(t, err)

	single, err := query.NewRuleEvaluator(adapter)
	require.NoError(t, err)

	complete, err := query.NewCompleteEvaluator(single)
	require.NoError(t, err)

	activeRules := query.NewMockGetActiveRulesExecutor(ctrl)
	activeRules.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(rules, nil).Times(1)

	eval, err := query.NewEvaluateRulesQuery(activeRules, complete, &query.EvaluationConfig{
		DefaultDecisionWhenNoMatch: model.DecisionAllow,
		MaxRulesPerRequest:         100,
	})
	require.NoError(t, err)

	return eval
}

// Rules the real pipeline evaluates against ruleEvaluationRequest: tierRule
// fails at runtime on a string "tier", denyRule matches its amount of 100.
func tierRule(id uuid.UUID) *model.Rule {
	return &model.Rule{ID: id, Name: "tier above one", Expression: `metadata["tier"] > 1`, Action: model.DecisionDeny, Status: model.RuleStatusActive}
}

func denyRule(id uuid.UUID) *model.Rule {
	return &model.Rule{ID: id, Name: "amount above fifty", Expression: `amount > 50`, Action: model.DecisionDeny, Status: model.RuleStatusActive}
}

func allowRule(id uuid.UUID) *model.Rule {
	return &model.Rule{ID: id, Name: "any amount", Expression: `amount > 0`, Action: model.DecisionAllow, Status: model.RuleStatusActive}
}

func TestValidate_RuleExpressionFailure_RoutesToReview(t *testing.T) {
	t.Parallel()

	ruleID := testutil.MustDeterministicUUID(510)

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "evaluation failure wrapped by the rule query",
			err: fmt.Errorf("failed to evaluate rules: %w", &query.RuleEvaluationError{
				RuleID: ruleID,
				Err:    fmt.Errorf("failed to evaluate expression: %w: no such overload", constant.ErrExpressionEvaluation),
			}),
		},
		{
			name: "type failure",
			err:  &query.RuleEvaluationError{RuleID: ruleID, Err: fmt.Errorf("%w: expected bool, got string", constant.ErrExpressionType)},
		},
		{
			name: "compile fallback failure",
			err:  &query.RuleEvaluationError{RuleID: ruleID, Err: fmt.Errorf("failed to compile expression: %w", constant.ErrExpressionSyntax)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			f := newRuleEvaluationFixture(ctrl)

			eval := mocks.NewMockRuleEvaluator(ctrl)
			eval.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, tt.err).Times(1)

			var record ruleEvaluationRecord
			f.expectReviewPersisted(&record)

			result, err := f.service(t, eval).Validate(context.Background(), ruleEvaluationRequest(nil))
			require.NoError(t, err)

			assertRuleEvaluationReview(t, result, &record, ruleID)
		})
	}
}

func TestValidate_RuleExpressionFailure_WithoutRuleID_RoutesToReview(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	f := newRuleEvaluationFixture(ctrl)

	eval := mocks.NewMockRuleEvaluator(ctrl)
	eval.EXPECT().Execute(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("%w: cost limit exceeded", constant.ErrExpressionEvaluation)).Times(1)

	var record ruleEvaluationRecord
	f.expectReviewPersisted(&record)

	result, err := f.service(t, eval).Validate(context.Background(), ruleEvaluationRequest(nil))
	require.NoError(t, err)

	assertRuleEvaluationReview(t, result, &record, []uuid.UUID{}...)
}

func TestValidate_RuleExpressionFailure_LimitExceededStillDenies(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	f := newRuleEvaluationFixture(ctrl)
	ruleID := testutil.MustDeterministicUUID(520)
	limitID := testutil.MustDeterministicUUID(521)

	eval := mocks.NewMockRuleEvaluator(ctrl)
	eval.EXPECT().Execute(gomock.Any(), gomock.Any()).
		Return(nil, &query.RuleEvaluationError{RuleID: ruleID, Err: constant.ErrExpressionEvaluation}).Times(1)

	f.txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(f.tx, nil).Times(1)
	f.limits.EXPECT().CheckLimits(gomock.Any(), f.tx, gomock.Any()).
		Return(&model.CheckLimitsOutput{Allowed: false, ExceededLimitIDs: []uuid.UUID{limitID}}, nil).Times(1)
	f.tx.EXPECT().Rollback().Return(nil).Times(1)

	var record ruleEvaluationRecord
	f.expectPersisted(&record)

	result, err := f.service(t, eval).Validate(context.Background(), ruleEvaluationRequest(nil))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, model.DecisionDeny, result.Response.Decision)
	assert.Equal(t, reasonLimitExceeded, result.Response.Reason)
	require.NotNil(t, record.audited)
	assert.Equal(t, model.DecisionDeny, record.audited.Decision)
	assert.Equal(t, reasonLimitExceeded, record.audited.Reason)
}

func TestValidate_RuleStepNonRuleFailure_StillPropagates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "rule cache not ready", err: fmt.Errorf("failed to load rules: %w", constant.ErrRuleCacheNotReady)},
		{name: "generic failure", err: errors.New("connection reset")},
		{
			name: "amount beyond CEL precision",
			err: fmt.Errorf("%w: failed to build activation: %w", constant.ErrExpressionEvaluation,
				fmt.Errorf("amount exceeds safe precision for CEL evaluation: %w", constant.ErrAmountExceedsPrecision)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			f := newRuleEvaluationFixture(ctrl)

			eval := mocks.NewMockRuleEvaluator(ctrl)
			eval.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, tt.err).Times(1)

			f.txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
			f.repo.EXPECT().Insert(gomock.Any(), gomock.Any()).Times(0)
			f.audit.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			result, err := f.service(t, eval).Validate(context.Background(), ruleEvaluationRequest(nil))
			require.Error(t, err)
			require.ErrorIs(t, err, tt.err)
			assert.Nil(t, result)
		})
	}
}

// TestValidate_RealCEL_TypeMismatchRoutesToReview drives the real rule pipeline:
// comparing a string metadata value with an int fails at runtime, and the
// validation must answer REVIEW attributed to that rule instead of failing.
func TestValidate_RealCEL_TypeMismatchRoutesToReview(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	f := newRuleEvaluationFixture(ctrl)
	ruleID := testutil.MustDeterministicUUID(530)

	var record ruleEvaluationRecord
	f.expectReviewPersisted(&record)

	result, err := f.service(t, realRuleQuery(t, ctrl, tierRule(ruleID))).
		Validate(context.Background(), ruleEvaluationRequest(map[string]any{"tier": "1"}))
	require.NoError(t, err)

	assertRuleEvaluationReview(t, result, &record, ruleID)
}

// A newer rule that fails at runtime must not hide an older DENY that matches.
func TestValidate_RealCEL_FailingRuleDoesNotHideOlderDeny(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	f := newRuleEvaluationFixture(ctrl)
	failingID := testutil.MustDeterministicUUID(540)
	denyID := testutil.MustDeterministicUUID(541)

	// DENY by rule persists outside any transaction and checks no limit.
	var record ruleEvaluationRecord
	f.expectPersisted(&record)

	result, err := f.service(t, realRuleQuery(t, ctrl, tierRule(failingID), denyRule(denyID))).
		Validate(context.Background(), ruleEvaluationRequest(map[string]any{"tier": "1"}))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, model.DecisionDeny, result.Response.Decision)
	assert.Equal(t, []uuid.UUID{denyID}, result.Response.MatchedRuleIDs)
	assert.NotEqual(t, reasonRuleEvaluationError, result.Response.Reason)
	require.NotNil(t, record.audited)
	assert.Equal(t, model.DecisionDeny, record.audited.Decision)
	assert.Equal(t, []uuid.UUID{denyID}, record.audited.MatchedRuleIDs)
}

// Without a matching DENY, every rule that failed is attributed, even when
// another rule matched.
func TestValidate_RealCEL_FailingRulesWithoutDenyRouteToReview(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	f := newRuleEvaluationFixture(ctrl)
	firstFailing := testutil.MustDeterministicUUID(550)
	allowID := testutil.MustDeterministicUUID(551)
	secondFailing := testutil.MustDeterministicUUID(552)

	var record ruleEvaluationRecord
	f.expectReviewPersisted(&record)

	result, err := f.service(t, realRuleQuery(t, ctrl, tierRule(firstFailing), allowRule(allowID), tierRule(secondFailing))).
		Validate(context.Background(), ruleEvaluationRequest(map[string]any{"tier": "1"}))
	require.NoError(t, err)

	assertRuleEvaluationReview(t, result, &record, firstFailing, secondFailing)
}

// The review Warn names the rules and the failure class; the CEL error text,
// which can quote request metadata, is not logged.
func TestValidate_RuleExpressionFailure_WarnOmitsRawError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	f := newRuleEvaluationFixture(ctrl)
	ruleID := testutil.MustDeterministicUUID(560)

	var record ruleEvaluationRecord
	f.expectReviewPersisted(&record)

	logger := testutil.NewMockLogger()
	ctx := libObservability.ContextWithLogger(context.Background(), logger)

	_, err := f.service(t, realRuleQuery(t, ctrl, tierRule(ruleID))).
		Validate(ctx, ruleEvaluationRequest(map[string]any{"tier": "secret-value"}))
	require.NoError(t, err)

	warn := findLogCall(t, logger, "Validation routed to review: rule evaluation failed")
	fields := testutil.FieldsToMap(warn.Fields)
	assert.Equal(t, constant.ErrExpressionEvaluation.Error(), fields["error.class"])
	assert.Equal(t, []string{ruleID.String()}, fields["rule_ids"])
	assert.NotContains(t, fields, "error")

	for _, call := range logger.Calls {
		for _, field := range call.Fields {
			assert.NotContains(t, fmt.Sprint(field.Value), "secret-value", "log %q leaks request metadata", call.Message)
		}
	}
}

func TestValidate_RuleEvaluationNilResult_ReturnsSentinel(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	f := newRuleEvaluationFixture(ctrl)

	eval := mocks.NewMockRuleEvaluator(ctrl)
	eval.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)

	result, err := f.service(t, eval).Validate(context.Background(), ruleEvaluationRequest(nil))
	require.ErrorIs(t, err, ErrNilRuleEvaluationResult)
	assert.Nil(t, result)
}

// Sequential: SetupTestTracing swaps the process-global tracer provider.
func TestValidate_AmountBeyondPrecision_KeepsSpanGreen(t *testing.T) {
	tracing := testutil.SetupTestTracing(t)
	ctrl := gomock.NewController(t)
	f := newRuleEvaluationFixture(ctrl)

	eval := mocks.NewMockRuleEvaluator(ctrl)
	eval.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("%w: failed to build activation: %w",
		constant.ErrExpressionEvaluation, constant.ErrAmountExceedsPrecision)).Times(1)

	_, err := f.service(t, eval).Validate(context.Background(), ruleEvaluationRequest(nil))
	require.ErrorIs(t, err, constant.ErrAmountExceedsPrecision)

	var found bool

	for _, span := range tracing.Exporter.GetSpans() {
		if span.Name == "service.validation.orchestrate" {
			found = true

			assert.Equal(t, codes.Unset, span.Status.Code, "an amount beyond CEL precision is a caller fault")
		}
	}

	assert.True(t, found, "orchestrate span not exported")
}

// findLogCall returns the one captured call logged with message.
func findLogCall(t *testing.T, logger *testutil.MockLogger, message string) testutil.LogCall {
	t.Helper()

	var matches []testutil.LogCall

	for _, call := range logger.Calls {
		if call.Message == message {
			matches = append(matches, call)
		}
	}

	require.Len(t, matches, 1, "expected exactly one %q log", message)

	return matches[0]
}

func reviewRule(id uuid.UUID) *model.Rule {
	return &model.Rule{ID: id, Name: "amount above fifty for review", Expression: `amount > 50`, Action: model.DecisionReview, Status: model.RuleStatusActive}
}

// assertRuleEvaluationAttribution asserts the evaluated ids and the loaded rule
// count of a REVIEW caused by a rule evaluation failure on the response, the
// persisted validation and the audit event, and that no failing rule is
// reported as evaluated.
func assertRuleEvaluationAttribution(t *testing.T, result *ValidateResult, record *ruleEvaluationRecord, evaluated, failing []uuid.UUID, totalLoaded int) {
	t.Helper()

	for name, got := range map[string]model.EvaluationResult{
		"response":  result.Response.EvaluationResult,
		"persisted": record.persisted.EvaluationResult,
		"audited":   *record.audited,
	} {
		assert.Equal(t, evaluated, got.EvaluatedRuleIDs, "%s evaluated rule ids", name)
		assert.Equal(t, totalLoaded, got.TotalRulesLoaded, "%s total rules loaded", name)
		assert.False(t, got.Truncated, "%s truncated", name)

		for _, id := range failing {
			assert.NotContains(t, got.EvaluatedRuleIDs, id, "%s reports failing rule %s as evaluated", name, id)
		}
	}
}

// A matched REVIEW rule and the rules that did evaluate survive a failing rule.
func TestValidate_RealCEL_MatchedReviewAndFailingRuleRouteToReview(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	f := newRuleEvaluationFixture(ctrl)
	reviewID := testutil.MustDeterministicUUID(570)
	failingID := testutil.MustDeterministicUUID(571)
	allowID := testutil.MustDeterministicUUID(572)

	var record ruleEvaluationRecord
	f.expectReviewPersisted(&record)

	result, err := f.service(t, realRuleQuery(t, ctrl, reviewRule(reviewID), tierRule(failingID), allowRule(allowID))).
		Validate(context.Background(), ruleEvaluationRequest(map[string]any{"tier": "1"}))
	require.NoError(t, err)

	assertRuleEvaluationReview(t, result, &record, reviewID, failingID)
	assertRuleEvaluationAttribution(t, result, &record, []uuid.UUID{reviewID, allowID}, []uuid.UUID{failingID}, 3)
}

// A matched ALLOW rule is evaluated but does not drive the REVIEW.
func TestValidate_RealCEL_MatchedAllowAndFailingRuleRouteToReview(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	f := newRuleEvaluationFixture(ctrl)
	allowID := testutil.MustDeterministicUUID(580)
	failingID := testutil.MustDeterministicUUID(581)

	var record ruleEvaluationRecord
	f.expectReviewPersisted(&record)

	result, err := f.service(t, realRuleQuery(t, ctrl, allowRule(allowID), tierRule(failingID))).
		Validate(context.Background(), ruleEvaluationRequest(map[string]any{"tier": "1"}))
	require.NoError(t, err)

	assertRuleEvaluationReview(t, result, &record, failingID)
	assertRuleEvaluationAttribution(t, result, &record, []uuid.UUID{allowID}, []uuid.UUID{failingID}, 2)
}

// assertNoSpanEventCarries fails when any exported span event attribute
// contains secret.
func assertNoSpanEventCarries(t *testing.T, tracing *testutil.TestTracer, secret string) {
	t.Helper()

	for _, span := range tracing.Exporter.GetSpans() {
		for _, event := range span.Events {
			for _, attr := range event.Attributes {
				assert.NotContains(t, attr.Value.Emit(), secret, "span %s event %q leaks the error text", span.Name, event.Name)
			}
		}
	}
}

// Sequential: SetupTestTracing swaps the process-global tracer provider.
func TestValidate_RuleExpressionFailure_SpanEventsOmitRawError(t *testing.T) {
	const secret = "secret-metadata-value"

	for _, tc := range []struct {
		name string
		err  error
	}{
		{
			name: "unevaluable rule",
			err: &query.RuleEvaluationError{
				RuleID: testutil.MustDeterministicUUID(590),
				Err:    fmt.Errorf("%w: no such overload: %s", constant.ErrExpressionEvaluation, secret),
			},
		},
		{
			name: "amount beyond CEL precision",
			err: fmt.Errorf("%w: failed to build activation: amount %s: %w",
				constant.ErrExpressionEvaluation, secret, constant.ErrAmountExceedsPrecision),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracing := testutil.SetupTestTracing(t)
			ctrl := gomock.NewController(t)
			f := newRuleEvaluationFixture(ctrl)

			eval := mocks.NewMockRuleEvaluator(ctrl)
			eval.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, tc.err).Times(1)

			if query.IsUnevaluableRule(tc.err) {
				var record ruleEvaluationRecord
				f.expectReviewPersisted(&record)
			}

			_, _ = f.service(t, eval).Validate(context.Background(), ruleEvaluationRequest(nil))

			assertNoSpanEventCarries(t, tracing, secret)
		})
	}
}
