// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// businessErrorCode returns the numeric code a business error renders. Every
// business error type in pkg formats itself as "<code> - <message>" whenever it
// carries a code, so the prefix is the code whichever type answered.
func businessErrorCode(err error) string {
	code, _, found := strings.Cut(err.Error(), " - ")
	if !found {
		return ""
	}

	return code
}

// refusedBy names the layer expected to turn a payload down.
type refusedBy int

const (
	// refusedByPayloadValidation is the check the handler runs on the request
	// body before the package service is called at all.
	refusedByPayloadValidation refusedBy = iota
	// refusedByService is the check the package service runs while building the
	// update, which is where the crash used to be.
	refusedByService
)

// TestUpdatePackageByID_FeeWithoutCalculationIsRefused pins the answer a PATCH
// gets when it adds a fee whose calculation values are absent or incomplete, and
// which layer produces it. Nothing here may be written: when the service is
// reached at all, its repository mock carries no expectation for Update, so any
// write fails the case.
//
// The first case is the reported defect. A fee entry carrying only a label
// passes the request-body validation untouched, then used to dereference a
// calculation model that was never sent, crashing the request (HTTP 500) instead
// of telling the caller what is missing.
func TestUpdatePackageByID_FeeWithoutCalculationIsRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		layer    refusedBy
		wantCode string
	}{
		{
			name:     "fee carries only a label, no calculation model at all",
			body:     `{"fees":{"adminFee":{"feeLabel":"Taxa Administrativa"}}}`,
			layer:    refusedByService,
			wantCode: constant.ErrCalculationRequired.Error(),
		},
		{
			name:     "fee carries a label and an empty calculation model object",
			body:     `{"fees":{"adminFee":{"feeLabel":"Taxa Administrativa","calculationModel":{}}}}`,
			layer:    refusedByService,
			wantCode: constant.ErrFeeFieldsRequired.Error(),
		},
		{
			name:     "calculation model carries no application rule",
			body:     `{"fees":{"adminFee":{"feeLabel":"Taxa Administrativa","referenceAmount":"originalAmount","priority":1,"creditAccount":"conta_taxas","isDeductibleFrom":false,"calculationModel":{"calculations":[{"type":"flat","value":"10.00"}]}}}}`,
			layer:    refusedByService,
			wantCode: constant.ErrFeeFieldsRequired.Error(),
		},
		{
			name:     "calculation model carries no calculations",
			body:     `{"fees":{"adminFee":{"feeLabel":"Taxa Administrativa","referenceAmount":"originalAmount","priority":1,"creditAccount":"conta_taxas","isDeductibleFrom":false,"calculationModel":{"applicationRule":"flatFee","calculations":[]}}}}`,
			layer:    refusedByPayloadValidation,
			wantCode: constant.ErrAppRuleFlatFeeAndPercentual.Error(),
		},
		{
			name:     "fee carries no credit account",
			body:     `{"fees":{"adminFee":{"feeLabel":"Taxa Administrativa","referenceAmount":"originalAmount","priority":1,"isDeductibleFrom":false,"calculationModel":{"applicationRule":"flatFee","calculations":[{"type":"flat","value":"10.00"}]}}}}`,
			layer:    refusedByService,
			wantCode: constant.ErrFeeFieldsRequired.Error(),
		},
		{
			name:     "calculation carries no type",
			body:     `{"fees":{"adminFee":{"feeLabel":"Taxa Administrativa","referenceAmount":"originalAmount","priority":1,"creditAccount":"conta_taxas","isDeductibleFrom":false,"calculationModel":{"applicationRule":"flatFee","calculations":[{"value":"10.00"}]}}}}`,
			layer:    refusedByPayloadValidation,
			wantCode: constant.ErrCalculationTypeFlatFee.Error(),
		},
		{
			name:     "calculation carries no value",
			body:     `{"fees":{"adminFee":{"feeLabel":"Taxa Administrativa","referenceAmount":"originalAmount","priority":1,"creditAccount":"conta_taxas","isDeductibleFrom":false,"calculationModel":{"applicationRule":"flatFee","calculations":[{"type":"flat"}]}}}}`,
			layer:    refusedByPayloadValidation,
			wantCode: constant.ErrConvertToDecimal.Error(),
		},
		{
			// Already refused before the guard above existed, and its answer
			// must not move: an entry with no label is turned down by the label
			// check, never reaching the calculation model.
			name:     "fee entry is entirely empty",
			body:     `{"fees":{"adminFee":{}}}`,
			layer:    refusedByService,
			wantCode: constant.ErrFeeFieldsRequired.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var input model.UpdatePackageInput
			require.NoError(t, json.Unmarshal([]byte(tt.body), &input))

			// The handler validates the request body before the service sees
			// it, so the refusal is whichever of the two comes first.
			err := input.ValidateFees()

			if tt.layer == refusedByService {
				require.NoError(t, err, "request-body validation was expected to let this through")

				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				mockPackageRepo := pack.NewMockRepository(ctrl)
				mockResolver := feeshared.NewMockMidazResolver(ctrl)

				mockPackageRepo.EXPECT().
					FindFeesAndAmountDataByPackageID(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&model.AmountData{
						MinAmount: decimal.NewFromInt(100),
						MaxAmount: decimal.NewFromInt(1000),
						Fees:      map[string]model.Fee{},
						LedgerID:  uuid.New(),
					}, nil)

				svc := &UseCase{packageRepo: mockPackageRepo, resolver: mockResolver}

				err = svc.UpdatePackageByID(context.Background(), uuid.New(), uuid.New(), uuid.Nil, &input)
			}

			require.Error(t, err, "the package must be refused, not written")
			assert.Equal(t, tt.wantCode, businessErrorCode(err), "refusal code, got %q", err.Error())
		})
	}
}
