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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestDeleteAsset_SpanClassContract is the T5 contract test for command.delete_asset: a
// single call walks FOUR distinct dual-class repository sites in sequence (AssetRepo.Find,
// AccountRepo.ListExternalAccountsByAssetCode, AccountRepo.Delete per external account,
// AssetRepo.Delete), each now routed through recordCommandError (card 4413). A statically
// chosen span helper would be wrong for at least one direction at every one of them, so this
// pins the dispatch — business leaves the span UNSET with a named event, technical flips it to
// Error — independently at each site, proving the fix travels correctly across a multi-repo
// flow and not just a single-repository one.
func TestDeleteAsset_SpanClassContract(t *testing.T) {
	t.Parallel()

	assetFixture := &mmodel.Asset{ID: uuid.New().String(), Code: "USD"}

	tests := []struct {
		name             string
		mockSetup        func(*asset.MockRepository, *account.MockRepository)
		wantSpanStatus   codes.Code
		wantSpanEvent    string
		wantEventDetail  string
		wantMetricResult string
	}{
		{
			name: "asset lookup not-found leaves the span unset",
			mockSetup: func(assetRepo *asset.MockRepository, _ *account.MockRepository) {
				assetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			wantSpanStatus:   codes.Unset,
			wantSpanEvent:    "Failed to get asset on repo by id",
			wantMetricResult: "business_error",
		},
		{
			name: "asset lookup failure sets the span to error",
			mockSetup: func(assetRepo *asset.MockRepository, _ *account.MockRepository) {
				assetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("connection reset by peer")).
					Times(1)
			},
			wantSpanStatus:   codes.Error,
			wantSpanEvent:    "exception",
			wantEventDetail:  "Failed to get asset on repo by id",
			wantMetricResult: "technical_error",
		},
		{
			name: "external accounts listing failure sets the span to error",
			mockSetup: func(assetRepo *asset.MockRepository, accountRepo *account.MockRepository) {
				assetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(assetFixture, nil).
					Times(1)
				accountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), assetFixture.Code).
					Return(nil, errors.New("connection reset by peer")).
					Times(1)
			},
			wantSpanStatus:   codes.Error,
			wantSpanEvent:    "exception",
			wantEventDetail:  "Failed to retrieve asset external accounts",
			wantMetricResult: "technical_error",
		},
		{
			name: "external account delete failure sets the span to error",
			mockSetup: func(assetRepo *asset.MockRepository, accountRepo *account.MockRepository) {
				extAccount := &mmodel.Account{ID: uuid.New().String()}

				assetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(assetFixture, nil).
					Times(1)
				accountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), assetFixture.Code).
					Return([]*mmodel.Account{extAccount}, nil).
					Times(1)
				accountRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), uuid.MustParse(extAccount.ID)).
					Return(errors.New("connection reset by peer")).
					Times(1)
			},
			wantSpanStatus:   codes.Error,
			wantSpanEvent:    "exception",
			wantEventDetail:  "Failed to delete asset external account",
			wantMetricResult: "technical_error",
		},
		{
			name: "asset delete not-found leaves the span unset",
			mockSetup: func(assetRepo *asset.MockRepository, accountRepo *account.MockRepository) {
				assetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(assetFixture, nil).
					Times(1)
				accountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), assetFixture.Code).
					Return(nil, nil).
					Times(1)
				assetRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(services.ErrDatabaseItemNotFound).
					Times(1)
			},
			wantSpanStatus:   codes.Unset,
			wantSpanEvent:    "Failed to delete asset on repo by id",
			wantMetricResult: "business_error",
		},
		{
			name: "asset delete failure sets the span to error",
			mockSetup: func(assetRepo *asset.MockRepository, accountRepo *account.MockRepository) {
				assetRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(assetFixture, nil).
					Times(1)
				accountRepo.EXPECT().
					ListExternalAccountsByAssetCode(gomock.Any(), gomock.Any(), gomock.Any(), assetFixture.Code).
					Return(nil, nil).
					Times(1)
				assetRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("connection reset by peer")).
					Times(1)
			},
			wantSpanStatus:   codes.Error,
			wantSpanEvent:    "exception",
			wantEventDetail:  "Failed to delete asset on repo by id",
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

			mockAssetRepo := asset.NewMockRepository(ctrl)
			mockAccountRepo := account.NewMockRepository(ctrl)

			uc := &UseCase{
				AssetRepo:      mockAssetRepo,
				AccountRepo:    mockAccountRepo,
				MetricsFactory: factory,
			}

			ctx, recorder := recordingContext()

			tt.mockSetup(mockAssetRepo, mockAccountRepo)

			err := uc.DeleteAssetByID(ctx, uuid.New(), uuid.New(), uuid.New())
			require.Error(t, err)

			require.Equal(t, tt.wantMetricResult == "business_error", pkg.IsBusinessError(err),
				"case must exercise the error class it claims")

			span := findSpan(t, recorder, "command.delete_asset_by_id")

			assert.Equal(t, tt.wantSpanStatus, span.Status().Code,
				"span status must follow the error class (T5)")

			event, ok := findEvent(span, tt.wantSpanEvent)
			require.True(t, ok, "the failure must be recorded as span event %q; got %v", tt.wantSpanEvent, span.Events())

			if tt.wantEventDetail != "" {
				assert.Contains(t, eventText(event), tt.wantEventDetail,
					"the recorded event must name the failing operation")
			}

			totals := collectDomainCounters(t, reader)
			assert.Equal(t, int64(1), totals["ledger/delete_asset/"+tt.wantMetricResult],
				"domain_operations_total must classify the same error the span did")
		})
	}
}
