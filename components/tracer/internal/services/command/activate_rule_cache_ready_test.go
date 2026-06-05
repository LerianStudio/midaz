// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestActivateRule_MarksCacheReadyAfterUpsert verifies the backstop for the
// multi-tenant cache-readiness gap. In MT mode, /v1/validations is gated on
// RuleCache.IsReady(ctx) and returns TRC-0281 until it's true. Only
// cache.WarmUp and the sync worker call MarkReady today; the worker may not
// have completed its first poll when a user activates a rule for the first
// time on a fresh tenant.
//
// ActivateRuleService.Execute MUST call RuleCacheWriter.MarkReady(ctx) after a
// successful UpsertRule so the first activation on a freshly spawned tenant
// immediately opens the readiness gate.
func TestActivateRule_MarksCacheReadyAfterUpsert(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-b")
	ruleID := testutil.MustDeterministicUUID(42)

	inputRule := &model.Rule{
		ID:         ruleID,
		Name:       "cache-ready-backstop",
		Status:     model.RuleStatusDraft,
		Expression: "amount > 1000",
	}

	mockRepo := NewMockRuleRepository(ctrl)
	mockExprCompiler := NewMockExpressionCompiler(ctrl)
	mockCache := NewMockRuleCacheWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockRepo.EXPECT().
		GetByID(gomock.Any(), ruleID).
		Return(inputRule, nil)
	mockExprCompiler.EXPECT().
		Compile(gomock.Any(), inputRule.Expression).
		Return("compiled-program", nil)

	// Transactional chain mirrors the production path: BeginTx -> UpdateWithTx -> Commit.
	// We pass nil for the AuditWriter so RecordRuleEventWithTx is never invoked.
	txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil)
	mockRepo.EXPECT().
		UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).
		Return(nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Rollback().Times(0)

	// The load-bearing assertion: after a successful Commit, the cache writer
	// MUST receive both UpsertRule AND MarkReady — in that order. Using
	// InOrder guards against a future refactor that accidentally calls
	// MarkReady before UpsertRule (which would leave the bucket ready but
	// empty).
	upsertCall := mockCache.EXPECT().
		UpsertRule(gomock.Any(), gomock.Any(), "compiled-program").
		Times(1)
	mockCache.EXPECT().
		MarkReady(gomock.Any()).
		Times(1).
		After(upsertCall)

	service, err := NewActivateRuleService(
		mockRepo, mockExprCompiler, testutil.NewDefaultMockClock(), nil, mockCache, txBeginner,
	)
	require.NoError(t, err)

	_, err = service.Execute(ctx, ruleID)
	require.NoError(t, err)
}
