// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"go.uber.org/mock/gomock"

	libStreaming "github.com/LerianStudio/lib-streaming"
	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// TestCommandStructs_StreamingFieldSettable asserts that every rule and limit
// command struct carries an exported Streaming field of type
// libStreaming.Emitter that accepts a mock emitter. The field is left
// zero-valued (nil) by the New* constructors — nil disables emission — and is
// set post-construction by the bootstrap wiring, mirroring how fees inject the
// emitter onto the ledger UseCase.
func TestCommandStructs_StreamingFieldSettable(t *testing.T) {
	t.Parallel()

	mock := pkgStreaming.NewMockEmitter()

	// Compile-time + runtime proof that each struct exposes a settable
	// Streaming field of the Emitter interface type. If any struct lacks the
	// field (or names/types it differently) this file fails to compile.
	fields := []libStreaming.Emitter{
		CreateRuleCommand{Streaming: mock}.Streaming,
		UpdateRuleCommand{Streaming: mock}.Streaming,
		ActivateRuleService{Streaming: mock}.Streaming,
		DeactivateRuleService{Streaming: mock}.Streaming,
		DraftRuleService{Streaming: mock}.Streaming,
		DeleteRuleService{Streaming: mock}.Streaming,
		CreateLimitCommand{Streaming: mock}.Streaming,
		UpdateLimitCommand{Streaming: mock}.Streaming,
		ActivateLimitCommand{Streaming: mock}.Streaming,
		DeactivateLimitCommand{Streaming: mock}.Streaming,
		DraftLimitCommand{Streaming: mock}.Streaming,
		DeleteLimitCommand{Streaming: mock}.Streaming,
	}

	if len(fields) != 12 {
		t.Fatalf("expected 12 command structs to carry a Streaming field, got %d", len(fields))
	}

	for i, f := range fields {
		if f != mock {
			t.Errorf("command struct #%d: Streaming field not settable to the mock emitter", i)
		}
	}
}

// TestNewCommandConstructors_LeaveStreamingNil asserts the New* constructors
// leave Streaming zero-valued (nil), so existing command tests keep passing
// and nil-emitter call sites treat the field as "streaming disabled".
func TestNewCommandConstructors_LeaveStreamingNil(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	ruleRepo := NewMockRuleRepository(ctrl)
	cel := NewMockExpressionCompiler(ctrl)
	limitRepo := NewMockLimitRepository(ctrl)
	audit := NewMockAuditWriter(ctrl)
	tx := pgdbMocks.NewMockTxBeginner(ctrl)
	clk := testutil.NewDefaultMockClock()

	createRule, err := NewCreateRuleCommand(ruleRepo, cel, clk, audit, tx)
	if err != nil {
		t.Fatalf("NewCreateRuleCommand returned error: %v", err)
	}

	if createRule.Streaming != nil {
		t.Errorf("NewCreateRuleCommand: expected Streaming nil by default, got non-nil")
	}

	createLimit, err := NewCreateLimitCommand(limitRepo, clk, audit, tx)
	if err != nil {
		t.Fatalf("NewCreateLimitCommand returned error: %v", err)
	}

	if createLimit.Streaming != nil {
		t.Errorf("NewCreateLimitCommand: expected Streaming nil by default, got non-nil")
	}
}
