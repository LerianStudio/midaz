// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	pgdb "tracer/internal/adapters/postgres/db"
	pgdbMocks "tracer/internal/adapters/postgres/db/mocks"
	"tracer/pkg/model"
)

// expectRuleUpdateTxSuccess wires the full BeginTx → UpdateWithTx →
// RecordRuleEventWithTx → Commit chain as a gomock.InOrder expectation for
// a successful rule lifecycle command that persists both the rule mutation
// and its audit event atomically. Use for activate/deactivate/draft happy
// paths.
//
// Argument matching strategy (chosen to keep the helper reusable across
// seeds and fixtures):
//   - ctx                → gomock.Any() (tests own ctx lifetime)
//   - BeginTx opts       → nil (matches the production call: BeginTx(ctx, nil))
//   - db (inside *WithTx) → mockTx (exact instance; enforces atomicity invariant)
//   - rule               → gomock.Any() (the in-memory rule is mutated pre-call)
//   - eventType/action   → exact values (pinned by caller)
//   - ruleID             → exact ruleID (pinned by caller)
//   - beforeState/after  → gomock.Any() (snapshot shape asserted elsewhere)
//   - reason             → exact reason string (pinned by caller)
//   - clientIP           → gomock.Any() (propagated from request context)
func expectRuleUpdateTxSuccess(
	t *testing.T,
	txBeginner *pgdbMocks.MockTxBeginner,
	mockTx *pgdbMocks.MockTx,
	repo *MockRuleRepository,
	audit *MockAuditWriter,
	ruleID uuid.UUID,
	eventType model.AuditEventType,
	action model.AuditAction,
	reason string,
) {
	t.Helper()
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		repo.EXPECT().
			UpdateWithTx(gomock.Any(), mockTx, gomock.Not(gomock.Nil())).
			Return(nil),
		audit.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),             // ctx
				mockTx,                   // db (must be the exact tx returned by BeginTx)
				eventType,                // eventType
				action,                   // action
				ruleID,                   // ruleID
				gomock.Not(gomock.Nil()), // beforeState (UPDATE always has non-nil before)
				gomock.Any(),             // afterState
				reason,                   // reason
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)
	// Commit path must never trigger rollback.
	mockTx.EXPECT().Rollback().Times(0)
}

// expectRuleUpdateTxSuccessNoAudit wires the reduced BeginTx → UpdateWithTx →
// Commit chain for the `auditWriter == nil` short-circuit branch inside the
// transactional callback. Use in *_Success_NilAuditWriter tests. Also pins
// mockTx.Rollback to Times(0) so the commit path cannot silently rollback.
func expectRuleUpdateTxSuccessNoAudit(
	t *testing.T,
	txBeginner *pgdbMocks.MockTxBeginner,
	mockTx *pgdbMocks.MockTx,
	repo *MockRuleRepository,
) {
	t.Helper()
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		repo.EXPECT().
			UpdateWithTx(gomock.Any(), mockTx, gomock.Any()).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)
	mockTx.EXPECT().Rollback().Times(0)
}

// expectRuleCreateTxSuccess wires the full BeginTx → CreateWithTx →
// RecordRuleEventWithTx → Commit chain as a gomock.InOrder expectation for
// a successful rule creation that persists both the rule insert and its
// audit event atomically.
//
// Argument matching strategy mirrors expectRuleUpdateTxSuccess:
//   - ctx                → gomock.Any() (tests own ctx lifetime)
//   - BeginTx opts       → nil (matches the production call: BeginTx(ctx, nil))
//   - db (inside *WithTx) → mockTx (exact instance; enforces atomicity invariant)
//   - rule               → gomock.Any() (the in-memory rule has a generated UUID)
//   - eventType/action   → exact values (pinned by caller)
//   - ruleID arg in audit → gomock.Any() (the rule's UUID is generated inside
//     Execute and is not known to the caller; the test inspects it via the
//     captured rule in CreateWithTx if needed)
//   - beforeState        → gomock.Nil() (no "before" state for create)
//   - afterState         → gomock.Not(gomock.Nil()) (the new rule's snapshot)
//   - reason             → exact reason string (pinned by caller)
//   - clientIP           → gomock.Any() (propagated from request context)
//
// The CreateWithTx call returns the rule unchanged via DoAndReturn so the
// caller's Execute observes the same instance back from the repository.
func expectRuleCreateTxSuccess(
	t *testing.T,
	txBeginner *pgdbMocks.MockTxBeginner,
	mockTx *pgdbMocks.MockTx,
	repo *MockRuleRepository,
	audit *MockAuditWriter,
	eventType model.AuditEventType,
	action model.AuditAction,
	reason string,
) {
	t.Helper()
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		repo.EXPECT().
			CreateWithTx(gomock.Any(), mockTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ pgdb.DB, r *model.Rule) (*model.Rule, error) {
				return r, nil
			}),
		audit.EXPECT().
			RecordRuleEventWithTx(
				gomock.Any(),             // ctx
				mockTx,                   // db (must be the exact tx returned by BeginTx)
				eventType,                // eventType
				action,                   // action
				gomock.Any(),             // ruleID (generated UUID)
				gomock.Nil(),             // beforeState (no "before" for create)
				gomock.Not(gomock.Nil()), // afterState
				reason,                   // reason
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)
	mockTx.EXPECT().Rollback().Times(0)
}
