// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build !integration

package bootstrap

import "github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"

func integrationControllableClock(clk clock.Clock) clock.Clock {
	return clk
}
