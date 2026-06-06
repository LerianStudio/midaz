// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libZap "github.com/LerianStudio/lib-observability/zap"
	"github.com/stretchr/testify/assert"
)

func TestResolveZapEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		envName string
		want    libZap.Environment
	}{
		{name: "production", envName: "production", want: libZap.EnvironmentProduction},
		{name: "prod", envName: "prod", want: libZap.EnvironmentProduction},
		{name: "staging", envName: "staging", want: libZap.EnvironmentStaging},
		{name: "uat", envName: "uat", want: libZap.EnvironmentUAT},
		{name: "development", envName: "development", want: libZap.EnvironmentDevelopment},
		{name: "dev", envName: "dev", want: libZap.EnvironmentDevelopment},
		{name: "empty", envName: "", want: libZap.EnvironmentLocal},
		{name: "mixed case Production", envName: "Production", want: libZap.EnvironmentProduction},
		{name: "padded prod", envName: "  prod  ", want: libZap.EnvironmentProduction},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveZapEnvironment(tt.envName)
			assert.Equal(t, tt.want, got)
		})
	}
}
