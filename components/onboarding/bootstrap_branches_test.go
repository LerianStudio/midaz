// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package onboarding

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// stubBalancePort is a minimal mbootstrap.BalancePort so we can exercise the
// unified-mode path on InitServiceWithOptionsOrError without a real transaction
// module.
type stubBalancePort struct{}

func (stubBalancePort) CreateBalanceSync(_ context.Context, _ mmodel.CreateBalanceInput) (*mmodel.Balance, error) {
	return &mmodel.Balance{
		Available: decimal.NewFromInt(0),
		OnHold:    decimal.NewFromInt(0),
	}, nil
}

func (stubBalancePort) DeleteAllBalancesByAccountID(_ context.Context, _, _, _ uuid.UUID, _ string) error {
	return nil
}

func (stubBalancePort) CheckHealth(_ context.Context) error { return nil }

func TestInitServiceWithOptionsOrError_UnifiedModeRequiresBalancePort(t *testing.T) {
	// Cannot t.Parallel() because we mutate env vars.
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("REDIS_HOST", "localhost:9999")

	opts := &Options{UnifiedMode: true, BalancePort: nil}

	require.NotPanics(t, func() {
		svc, err := InitServiceWithOptionsOrError(opts)
		assert.Nil(t, svc)
		assert.ErrorIs(t, err, ErrUnifiedModeRequiresBalancePort)
	})
}

func TestInitServiceWithOptionsOrError_NonUnifiedFailsOnRedis(t *testing.T) {
	// Cannot t.Parallel() because we mutate env vars.
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("MONGO_HOST", "localhost")
	t.Setenv("REDIS_HOST", "localhost:9999")

	opts := &Options{UnifiedMode: false, BalancePort: stubBalancePort{}}

	require.NotPanics(t, func() {
		svc, err := InitServiceWithOptionsOrError(opts)
		assert.Nil(t, svc)
		assert.Error(t, err)
	})
}
