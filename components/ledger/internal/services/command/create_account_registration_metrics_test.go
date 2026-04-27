// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/LerianStudio/lib-commons/v4/commons/opentelemetry/metrics"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// TestEmitCounter_NilFactory_NoOp confirms the early-return guard when the metric
// factory is unavailable. This is the path taken by every code site that calls into
// emit* without an OTel pipeline configured (e.g., unit tests, dry-run bootstraps).
func TestEmitCounter_NilFactory_NoOp(t *testing.T) {
	t.Parallel()

	// nilLogger is acceptable: emitCounter must not log when factory is nil.
	emitCounter(context.Background(), nil, nil, utils.AccountRegistrationStartedTotal, nil)
	// No assertion needed: the test passes if no panic occurs.
}

// TestEmitCounter_NopFactory_NoError exercises the success branch end-to-end. The
// no-op meter accepts every call without error, so emitCounter must traverse the
// factory.Counter → counter.WithLabels → AddOne path and return cleanly.
func TestEmitCounter_NopFactory_NoError(t *testing.T) {
	t.Parallel()

	factory := metrics.NewNopFactory()
	logger := libLog.NewNop()

	emitCounter(context.Background(), logger, factory, utils.AccountRegistrationStartedTotal, map[string]string{
		"organization_id": uuid.New().String(),
		"ledger_id":       uuid.New().String(),
	})
}

// TestEmitSagaStartedCompletedFailed_AllPathsExecute walks each of the public emit*
// helpers exposed on the UseCase. Each one must format its labels correctly and reach
// the underlying counter. With a no-op factory the calls simply succeed; the value of
// the test is that all three paths execute, not the metric values.
func TestEmitSagaStartedCompletedFailed_AllPathsExecute(t *testing.T) {
	t.Parallel()

	factory := metrics.NewNopFactory()
	logger := libLog.NewNop()

	uc := &UseCase{}
	orgID := uuid.New()
	ledgerID := uuid.New()

	uc.emitSagaStarted(context.Background(), logger, factory, orgID, ledgerID)
	uc.emitSagaCompleted(context.Background(), logger, factory, orgID, ledgerID)
	uc.emitSagaFailed(context.Background(), logger, factory, orgID, ledgerID, "TEST_REASON")
}

// TestEmitSagaCompleted_NilFactory_NoOp confirms the saga-completion helper inherits
// the no-op-on-nil contract from emitCounter. A failed observability pipeline must
// never block the durable saga from finalising.
func TestEmitSagaCompleted_NilFactory_NoOp(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}
	uc.emitSagaCompleted(context.Background(), nil, nil, uuid.New(), uuid.New())
}
