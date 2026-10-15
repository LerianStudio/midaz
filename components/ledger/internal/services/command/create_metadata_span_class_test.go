// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	txMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestCreateMetadataHelpers_RecordOnSpanByClassWithoutLogging pins the single-point logging
// contract of the two metadata write helpers: a failed write is recorded on the helper's own
// span by error class (technical flips it red, business stays green with an event) and returned
// unchanged, and neither class is logged there, because the calling use case owns the log line.
func TestCreateMetadataHelpers_RecordOnSpanByClassWithoutLogging(t *testing.T) {
	const message = "Failed to create metadata"

	classes := []struct {
		name       string
		err        error
		wantStatus codes.Code
		wantEvent  bool
	}{
		{name: "technical", err: errors.New("mongo unavailable"), wantStatus: codes.Error},
		{
			name:       "business",
			err:        pkg.ValidateBusinessError(constant.ErrMetadataIndexAlreadyExists, constant.EntityLedger),
			wantStatus: codes.Unset,
			wantEvent:  true,
		},
	}

	helpers := []struct {
		name     string
		spanName string
		run      func(t *testing.T, ctx context.Context, ctrl *gomock.Controller, writeErr error) (map[string]any, error)
	}{
		{
			name:     "onboarding",
			spanName: "command.create_metadata",
			run: func(_ *testing.T, ctx context.Context, ctrl *gomock.Controller, writeErr error) (map[string]any, error) {
				repo := onbMongo.NewMockRepository(ctrl)
				repo.EXPECT().Create(gomock.Any(), constant.EntityLedger, gomock.Any()).Return(writeErr).Times(1)

				uc := &UseCase{OnboardingMetadataRepo: repo}

				return uc.CreateOnboardingMetadata(ctx, constant.EntityLedger, "entity-id", map[string]any{"k": "v"})
			},
		},
		{
			name:     "transaction",
			spanName: "command.create_transaction_metadata",
			run: func(_ *testing.T, ctx context.Context, ctrl *gomock.Controller, writeErr error) (map[string]any, error) {
				repo := txMongo.NewMockRepository(ctrl)
				repo.EXPECT().Create(gomock.Any(), constant.EntityTransactionRoute, gomock.Any()).Return(writeErr).Times(1)

				uc := &UseCase{TransactionMetadataRepo: repo}

				return uc.CreateTransactionMetadata(ctx, constant.EntityTransactionRoute, "entity-id", map[string]any{"k": "v"})
			},
		},
	}

	for _, helper := range helpers {
		for _, class := range classes {
			t.Run(helper.name+"/"+class.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				ctx, recorder, logger := accountFindErrorContext()

				result, err := helper.run(t, ctx, ctrl, class.err)

				require.ErrorIs(t, err, class.err)
				assert.Nil(t, result)

				span := findSpan(t, recorder, helper.spanName)
				assert.Equal(t, class.wantStatus, span.Status().Code)

				_, hasEvent := findEvent(span, message)
				if class.wantEvent {
					assert.True(t, hasEvent, "a business error is recorded as a span event")
				}

				assert.Empty(t, logger.snapshot(), "the helper never logs; the use case owns the log line")
			})
		}
	}
}
