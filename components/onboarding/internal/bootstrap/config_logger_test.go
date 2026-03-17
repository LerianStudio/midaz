package bootstrap

import (
	"testing"

	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
	"github.com/stretchr/testify/assert"
)

func TestResolveLoggerEnvironment(t *testing.T) {
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
