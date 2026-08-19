// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package bootstrap

import (
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
)

func integrationControllableClock(clk clock.Clock) clock.Clock {
	return clock.NewSwitchableClock(clk)
}

// UseIntegrationTime swaps the clock shared by every service dependency and
// returns a restore function. It exists only in integration builds.
func (app *Service) UseIntegrationTime(at time.Time) (func(), error) {
	clk, ok := app.clock.(*clock.SwitchableClock)
	if !ok {
		return nil, fmt.Errorf("service clock is not integration-controllable")
	}

	return clk.Use(clock.NewFixedClock(at)), nil
}
