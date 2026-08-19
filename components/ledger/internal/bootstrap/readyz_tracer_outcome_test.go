// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type financialDurabilityCheckerStub struct {
	err error
}

func (s financialDurabilityCheckerStub) FinancialDurability(context.Context) error {
	return s.err
}

func TestFinancialRedisDurabilityCheckerFailsReadinessClosed(t *testing.T) {
	t.Parallel()

	checker := NewFinancialRedisDurabilityChecker(
		financialDurabilityCheckerStub{err: errors.New("appendfsync must be always or everysec")}, true)
	check := checker.Check(context.Background())
	assert.Equal(t, StatusDown, check.Status)
	assert.Contains(t, check.Error, "appendfsync")
	assert.True(t, checker.TLSEnabled())

	healthy := NewFinancialRedisDurabilityChecker(financialDurabilityCheckerStub{}, false)
	assert.Equal(t, StatusUp, healthy.Check(context.Background()).Status)
}

func TestTracerOutcomeRequiresDurableRedisForModeOrDrainWorker(t *testing.T) {
	t.Parallel()

	assert.True(t, tracerOutcomeRequiresDurableRedis(tracerOutcomeModeV2, false))
	assert.True(t, tracerOutcomeRequiresDurableRedis(tracerOutcomeModeLegacy, true),
		"rollback drain keeps the old durable backlog authoritative")
	assert.False(t, tracerOutcomeRequiresDurableRedis(tracerOutcomeModeLegacy, false))
}
