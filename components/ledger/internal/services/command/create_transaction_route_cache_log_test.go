// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestCreateAccountingRouteCache_RecordsWriteFailureOnce locks the single owner of the
// cache failure logs: the use case logs each failed branch once at Error and marks its
// span red, because the handler swallows the error after a successful create or update.
// The joined error is not logged again. A write that succeeds logs nothing.
func TestCreateAccountingRouteCache_RecordsWriteFailureOnce(t *testing.T) {
	const (
		setMessage    = "Failed to create transaction route cache"
		legacyMessage = "Failed to delete ledger-scoped transaction route cache"
	)

	organizationID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	routeID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	setFailure := errors.New("redis connection error")
	delFailure := errors.New("redis del timeout")

	tests := []struct {
		name         string
		ledgerScoped bool
		setErr       error
		delErr       error
		wantMessages []string
		wantStatus   codes.Code
	}{
		{
			name:         "failing cache write logs one error and marks the span red",
			setErr:       setFailure,
			wantMessages: []string{setMessage},
			wantStatus:   codes.Error,
		},
		{
			name:       "successful cache write logs nothing",
			wantStatus: codes.Unset,
		},
		{
			name:         "failing ledger-scoped delete logs one error",
			ledgerScoped: true,
			delErr:       delFailure,
			wantMessages: []string{legacyMessage},
			wantStatus:   codes.Error,
		},
		{
			name:         "failing set and delete log one error per branch",
			ledgerScoped: true,
			setErr:       setFailure,
			delErr:       delFailure,
			wantMessages: []string{setMessage, legacyMessage},
			wantStatus:   codes.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRedisRepo := redis.NewMockRedisRepository(ctrl)
			mockRedisRepo.EXPECT().
				SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(0)).
				Return(tt.setErr).
				Times(1)

			// An organization-level route has no ledger-scoped key, so only the set runs.
			route := &mmodel.TransactionRoute{
				ID:             routeID,
				OrganizationID: organizationID,
				Title:          "Test Route",
			}

			if tt.ledgerScoped {
				route.LedgerID = &ledgerID

				mockRedisRepo.EXPECT().
					Del(gomock.Any(), gomock.Any()).
					Return(tt.delErr).
					Times(1)
			}

			uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

			ctx, recorder := spantest.RecordingContext()
			logger := &capturingLogger{}
			ctx = libObservability.ContextWithLogger(ctx, logger)

			err := uc.CreateAccountingRouteCache(ctx, route)

			span := spantest.FindSpan(t, recorder, "command.create_transaction_route_cache")
			assert.Equal(t, tt.wantStatus, span.Status().Code)

			if len(tt.wantMessages) == 0 {
				require.NoError(t, err)
				assert.Empty(t, logger.snapshot(), "a successful cache write must not log")

				return
			}

			for _, branchErr := range []error{tt.setErr, tt.delErr} {
				if branchErr != nil {
					require.ErrorIs(t, err, branchErr, "the joined error must carry every branch failure")
				}
			}

			lines := logger.snapshot()
			require.Len(t, lines, len(tt.wantMessages), "each failed branch must log exactly once")

			for i, msg := range tt.wantMessages {
				assert.Equal(t, libLog.LevelError, lines[i].Level)
				assert.Equal(t, msg, lines[i].Msg)
			}
		})
	}
}

// TestCreateAccountingRouteCache_RecordsConversionFailureOnce covers the msgpack branch:
// a route that cannot be encoded logs once at Error, marks the span red and never
// reaches Redis.
func TestCreateAccountingRouteCache_RecordsConversionFailureOnce(t *testing.T) {
	ctrl := gomock.NewController(t)

	// No Redis expectation: an encode failure must return before any write.
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{TransactionRedisRepo: mockRedisRepo}

	organizationID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	route := &mmodel.TransactionRoute{
		ID:             uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		OrganizationID: organizationID,
		Title:          "Test Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:                uuid.MustParse("44444444-4444-4444-4444-444444444444"),
				OrganizationID:    organizationID,
				OperationType:     "source",
				AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  make(chan int), // msgpack cannot encode a channel
				},
			},
		},
	}

	ctx, recorder := spantest.RecordingContext()
	logger := &capturingLogger{}
	ctx = libObservability.ContextWithLogger(ctx, logger)

	require.Error(t, uc.CreateAccountingRouteCache(ctx, route))

	assert.Equal(t, codes.Error, spantest.FindSpan(t, recorder, "command.create_transaction_route_cache").Status().Code)

	line := onlyLoggedLine(t, logger)
	assert.Equal(t, libLog.LevelError, line.Level)
	assert.Equal(t, "Failed to convert route to cache data", line.Msg)
}
