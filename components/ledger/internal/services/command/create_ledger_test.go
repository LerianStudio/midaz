// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libPointers "github.com/LerianStudio/lib-commons/v6/commons/pointers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestCreateLedger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()

	tests := []struct {
		name        string
		input       *mmodel.CreateLedgerInput
		mockSetup   func()
		expectedErr error
		expectedRes *mmodel.Ledger
	}{
		{
			name: "success - ledger created",
			input: &mmodel.CreateLedgerInput{
				Name: "Finance Ledger",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: libPointers.String("Ledger for financial transactions"),
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					FindByName(gomock.Any(), organizationID, "Finance Ledger").
					Return(true, nil).
					Times(1)

				mockLedgerRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Ledger{
						ID:             uuid.New().String(),
						OrganizationID: organizationID.String(),
						Name:           "Finance Ledger",
						Status: mmodel.Status{
							Code:        "ACTIVE",
							Description: libPointers.String("Ledger for financial transactions"),
						},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)
			},
			expectedErr: nil,
			expectedRes: &mmodel.Ledger{
				Name: "Finance Ledger",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
		},
		{
			name: "error - failed to find ledger by name",
			input: &mmodel.CreateLedgerInput{
				Name: "Finance Ledger",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					FindByName(gomock.Any(), organizationID, "Finance Ledger").
					Return(false, errors.New("database error")).
					Times(1)
			},
			expectedErr: errors.New("database error"),
			expectedRes: nil,
		},
		{
			name: "error - failed to create ledger",
			input: &mmodel.CreateLedgerInput{
				Name: "Finance Ledger",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					FindByName(gomock.Any(), organizationID, "Finance Ledger").
					Return(false, nil).
					Times(1)

				mockLedgerRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to insert ledger")).
					Times(1)
			},
			expectedErr: errors.New("failed to insert ledger"),
			expectedRes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			result, err := uc.CreateLedger(ctx, organizationID, tt.input)
			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedRes.Name, result.Name)
				assert.Equal(t, tt.expectedRes.Status.Code, result.Status.Code)
			}
		})
	}
}

