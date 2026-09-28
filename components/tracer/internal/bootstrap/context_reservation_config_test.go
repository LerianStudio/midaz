// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

func TestContextReservationRequiresBounds(t *testing.T) {
	t.Parallel()

	cfg := validContextReservationConfig()
	cfg.ContextPolicyAdminEnabled = false
	result, err := loadTestContextReservationConfig(t, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, cfg.ContextPolicyAdminEnabled, "admission must not implicitly enable administration")
	require.Equal(t, 5*time.Minute, result.admission.ReservationLifetime)
	require.Equal(t, 720*time.Hour, result.admission.LongLivedLifetime)
	cfg.ReservationLongLivedTTLHours = "48"
	result, err = loadTestContextReservationConfig(t, cfg)
	require.NoError(t, err)
	require.Equal(t, 48*time.Hour, result.admission.LongLivedLifetime)
	cfg.ReservationLongLivedTTLHours = "0"
	_, err = loadTestContextReservationConfig(t, cfg)
	require.Error(t, err)
	cfg.ReservationLongLivedTTLHours = ""
	cfg.ContextReserveMaxReservations = math.MaxInt32 + 1
	_, err = loadTestContextReservationConfig(t, cfg)
	require.Error(t, err)
	cfg.ContextReserveMaxReservations = 100
	cfg.ContextPolicyCacheEntries = 0
	_, err = loadTestContextReservationConfig(t, cfg)
	require.Error(t, err)
	cfg.ContextPolicyCacheEntries = 10
	cfg.ContextMaxAccounts = 0
	_, err = loadTestContextReservationConfig(t, cfg)
	require.Error(t, err)
}

func TestReserveOperationExpiryIsWiredWithoutReserve(t *testing.T) {
	limitDeps := reaperWiringDeps()
	cfg := &Config{ContextMaxRules: 10, ContextReserveMaxReservations: 100}
	conn := &testutil.IntegrationDBAdapter{}
	expiry, err := initReserveOperationExpiry(cfg, conn, dbmocks.NewMockTxBeginner(gomock.NewController(t)), postgres.NewAuditEventRepositoryWithConnection(conn), limitDeps.reservationRepo)
	require.NoError(t, err)
	require.NotNil(t, expiry, "decision capacity must expire even while Reserve is disabled")
	cfg.ContextReserveMaxReservations = 0
	_, err = initReserveOperationExpiry(cfg, conn, dbmocks.NewMockTxBeginner(gomock.NewController(t)), postgres.NewAuditEventRepositoryWithConnection(conn), limitDeps.reservationRepo)
	require.Error(t, err)
}

// TestParseReservationLongLivedTTLHoursResolvesTheDefault locks the single
// place the long-lived default is resolved for both reservation profiles.
func TestParseReservationLongLivedTTLHoursResolvesTheDefault(t *testing.T) {
	t.Parallel()

	got, err := parseReservationLongLivedTTLHours("")
	require.NoError(t, err)
	require.Equal(t, services.DefaultLongLivedReservationTTL, got)

	got, err = parseReservationLongLivedTTLHours("48")
	require.NoError(t, err)
	require.Equal(t, 48*time.Hour, got)

	for _, invalid := range []string{"0", "-1", "8761", "week"} {
		_, err = parseReservationLongLivedTTLHours(invalid)
		require.Error(t, err, invalid)
	}
}
