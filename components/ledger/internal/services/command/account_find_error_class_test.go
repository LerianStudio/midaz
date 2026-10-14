// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

const accountFindErrorMessage = "Failed to find account by id"

// accountFindErrorCase drives the account lookup that guards update and delete.
type accountFindErrorCase struct {
	name       string
	findErr    error
	business   bool
	wantStatus codes.Code
}

func accountFindErrorCases() []accountFindErrorCase {
	return []accountFindErrorCase{
		{
			// The account repository answers a missing row with an EntityNotFoundError.
			name:       "not found from the repository is a business event without a log",
			findErr:    pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAccount),
			business:   true,
			wantStatus: codes.Unset,
		},
		{
			name:       "infrastructure failure marks the span as error and logs once",
			findErr:    errors.New("connection refused"),
			business:   false,
			wantStatus: codes.Error,
		},
	}
}

// accountFindErrorContext wires a span recorder and a capturing logger into the tracking context.
func accountFindErrorContext() (context.Context, *tracetest.SpanRecorder, *capturingLogger) {
	ctx, recorder := recordingContext()
	logger := &capturingLogger{}

	return libObservability.ContextWithLogger(ctx, logger), recorder, logger
}

func assertAccountFindErrorRecorded(t *testing.T, recorder *tracetest.SpanRecorder, logger *capturingLogger, spanName string, tc accountFindErrorCase) {
	t.Helper()

	span := findSpan(t, recorder, spanName)
	assert.Equal(t, tc.wantStatus, span.Status().Code)

	if tc.business {
		_, ok := findEvent(span, accountFindErrorMessage)
		assert.True(t, ok, "business event must be recorded on the span")
		assert.Empty(t, logger.snapshot(), "a business error must not be logged")

		return
	}

	line := onlyLoggedLine(t, logger)
	assert.Equal(t, libLog.LevelError, line.Level)
	assert.Equal(t, accountFindErrorMessage, line.Msg)
	assert.Contains(t, line.Fields, tc.findErr.Error())
}

func TestUpdateAccount_FindErrorIsRecordedByClass(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	for _, tc := range accountFindErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAccountRepo := account.NewMockRepository(ctrl)
			mockAccountRepo.EXPECT().
				Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
				Return(nil, tc.findErr).
				Times(1)

			uc := &UseCase{AccountRepo: mockAccountRepo}

			ctx, recorder, logger := accountFindErrorContext()

			result, err := uc.UpdateAccount(ctx, organizationID, ledgerID, nil, accountID, &mmodel.UpdateAccountInput{}, mmodel.HolderOffV1)

			assert.Nil(t, result)
			require.Error(t, err)
			assert.Equal(t, tc.findErr, err, "the repository error must be returned unchanged")
			assertAccountFindErrorRecorded(t, recorder, logger, "command.update_account", tc)
		})
	}
}

func TestDeleteAccountByID_FindErrorIsRecordedByClass(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	for _, tc := range accountFindErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAccountRepo := account.NewMockRepository(ctrl)
			mockAccountRepo.EXPECT().
				Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
				Return(nil, tc.findErr).
				Times(1)

			uc := &UseCase{AccountRepo: mockAccountRepo}

			ctx, recorder, logger := accountFindErrorContext()

			err := uc.DeleteAccountByID(ctx, organizationID, ledgerID, nil, accountID, "")

			require.Error(t, err)
			assert.Equal(t, tc.findErr, err, "the repository error must be returned unchanged")
			assertAccountFindErrorRecorded(t, recorder, logger, "command.delete_account_by_id", tc)
		})
	}
}
