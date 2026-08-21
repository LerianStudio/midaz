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
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
)

// validationErrorCode asserts err carries a pkg.ValidationError and returns its code.
// errors.As against the wrong concrete type terminates false, so the assertion has to be
// fatal here: used as a bare guard it would let a reclassified error skip the code check
// while the row stayed green.
func validationErrorCode(t *testing.T, err error) string {
	t.Helper()

	var vErr pkg.ValidationError
	require.True(t, errors.As(err, &vErr), "expected pkg.ValidationError, got %T", err)

	return vErr.Code
}

// conflictErrorCode is the same for pkg.EntityConflictError, a distinct struct that no
// ValidationError target can ever match.
func conflictErrorCode(t *testing.T, err error) string {
	t.Helper()

	var cErr pkg.EntityConflictError
	require.True(t, errors.As(err, &cErr), "expected pkg.EntityConflictError, got %T", err)

	return cErr.Code
}

// TestCreateLedger_SpanClassContract is the T5 contract test for command.create_ledger: the
// span-error helper must follow the error CLASS, so a business/4xx failure leaves the span
// status UNSET with a business event recorded while a technical/5xx failure flips it to Error.
//
// Both classes reach CreateLedger through the SAME two call sites — FindByName maps a name
// collision to a typed business error and passes connection failures through, Create does the
// same for constraint violations — so a statically chosen helper is wrong for one of them at
// every site. This test pins recordCommandError's dispatch in both directions, and the
// domain_operations_total result label alongside it: span class and metric class are decided by
// the same pkg.IsBusinessError predicate and must never disagree.
func TestCreateLedger_SpanClassContract(t *testing.T) {
	t.Parallel()

	nameConflict := pkg.ValidateBusinessError(constant.ErrLedgerNameConflict, constant.EntityLedger, "Conflicting Ledger")

	tests := []struct {
		name             string
		input            *mmodel.CreateLedgerInput
		mockSetup        func(*ledger.MockRepository, uuid.UUID)
		wantErrCode      string                         // "" = not a typed business error
		wantErrCodeFrom  func(*testing.T, error) string // extracts wantErrCode, asserting the concrete typed error first
		wantSpanStatus   codes.Code
		wantSpanEvent    string // event NAME: the message for a business event, "exception" for a technical one
		wantEventDetail  string // substring the event must carry; only technical events need it
		wantMetricResult string
		wantBoolAttrs    map[string]bool // span attributes the row requires, exact value
	}{
		{
			// The 4390 path: a settings value outside its allowed set is a caller error.
			name: "settings validation failure leaves the span unset",
			input: &mmodel.CreateLedgerInput{
				Name: "Ledger Bad Settings",
				Settings: &mmodel.LedgerSettingsInput{
					Tracer: &mmodel.TracerSettingsInput{Mode: testutils.Ptr("enfroce")},
				},
			},
			mockSetup: func(repo *ledger.MockRepository, orgID uuid.UUID) {
				repo.EXPECT().
					FindByName(gomock.Any(), orgID, gomock.Any()).
					Return(false, nil).
					Times(1)
			},
			wantErrCode:      constant.ErrInvalidSettingsFieldValue.Error(),
			wantErrCodeFrom:  validationErrorCode,
			wantSpanStatus:   codes.Unset,
			wantSpanEvent:    "Settings validation failed",
			wantMetricResult: "business_error",
			// The presence flags must be set BEFORE ValidateSettings runs, or the error's
			// field path is indistinguishable from one naming a group never sent.
			wantBoolAttrs: map[string]bool{
				"app.request.settings.has_accounting": false,
				"app.request.settings.has_tracer":     true,
				"app.request.settings.has_overrides":  false,
			},
		},
		{
			// Business half of recordCommandError at the FindByName site.
			name:  "duplicate name conflict leaves the span unset",
			input: &mmodel.CreateLedgerInput{Name: "Conflicting Ledger"},
			mockSetup: func(repo *ledger.MockRepository, orgID uuid.UUID) {
				repo.EXPECT().
					FindByName(gomock.Any(), orgID, gomock.Any()).
					Return(true, nameConflict).
					Times(1)
			},
			wantErrCode:      nameConflict.(pkg.EntityConflictError).Code,
			wantErrCodeFrom:  conflictErrorCode,
			wantSpanStatus:   codes.Unset,
			wantSpanEvent:    "Failed to find ledger by name",
			wantMetricResult: "business_error",
		},
		{
			// Technical half at the same site: a connection failure must flip the span red.
			name:  "connection failure on lookup sets the span to error",
			input: &mmodel.CreateLedgerInput{Name: "Ledger Lookup Outage"},
			mockSetup: func(repo *ledger.MockRepository, orgID uuid.UUID) {
				repo.EXPECT().
					FindByName(gomock.Any(), orgID, gomock.Any()).
					Return(false, errors.New("connection reset by peer")).
					Times(1)
			},
			wantSpanStatus:   codes.Error,
			wantSpanEvent:    "exception",
			wantEventDetail:  "Failed to find ledger by name",
			wantMetricResult: "technical_error",
		},
		{
			// Technical half at the Create site.
			name:  "write failure on create sets the span to error",
			input: &mmodel.CreateLedgerInput{Name: "Ledger Write Outage"},
			mockSetup: func(repo *ledger.MockRepository, orgID uuid.UUID) {
				repo.EXPECT().
					FindByName(gomock.Any(), orgID, gomock.Any()).
					Return(false, nil).
					Times(1)
				repo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("connection refused")).
					Times(1)
			},
			wantSpanStatus:   codes.Error,
			wantSpanEvent:    "exception",
			wantEventDetail:  "Failed to create ledger",
			wantMetricResult: "technical_error",
		},
		{
			// Business half at the Create site: an unknown organization surfaces as a
			// constraint violation mapped to a typed business error, and must not flip the
			// span red just because it arrived from the database.
			name:  "constraint violation on create leaves the span unset",
			input: &mmodel.CreateLedgerInput{Name: "Ledger Unknown Org"},
			mockSetup: func(repo *ledger.MockRepository, orgID uuid.UUID) {
				repo.EXPECT().
					FindByName(gomock.Any(), orgID, gomock.Any()).
					Return(false, nil).
					Times(1)
				repo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, constant.EntityLedger)).
					Times(1)
			},
			wantSpanStatus:   codes.Unset,
			wantSpanEvent:    "Failed to create ledger",
			wantMetricResult: "business_error",
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
			organizationID := uuid.New()

			tt.mockSetup(mockLedgerRepo, organizationID)

			_, err := uc.CreateLedger(ctx, organizationID, tt.input)
			require.Error(t, err)

			if tt.wantErrCode != "" {
				require.NotNil(t, tt.wantErrCodeFrom,
					"a row asserting an error code must name the typed error that carries it")
				assert.Equal(t, tt.wantErrCode, tt.wantErrCodeFrom(t, err))
			}

			// Guard: the span helper and the metric label are both driven by this predicate,
			// so an inverted expectation here would be invisible in the assertions below.
			require.Equal(t, tt.wantMetricResult == "business_error", pkg.IsBusinessError(err),
				"case must exercise the error class it claims")

			span := findSpan(t, recorder, "command.create_ledger")

			assert.Equal(t, tt.wantSpanStatus, span.Status().Code,
				"span status must follow the error class (T5)")
			event, ok := findEvent(span, tt.wantSpanEvent)
			require.True(t, ok, "the failure must be recorded as span event %q; got %v", tt.wantSpanEvent, span.Events())

			if tt.wantEventDetail != "" {
				assert.Contains(t, eventText(event), tt.wantEventDetail,
					"the recorded event must name the failing operation")
			}

			if tt.wantBoolAttrs != nil {
				got := boolAttrs(span)
				for key, want := range tt.wantBoolAttrs {
					actual, ok := got[key]
					require.True(t, ok, "span must carry boolean attribute %q; got %v", key, got)
					assert.Equal(t, want, actual, "attribute %q", key)
				}
			}

			totals := collectDomainCounters(t, reader)
			assert.Equal(t, int64(1), totals["ledger/create_ledger/"+tt.wantMetricResult],
				"domain_operations_total must classify the same error the span did")
		})
	}
}
