// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libZap "github.com/LerianStudio/lib-commons/v5/commons/zap"
	"github.com/stretchr/testify/assert"
)

func TestOnboardingResolveLoggerEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  string
		want libZap.Environment
	}{
		{name: "production", env: "production", want: libZap.EnvironmentProduction},
		{name: "staging", env: "staging", want: libZap.EnvironmentStaging},
		{name: "uat", env: "uat", want: libZap.EnvironmentUAT},
		{name: "local", env: "local", want: libZap.EnvironmentLocal},
		{name: "default development", env: "", want: libZap.EnvironmentDevelopment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveLoggerEnvironment(tt.env))
		})
	}
}
