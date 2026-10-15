// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestResolveTypeDefaultDirection_ClassifiesLookupErrors verifies the best-effort
// lookup never fails the caller, keeps an expected miss at Debug, and surfaces a
// tolerated infrastructure failure once at Warn with the error and a span event,
// without changing the span status.
func TestResolveTypeDefaultDirection_ClassifiesLookupErrors(t *testing.T) {
	const (
		message    = "Account type default direction lookup miss; falling back to implied default"
		typeKey    = "checking"
		parentSpan = "test.parent"
	)

	organizationID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name       string
		findErr    error
		wantLevel  int
		wantStatus codes.Code
		wantEvent  bool
	}{
		{
			name:       "infrastructure failure logs one warn with the error",
			findErr:    errors.New("connection refused"),
			wantLevel:  libLog.LevelWarn,
			wantStatus: codes.Unset,
			wantEvent:  true,
		},
		{
			name:       "not found sentinel stays a debug miss",
			findErr:    services.ErrDatabaseItemNotFound,
			wantLevel:  libLog.LevelDebug,
			wantStatus: codes.Unset,
		},
		{
			name:       "business error stays a debug miss",
			findErr:    pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAccountType),
			wantLevel:  libLog.LevelDebug,
			wantStatus: codes.Unset,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			repo := accounttype.NewMockRepository(ctrl)
			repo.EXPECT().
				FindByKey(gomock.Any(), organizationID, ledgerID, typeKey).
				Return(nil, tt.findErr).
				Times(1)

			uc := &UseCase{AccountTypeRepo: repo}

			ctx, recorder := recordingContext()
			logger := &capturingLogger{}
			ctx = libObservability.ContextWithLogger(ctx, logger)

			_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
			ctx, span := tracer.Start(ctx, parentSpan)

			got := uc.resolveTypeDefaultDirection(ctx, organizationID, ledgerID, typeKey)

			span.End()

			assert.Empty(t, got, "the caller must fall back to the implied default")

			line := onlyLoggedLine(t, logger)
			assert.Equal(t, tt.wantLevel, line.Level)
			assert.Equal(t, message, line.Msg)
			assert.Contains(t, line.Fields, typeKey)

			if tt.wantLevel == libLog.LevelWarn {
				assert.Contains(t, line.Fields, tt.findErr.Error(), "the warn must carry the error")
			} else {
				assert.NotContains(t, line.Fields, tt.findErr.Error(), "an expected miss must not carry the error")
			}

			recorded := findSpan(t, recorder, parentSpan)
			require.Equal(t, tt.wantStatus, recorded.Status().Code, "a tolerated fallback must not change the span status")

			_, hasEvent := findEvent(recorded, message)
			assert.Equal(t, tt.wantEvent, hasEvent, "only a tolerated infrastructure failure records the event")
		})
	}
}