// TestCreateLedger_PartialSettings drives CreateLedger from raw request bodies through the real
// decode seam (pkgHTTP.DecodeAndValidate, the single sequence both transports share) so each case
// reproduces byte-for-byte what a client sends. The table names no settings request type: bodies are
// JSON strings and expectations are built from mmodel.LedgerSettings, the persistence shape.
//
// The load-bearing assertion is the *mmodel.LedgerSettings captured from LedgerRepo.Create: it is
// the only way to tell "persisted the defaults" from "persisted nothing", a distinction the API
// response cannot express. Error cases register no Create expectation, so ctrl.Finish() proves
// validation rejected the request before any write.
//
// Only the error CODE is asserted, never a settings field path: a body carrying more than one
// invalid field reports whichever ValidateSettings reaches first while iterating a Go map, so a
// path assertion would be order-dependent and flaky.
func TestCreateLedger_PartialSettings(t *testing.T) {
	t.Parallel()

	const (
		fixedLedgerID = "3f0d4b6c-8a1e-4c2d-9b7f-5e6a0c1d2b34"

		// constant.ErrInvalidSettingsFieldValue: a settings value outside its allowed set.
		codeInvalidSettingsFieldValue = "0176"
	)

	fixedNow := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)

	// settingsWith builds the expected persisted value: canonical defaults with one field changed.
	settingsWith := func(mutate func(s *mmodel.LedgerSettings)) *mmodel.LedgerSettings {
		s := mmodel.DefaultLedgerSettings()
		mutate(&s)

		return &s
	}

	tests := []struct {
		name          string
		body          string                 // raw request body, exactly what the Console sends
		wantErrCode   string                 // "" = expect success
		wantPersisted *mmodel.LedgerSettings // nil = Create must receive Settings == nil
	}{
		{
			// Control that must never regress. A "settings": null body is byte-identical here
			// (both an absent key and an explicit null decode to a nil pointer), so it earns no row.
			name:          "settings omitted persists nothing",
			body:          `{"name":"Ledger Without Settings"}`,
			wantPersisted: nil,
		},
		{
			name:          "empty settings object persists nothing",
			body:          `{"name":"Ledger Empty Settings","settings":{}}`,
			wantPersisted: nil,
		},
		{
			name: "partial accounting persists defaults plus the one flag set",
			body: `{"name":"Ledger Partial Accounting","settings":{"accounting":{"validateAccountType":true}}}`,
			wantPersisted: settingsWith(func(s *mmodel.LedgerSettings) {
				s.Accounting.ValidateAccountType = true
			}),
		},
		{
			name:          "empty accounting group persists nothing",
			body:          `{"name":"Ledger Empty Accounting","settings":{"accounting":{}}}`,
			wantPersisted: nil,
		},
		{
			name: "partial tracer persists the given mode plus tracer defaults",
			body: `{"name":"Ledger Partial Tracer","settings":{"tracer":{"mode":"enforce"}}}`,
			wantPersisted: settingsWith(func(s *mmodel.LedgerSettings) {
				s.Tracer.Mode = mmodel.TracerModeEnforce
			}),
		},
		{
			name: "partial overrides persists defaults plus the one opt-in set",
			body: `{"name":"Ledger Partial Overrides","settings":{"overrides":{"allowFeeSkip":true}}}`,
			wantPersisted: settingsWith(func(s *mmodel.LedgerSettings) {
				s.Overrides.AllowFeeSkip = true
			}),
		},
		{
			name:          "explicit default value persists nothing",
			body:          `{"name":"Ledger Explicit Default","settings":{"accounting":{"validateAccountType":false}}}`,
			wantPersisted: nil,
		},
		{
			// Anti-over-correction guard: a misspelled enum member must stay rejected.
			name:        "misspelled tracer mode is rejected",
			body:        `{"name":"Ledger Bad Tracer Mode","settings":{"tracer":{"mode":"enfroce"}}}`,
			wantErrCode: codeInvalidSettingsFieldValue,
		},
		{
			// Sharpest guard in the table: an explicitly empty tracer.mode is a present key with an
			// out-of-enum value and must stay rejected, even though accepting the empty settings
			// object above means an absent mode is fine.
			name:        "explicitly empty tracer mode is rejected",
			body:        `{"name":"Ledger Empty Tracer Mode","settings":{"tracer":{"mode":""}}}`,
			wantErrCode: codeInvalidSettingsFieldValue,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockLedgerRepo := ledger.NewMockRepository(ctrl)

			// The decode path fills an absent "metadata" key with an empty non-nil map
			// (parseMetadata, RFC 7396 merge-patch semantics), so the metadata write always
			// runs on the success path and the repository cannot be left nil here.
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)

			uc := &UseCase{
				LedgerRepo:             mockLedgerRepo,
				OnboardingMetadataRepo: mockMetadataRepo,
			}

			ctx := context.Background()
			organizationID := uuid.New()

			payload := new(mmodel.CreateLedgerInput)

			_, err := pkgHTTP.DecodeAndValidate([]byte(tt.body), payload)
			require.NoError(t, err, "body must clear the decode boundary; the error under test must originate in the use case")

			mockLedgerRepo.EXPECT().
				FindByName(gomock.Any(), organizationID, gomock.Any()).
				Return(false, nil).
				Times(1)

			var gotSettings *mmodel.LedgerSettings

			if tt.wantErrCode == "" {
				mockLedgerRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, led *mmodel.Ledger) (*mmodel.Ledger, error) {
						gotSettings = led.Settings
						led.ID = fixedLedgerID
						led.CreatedAt, led.UpdatedAt = fixedNow, fixedNow

						return led, nil
					}).
					Times(1)

				mockMetadataRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			}

			result, err := uc.CreateLedger(ctx, organizationID, payload)

			if tt.wantErrCode != "" {
				require.Error(t, err)

				var vErr pkg.ValidationError

				require.True(t, errors.As(err, &vErr), "expected pkg.ValidationError, got %T", err)
				assert.Equal(t, tt.wantErrCode, vErr.Code, "expected error code %q, got %q", tt.wantErrCode, vErr.Code)
				assert.Nil(t, result)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantPersisted, gotSettings)
		})
	}
}
