// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestTracerCombinedActivationRequiresReadyIntegration(t *testing.T) {
	for _, mode := range []string{"off", "advisory", "enforce"} {
		t.Run(mode, func(t *testing.T) {
			uc := &UseCase{}
			settings := mmodel.DefaultLedgerSettings().Tracer
			settings.Mode = mode
			settings.ValidationMode = "rules-and-limits"
			err := uc.validateTracerActivation(t.Context(), settings)
			if mode == "off" {
				require.NoError(t, err)
			} else {
				var unavailable pkg.ServiceUnavailableError
				require.ErrorAs(t, err, &unavailable)
			}
			settings.ValidationMode = "limits"
			require.NoError(t, uc.validateTracerActivation(t.Context(), settings))
		})
	}
}

func TestUpdateSettingsChecksMergedTracerActivation(t *testing.T) {
	org := uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338")
	id := uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10")
	for _, tc := range []struct {
		name           string
		current, patch map[string]any
		allowed        bool
	}{
		{"turn on preconfigured rules", map[string]any{"validationMode": "rules-and-limits", "mode": "off"}, map[string]any{"mode": "enforce"}, false},
		{"switch active ledger to rules", map[string]any{"mode": "advisory"}, map[string]any{"validationMode": "rules-and-limits"}, false},
		{"disable unavailable profile", map[string]any{"mode": "enforce", "validationMode": "rules-and-limits"}, map[string]any{"mode": "off"}, true},
		{"configure while off", map[string]any{"mode": "off"}, map[string]any{"validationMode": "rules-and-limits"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := ledger.NewMockRepository(gomock.NewController(t))
			repo.EXPECT().UpdateSettingsAtomic(gomock.Any(), org, id, gomock.Any()).DoAndReturn(func(_ context.Context, _, _ uuid.UUID, merge func(map[string]any) (map[string]any, error)) (map[string]any, error) {
				return merge(map[string]any{"tracer": tc.current})
			})
			uc := &UseCase{LedgerRepo: repo}
			result, err := uc.UpdateLedgerSettings(t.Context(), org, id, map[string]any{"tracer": tc.patch})
			if tc.allowed {
				require.NoError(t, err)
				require.NotNil(t, result)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
			}
		})
	}
}

func TestCreateLedgerRejectsUnavailableCombinedProfileBeforeIO(t *testing.T) {
	mode, validationMode := "enforce", "rules-and-limits"
	uc := &UseCase{LedgerRepo: ledger.NewMockRepository(gomock.NewController(t))}
	result, err := uc.CreateLedger(t.Context(), uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338"), &mmodel.CreateLedgerInput{Name: "combined", Settings: &mmodel.LedgerSettingsInput{Tracer: &mmodel.TracerSettingsInput{Mode: &mode, ValidationMode: &validationMode}}})
	require.Error(t, err)
	require.Nil(t, result)
}

func TestTracerActivationVerifier(t *testing.T) {
	for _, mode := range []string{"off", "advisory", "enforce"} {
		for _, ready := range []bool{false, true} {
			t.Run(mode+"/"+map[bool]string{false: "unavailable", true: "ready"}[ready], func(t *testing.T) {
				verifier := NewMockTracerActivationVerifier(gomock.NewController(t))
				settings := mmodel.DefaultLedgerSettings().Tracer
				settings.Mode, settings.ValidationMode = mode, "rules-and-limits"
				var expected error
				if !ready {
					expected = errors.New("integration not ready")
				}
				if mode != "off" {
					verifier.EXPECT().ValidateActivation(gomock.Any()).Return(expected)
				}
				uc := &UseCase{TracerActivation: verifier}
				err := uc.validateTracerActivation(t.Context(), settings)
				if mode == "off" || ready {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, expected)
				}
			})
		}
	}
}

func TestLegacyReserveCannotDowngradeCombinedControls(t *testing.T) {
	for _, tc := range []struct {
		name, mode, posture string
		skip                bool
		rejected            bool
	}{
		{"enforce closed", "enforce", "closed", false, true},
		{"enforce open", "enforce", "open", false, true},
		{"advisory", "advisory", "closed", false, true},
		{"off", "off", "closed", false, false},
		{"authorized skip", "enforce", "closed", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, span, logger := anchorDeps()
			defer span.End()
			uc := &UseCase{}
			settings := mmodel.TracerSettings{Mode: tc.mode, FailPosture: tc.posture, ValidationMode: "rules-and-limits"}
			result := uc.reserveTransaction(ctx, span, logger, settings, uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10"), decimal.NewFromInt(10), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, tc.skip)
			require.Equal(t, tc.rejected, result.Kind == reservationReject)
			require.Empty(t, result.Handle.ReservationIDs)
		})
	}
}
