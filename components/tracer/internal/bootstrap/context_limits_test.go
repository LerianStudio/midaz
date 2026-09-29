// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestInitContextLimitDefinitionPolicy(t *testing.T) {
	t.Parallel()

	for _, producers := range []string{"", "   "} {
		cfg := validContextPolicyConfig()
		cfg.TracerPlatformProducers = producers
		cfg.ContextLimitMaxScopes = 0

		disabled, err := initContextLimitDefinitionPolicy(cfg)
		require.NoError(t, err, "a validations-only Tracer reads no limit bounds")
		require.Nil(t, disabled, "a validations-only Tracer accepts any limit scope")
	}

	for _, tc := range []struct {
		name   string
		mutate func(*Config)
		valid  bool
	}{
		{name: "valid", mutate: func(*Config) {}, valid: true},
		{name: "invalid bounds", mutate: func(c *Config) { c.ContextMaxAccounts = 0 }},
		{name: "invalid fraction digits", mutate: func(c *Config) { c.ContextMaxFractionDigits = "1" }},
		{name: "no scopes", mutate: func(c *Config) { c.ContextLimitMaxScopes = 0 }},
		{name: "no scope bytes", mutate: func(c *Config) { c.ContextLimitMaxScopeBytes = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validContextPolicyConfig()
			cfg.TracerPlatformProducers = testPlatformProducers
			cfg.ContextLimitMaxScopes = 100
			cfg.ContextLimitMaxScopeBytes = 32768
			tc.mutate(cfg)

			policy, err := initContextLimitDefinitionPolicy(cfg)
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, policy)

				return
			}

			require.Error(t, err)
			require.Nil(t, policy)
		})
	}
}

// unusedAuditWriter satisfies the constructor; neither path under test
// reaches the audit write.
type unusedAuditWriter struct{ command.AuditWriter }

// TestLimitCreateScopeFollowsTheReservationSurface drives a merchant-scoped
// limit through the create command wired with the bootstrap policy. With the
// reservation surface enabled the policy refuses it before any write; without
// it the command proceeds to persistence, observed as the transaction begin.
func TestLimitCreateScopeFollowsTheReservationSurface(t *testing.T) {
	t.Parallel()

	errPersistReached := errors.New("persistence reached")

	for _, tc := range []struct {
		name      string
		producers string
		begins    int
		want      error
	}{
		{name: "validations-only accepts a merchant scope", producers: "", begins: 1, want: errPersistReached},
		{name: "reservation surface rejects a merchant scope", producers: testPlatformProducers, begins: 0, want: constant.ErrContextLimitsUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validContextPolicyConfig()
			cfg.TracerPlatformProducers = tc.producers
			cfg.ContextLimitMaxScopes = 10
			cfg.ContextLimitMaxScopeBytes = 4096

			policy, err := initContextLimitDefinitionPolicy(cfg)
			require.NoError(t, err)

			ctrl := gomock.NewController(t)
			txBeginner := dbmocks.NewMockTxBeginner(ctrl)
			txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(nil, errPersistReached).Times(tc.begins)

			cmd, err := command.NewCreateLimitCommand(command.NewMockLimitRepository(ctrl), testutil.NewDefaultMockClock(), unusedAuditWriter{}, txBeginner)
			require.NoError(t, err)

			cmd.ContextLimits = policy

			merchant := testutil.MustDeterministicUUID(921)
			result, err := cmd.Execute(t.Context(), &command.CreateLimitInput{
				Name: "Merchant limit", LimitType: model.LimitTypeDaily, Asset: "BRL",
				MaxAmount: decimal.NewFromInt(100), Scopes: []model.Scope{{MerchantID: &merchant}},
			})
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, result)
		})
	}
}
