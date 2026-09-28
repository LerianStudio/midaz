// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ambiguityLogger records every structured line so the test can assert on what
// an operator would actually read.
type ambiguityLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *ambiguityLogger) Log(_ context.Context, level int, msg string, fields ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.lines = append(l.lines, fmt.Sprintf("level=%d msg=%q fields=%v", level, msg, fields))
}

func (l *ambiguityLogger) With(_ ...any) libLog.Logger      { return l }
func (l *ambiguityLogger) WithGroup(_ string) libLog.Logger { return l }
func (l *ambiguityLogger) Enabled(_ int) bool               { return true }
func (l *ambiguityLogger) Sync(_ context.Context) error     { return nil }

// rendered joins the recorded lines into one searchable blob.
func (l *ambiguityLogger) rendered() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return strings.Join(l.lines, "\n")
}

// TestCalculateFee_AmbiguousPackagesAreNamed pins what an operator is given when
// a payment is refused because two stored fee packages tie on scope.
//
// The refusal is newly reachable: before route selection was repaired both
// packages were dropped and the payment posted free of charge, so a ledger whose
// packages overlap now answers 400 on payments that used to go through. The only
// fix is to re-scope one of the two packages, and the only way to find them
// without the ids is to re-derive the selection by hand against every package on
// the ledger. So the ids go in three places at once: the message the client
// reads, the span event, and a warn line in the service log.
//
// Both selection paths are covered. The sole-package path cannot tie on its own,
// so the two-package ledger drives the multi-package path; the sole-package path
// is pinned on the shape it can reach, which is no refusal at all.
func TestCalculateFee_AmbiguousPackagesAreNamed(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPackRepo := pack.NewMockRepository(ctrl)
	orgID := uuid.New()
	ledgerID := uuid.New()
	routeID := uuid.New().String()

	firstID := uuid.New()
	secondID := uuid.New()

	feeSvc := &UseCase{packageRepo: mockPackRepo}

	mockPackRepo.EXPECT().
		FindByOrganizationIDAndLedgerID(gomock.Any(), orgID, ledgerID).
		Return([]*pack.Package{
			routeScopedFlatPackage(firstID, routeID),
			routeScopedFlatPackage(secondID, routeID),
		}, nil)

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	logger := &ambiguityLogger{}

	ctx := libObservability.ContextWithLogger(
		libObservability.ContextWithTracer(context.Background(), provider.Tracer("fees_test")),
		logger,
	)

	feeInput := routedFeeInput(ledgerID, routeID)

	err := feeSvc.CalculateFee(ctx, feeInput, orgID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrFilterPackage.Error(),
		"the payment must still be refused with the package-filtering code")

	// The client-facing message names both packages whose scopes collide.
	assert.Contains(t, err.Error(), firstID.String(),
		"the refusal must name the first package that tied")
	assert.Contains(t, err.Error(), secondID.String(),
		"the refusal must name the second package that tied")

	// The operator log names them too, at a level that is actually collected.
	logged := logger.rendered()
	assert.Contains(t, logged, firstID.String(),
		"the service log must name the first package that tied")
	assert.Contains(t, logged, secondID.String(),
		"the service log must name the second package that tied")

	// And so does the span event, which is where a trace-first operator looks.
	var events []string

	for _, s := range recorder.Ended() {
		if s.Name() != "service.calculate_fee" {
			continue
		}

		for _, e := range s.Events() {
			for _, attr := range e.Attributes {
				events = append(events, e.Name+" "+attr.Value.AsString())
			}
		}
	}

	joined := strings.Join(events, "\n")
	assert.Contains(t, joined, firstID.String(),
		"the span event must name the first package that tied")
	assert.Contains(t, joined, secondID.String(),
		"the span event must name the second package that tied")
}
