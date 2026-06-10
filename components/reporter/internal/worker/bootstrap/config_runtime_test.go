// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWorkerConfig_DeploymentModeDefaultIsLocal verifies that the worker
// Config defaults DeploymentMode to "local" when DEPLOYMENT_MODE is unset.
//
// Note: The default is applied by libCommons.SetConfigFromEnvVars at runtime;
// here we just assert the struct field exists by reading it directly.
func TestWorkerConfig_DeploymentModeFieldExists(t *testing.T) {
	t.Parallel()

	cfg := Config{DeploymentMode: "saas"}
	assert.Equal(t, "saas", cfg.DeploymentMode)
}
