// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestUpdateLedgerSettings_SpanClassContract is the T5 contract test for
// command.update_ledger_settings. UpdateSettingsAtomic is dual-class exactly like the two
// CreateLedger repository calls: an absent ledger comes back as a typed business error while a
// connection, transaction-begin or scan failure comes back untyped. A statically chosen span
// helper is therefore wrong for one of the two at the single call site.
//
// These rows pin recordCommandError's dispatch in both directions alongside the
// domain_operations_total result label: span class and metric class are decided by the same
// pkg.IsBusinessError predicate and must never disagree.
func TestUpdateLedgerSettings_SpanClassContract(t *testing.T) {
	t.Parallel()

	notFound := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityLedger)

	tests := []struct {
		name             string
		repoErr          error
		wantSpanStatus   codes.Code
		wantSpanEvent    string // event NAME: the message for a business event, "exception" for a technical one
		wantEventDetail  string // substring the event must carry; only technical events need it
		wantMetricResult string
	}{
		{
			// Business half: the ledger does not exist. A 404 must not flip the span red or it
			// pollutes the error-rate SLO with caller mistakes.
			name:             "ledger not found leaves the span unset",
			repoErr:          notFound,
			wantSpanStatus:   codes.Unset,
			wantSpanEvent:    "Failed to update ledger settings",
			wantMetricResult: "business_error",
		},
		{
			// Technical half at the same site: a Postgres outage must flip the span red.
			name:             "connection failure on atomic update sets the span to error",
			repoErr:          errors.New("connection reset by peer"),
			wantSpanStatus:   codes.Error,
			wantSpanEvent:    "exception",
			wantEventDetail:  "Failed to update ledger settings",
			wantMetricResult: "technical_error",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			reader, factory := newReaderFactory(t)

			mockLedgerRepo := ledger.NewMockRepository(ctrl)

			uc := &UseCase{
				LedgerRepo:     mockLedgerRepo,
				MetricsFactory: factory,
			}

			ctx, recorder := recordingContext()
			organizationID, ledgerID := uuid.New(), uuid.New()

			mockLedgerRepo.EXPECT().
				UpdateSettingsAtomic(gomock.Any(), organizationID, ledgerID, gomock.Any()).
				Return(nil, tt.repoErr).
				Times(1)

			_, err := uc.UpdateLedgerSettings(ctx, organizationID, ledgerID,
				map[string]any{"tracer": map[string]any{"mode": "enforce"}})
			require.Error(t, err)

			// Guard: the span helper and the metric label are both driven by this predicate,
			// so an inverted expectation here would be invisible in the assertions below.
			require.Equal(t, tt.wantMetricResult == "business_error", pkg.IsBusinessError(err),
				"case must exercise the error class it claims")

			span := findSpan(t, recorder, "command.update_ledger_settings")

			assert.Equal(t, tt.wantSpanStatus, span.Status().Code,
				"span status must follow the error class (T5)")
			event, ok := findEvent(span, tt.wantSpanEvent)
			require.True(t, ok, "the failure must be recorded as span event %q; got %v", tt.wantSpanEvent, span.Events())

			if tt.wantEventDetail != "" {
				assert.Contains(t, eventText(event), tt.wantEventDetail,
					"the recorded event must name the failing operation")
			}

			totals := collectDomainCounters(t, reader)
			assert.Equal(t, int64(1), totals["ledger/update_ledger_settings/"+tt.wantMetricResult],
				"domain_operations_total must classify the same error the span did")
		})
	}
}
