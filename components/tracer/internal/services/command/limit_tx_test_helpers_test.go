// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// expectLimitStatusTxSuccess wires the full BeginTx → UpdateStatusWithTx →
// RecordLimitEventWithTx → Commit chain as a gomock.InOrder expectation for
// a successful status-change limit command. Use for activate/deactivate/
// draft/delete happy paths where the command wraps a status update + audit
// event in executeInTx.
//
// Argument matching strategy (chosen to keep the helper reusable across
// seeds and fixtures):
//   - ctx                → gomock.Any() (tests own ctx lifetime)
//   - db (BeginTx opts)  → nil (matches the production call: BeginTx(ctx, nil))
//   - db (inside *WithTx) → mockTx (exact instance; enforces atomicity invariant)
//   - limit UUID         → exact limitID (pinned by caller)
//   - target status      → exact targetStatus (pinned by caller)
//   - timestamp          → gomock.Any() (clock-driven, caller-independent)
//   - beforeState        → gomock.Any() (snapshot shape varies per command)
//   - afterState         → afterStateMatcher (gomock.Any() for normal status
//     changes; gomock.Nil() for delete, which intentionally omits "after")
//   - reason             → exact reason string (pinned by caller)
//   - clientIP           → gomock.Any() (propagated from request context)
func expectLimitStatusTxSuccess(
	t *testing.T,
	txBeginner *pgdbMocks.MockTxBeginner,
	mockTx *pgdbMocks.MockTx,
	repo *MockLimitRepository,
	audit *MockAuditWriter,
	limitID uuid.UUID,
	targetStatus model.LimitStatus,
	eventType model.AuditEventType,
	action model.AuditAction,
	reason string,
	afterStateMatcher gomock.Matcher,
) {
	t.Helper()
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		repo.EXPECT().
			UpdateStatusWithTx(gomock.Any(), mockTx, limitID, targetStatus, gomock.Any()).
			Return(nil),
		audit.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(),      // ctx
				mockTx,            // db (must be the exact tx instance)
				eventType,         // eventType
				action,            // action
				limitID,           // limitID
				gomock.Any(),      // beforeState
				afterStateMatcher, // afterState
				reason,            // reason
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)
}

// expectLimitStatusTxSuccessNoAudit wires the reduced BeginTx →
// UpdateStatusWithTx → Commit chain for the `auditWriter == nil`
// short-circuit branch inside executeInTx. Use in the *_Success_NilAuditWriter
// tests for activate/deactivate/draft/delete. Also pins `mockTx.Rollback`
// to Times(0) so the commit path cannot silently rollback.
//
// Note: BeginTx opts is pinned to nil to match executeInTx's BeginTx(ctx, nil)
// call exactly, keeping this helper as strict as the audited variant.
func expectLimitStatusTxSuccessNoAudit(
	t *testing.T,
	txBeginner *pgdbMocks.MockTxBeginner,
	mockTx *pgdbMocks.MockTx,
	repo *MockLimitRepository,
	limitID uuid.UUID,
	targetStatus model.LimitStatus,
) {
	t.Helper()
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		repo.EXPECT().
			UpdateStatusWithTx(gomock.Any(), mockTx, limitID, targetStatus, gomock.Any()).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)
	// Commit path must never trigger rollback.
	mockTx.EXPECT().Rollback().Times(0)
}

// expectLimitCreateTxSuccess wires the full BeginTx → CreateWithTx →
// RecordLimitEventWithTx → Commit chain as a gomock.InOrder expectation for
// a successful limit creation that persists both the limit insert and its
// audit event atomically.
//
// Argument matching strategy mirrors expectLimitStatusTxSuccess:
//   - ctx                → gomock.Any() (tests own ctx lifetime)
//   - BeginTx opts       → nil (matches the production call: BeginTx(ctx, nil))
//   - db (inside *WithTx) → mockTx (exact instance; enforces atomicity invariant)
//   - limit              → gomock.Any() (the in-memory limit has a generated UUID)
//   - eventType/action   → exact values (pinned by caller)
//   - limitID arg in audit → gomock.Any() (UUID generated inside Execute)
//   - beforeState        → gomock.Nil() (no "before" state for create)
//   - afterState         → gomock.Not(gomock.Nil()) (the new limit's snapshot)
//   - reason             → exact reason string (pinned by caller)
//   - clientIP           → gomock.Any() (propagated from request context)
//
// CreateWithTx returns nil (consistent with the existing Create signature on
// LimitRepository, which only returns error — the caller already holds the
// validated *model.Limit instance).
func expectLimitCreateTxSuccess(
	t *testing.T,
	txBeginner *pgdbMocks.MockTxBeginner,
	mockTx *pgdbMocks.MockTx,
	repo *MockLimitRepository,
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
			Return(nil),
		audit.EXPECT().
			RecordLimitEventWithTx(
				gomock.Any(),             // ctx
				mockTx,                   // db (must be the exact tx returned by BeginTx)
				eventType,                // eventType
				action,                   // action
				gomock.Any(),             // limitID (generated UUID)
				gomock.Nil(),             // beforeState (no "before" for create)
				gomock.Not(gomock.Nil()), // afterState
				reason,                   // reason
			).
			Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)
	mockTx.EXPECT().Rollback().Times(0)
}
