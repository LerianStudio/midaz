// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/stretchr/testify/assert"
)

func zeroTelemetry() *libOpentelemetry.Telemetry {
	return &libOpentelemetry.Telemetry{}
}

func TestResolveSettingsCacheTTL(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	assert.Equal(t, 5*time.Minute, resolveSettingsCacheTTL(&Config{}, logger))
	assert.Equal(t, 5*time.Minute, resolveSettingsCacheTTL(&Config{SettingsCacheTTL: "invalid"}, logger))
	assert.Equal(t, 5*time.Minute, resolveSettingsCacheTTL(&Config{SettingsCacheTTL: "-5s"}, logger))
	assert.Equal(t, 30*time.Second, resolveSettingsCacheTTL(&Config{SettingsCacheTTL: "30s"}, logger))
}
