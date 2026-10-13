// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	multiTenantWithoutAuthErr = "MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true; " +
		"running multi-tenant mode without authentication allows cross-tenant data access"
	saasWithoutAuthErr = "DEPLOYMENT_MODE=saas requires PLUGIN_AUTH_ENABLED=true; " +
		"a saas deployment never serves the business routes without authentication"
)

func TestAuthPresenceWarning(t *testing.T) {
	t.Parallel()

	// authEnv is the raw PLUGIN_AUTH_ENABLED value ("" when unset);
	// authEnabled is what the config loader derived from it.
	tests := []struct {
		name               string
		deploymentMode     string
		envName            string
		authEnv            string
		authEnabled        bool
		multiTenantEnabled bool
		wantErr            string
		wantWarn           string
	}{
		{
			name:           "byoc production single-tenant without auth boots with a warning",
			deploymentMode: "byoc",
			envName:        "production",
			authEnv:        "false",
			wantWarn:       authDisabledNonLocalMsg,
		},
		{
			name:           "undocumented mode without auth boots with the non-local warning",
			deploymentMode: "onprem",
			envName:        "production",
			wantWarn:       authDisabledNonLocalMsg,
		},
		{
			name:           "saas single-tenant without auth fails boot",
			deploymentMode: "saas",
			envName:        "staging",
			authEnv:        "false",
			wantErr:        saasWithoutAuthErr,
		},
		{
			name:           "padded mixed-case saas cannot slip the gate",
			deploymentMode: " SaaS ",
			envName:        "staging",
			wantErr:        saasWithoutAuthErr,
		},
		{
			name:               "multi-tenant local without auth fails boot",
			deploymentMode:     "local",
			envName:            "production",
			multiTenantEnabled: true,
			wantErr:            multiTenantWithoutAuthErr,
		},
		{
			name:               "multi-tenant byoc without auth fails boot",
			deploymentMode:     "byoc",
			envName:            "production",
			multiTenantEnabled: true,
			wantErr:            multiTenantWithoutAuthErr,
		},
		{
			name:               "multi-tenant saas without auth fails with the multi-tenant error",
			deploymentMode:     "saas",
			envName:            "production",
			multiTenantEnabled: true,
			wantErr:            multiTenantWithoutAuthErr,
		},
		{
			name:           "unset mode without auth boots with the development warning",
			deploymentMode: "",
			envName:        "local",
			wantWarn:       authDisabledLocalMsg,
		},
		{
			name:           "local mode without auth boots with the development warning",
			deploymentMode: "local",
			envName:        "development",
			authEnv:        "false",
			wantWarn:       authDisabledLocalMsg,
		},
		{
			name:           "production env name with unset mode does not require auth",
			deploymentMode: "",
			envName:        "production",
			wantWarn:       authDisabledLocalMsg,
		},
		{
			name:           "saas with auth boots silently",
			deploymentMode: "saas",
			envName:        "production",
			authEnv:        "true",
			authEnabled:    true,
		},
		{
			name:               "multi-tenant with auth boots silently",
			deploymentMode:     "saas",
			envName:            "production",
			authEnv:            "true",
			authEnabled:        true,
			multiTenantEnabled: true,
		},
		{
			name:           "byoc with auth boots silently",
			deploymentMode: "byoc",
			envName:        "production",
			authEnv:        "1",
			authEnabled:    true,
		},
		{
			name:           "unparseable auth flag fails boot in byoc",
			deploymentMode: "byoc",
			envName:        "production",
			authEnv:        "yes",
			wantErr:        `PLUGIN_AUTH_ENABLED="yes" is not a valid boolean; set true or false`,
		},
		{
			name:           "auth flag with a trailing newline fails boot in local",
			deploymentMode: "local",
			envName:        "development",
			authEnv:        "true\n",
			wantErr:        `PLUGIN_AUTH_ENABLED="true\n" is not a valid boolean; set true or false`,
		},
		{
			name:               "unparseable auth flag is reported before the multi-tenant rule",
			deploymentMode:     "saas",
			envName:            "production",
			authEnv:            "on",
			multiTenantEnabled: true,
			wantErr:            `PLUGIN_AUTH_ENABLED="on" is not a valid boolean; set true or false`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				DeploymentMode:     tt.deploymentMode,
				EnvName:            tt.envName,
				AuthEnabled:        tt.authEnabled,
				MultiTenantEnabled: tt.multiTenantEnabled,
			}

			warning, err := authPresenceWarning(cfg, tt.authEnv)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Empty(t, warning)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantWarn, warning)
		})
	}
}

func TestInitServersWithOptions_RejectsSaaSWithoutAuthBeforeInfrastructure(t *testing.T) {
	t.Setenv("DEPLOYMENT_MODE", "saas")
	t.Setenv("PLUGIN_AUTH_ENABLED", "false")
	t.Setenv("MULTI_TENANT_ENABLED", "false")
	t.Setenv("ENABLE_TELEMETRY", "false")

	reached := stubOnboardingPostgres(t)
	logger := &authGateLogger{}

	service, err := InitServersWithOptions(&Options{Logger: logger})

	require.EqualError(t, err, saasWithoutAuthErr)
	assert.Nil(t, service)
	assert.False(t, reached(), "no dependency connection may be attempted before the auth gate")
	assertRefusedBootLoggedNothing(t, logger)
}

func TestInitServersWithOptions_RejectsUnparseableAuthFlagBeforeInfrastructure(t *testing.T) {
	t.Setenv("DEPLOYMENT_MODE", "byoc")
	t.Setenv("PLUGIN_AUTH_ENABLED", "yes")
	t.Setenv("MULTI_TENANT_ENABLED", "false")
	t.Setenv("ENABLE_TELEMETRY", "false")

	reached := stubOnboardingPostgres(t)
	logger := &authGateLogger{}

	service, err := InitServersWithOptions(&Options{Logger: logger})

	require.EqualError(t, err, `PLUGIN_AUTH_ENABLED="yes" is not a valid boolean; set true or false`)
	assert.Nil(t, service)
	assert.False(t, reached(), "no dependency connection may be attempted before the auth gate")
	assertRefusedBootLoggedNothing(t, logger)
}

func TestInitServersWithOptions_BootsWithoutAuthOutsideSaaS(t *testing.T) {
	tests := []struct {
		name           string
		deploymentMode string
		wantMode       string
		wantWarn       string
	}{
		{name: "unset mode", deploymentMode: "", wantMode: DeploymentModeLocal, wantWarn: authDisabledLocalMsg},
		{name: "byoc mode", deploymentMode: "byoc", wantMode: DeploymentModeBYOC, wantWarn: authDisabledNonLocalMsg},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ENV_NAME", "production")
			t.Setenv("DEPLOYMENT_MODE", tt.deploymentMode)
			t.Setenv("PLUGIN_AUTH_ENABLED", "false")
			t.Setenv("MULTI_TENANT_ENABLED", "false")
			t.Setenv("ENABLE_TELEMETRY", "false")

			reached := stubOnboardingPostgres(t)
			logger := &authGateLogger{}

			service, err := InitServersWithOptions(&Options{Logger: logger})

			require.ErrorIs(t, err, errOnboardingPostgresReached, "boot must pass the auth gate and reach the first dependency")
			assert.Nil(t, service)
			assert.True(t, reached())

			warnings := logger.warningsAbout("PLUGIN_AUTH_ENABLED")
			require.Len(t, warnings, 1)
			assert.Equal(t, tt.wantWarn, warnings[0].msg)
			assert.Equal(t, tt.wantMode, warnings[0].fields["deployment_mode"])
		})
	}
}

// assertRefusedBootLoggedNothing checks that a refused boot logs no Warn or
// Error: main logs the returned error once. It also pins the gate before
// telemetry init, which logs a "Telemetry disabled" Warn under
// ENABLE_TELEMETRY=false.
func assertRefusedBootLoggedNothing(t *testing.T, logger *authGateLogger) {
	t.Helper()

	assert.Empty(t, logger.at(libLog.LevelError),
		"a refused boot must not log an Error: main logs the returned error once")
	assert.Empty(t, logger.at(libLog.LevelWarn),
		"a refused boot must not log a Warn, and the gate must run before telemetry init")
}

var errOnboardingPostgresReached = errors.New("onboarding postgres connector reached")

// stubOnboardingPostgres replaces the onboarding PostgreSQL connector with one
// that fails with errOnboardingPostgresReached, and reports whether it ran.
// Callers must not run in parallel: the connector is package state.
func stubOnboardingPostgres(t *testing.T) func() bool {
	t.Helper()

	var mu sync.Mutex

	called := false
	original := onboardingPostgresConnector
	onboardingPostgresConnector = func(_ *Config, _ libLog.Logger) (*libPostgres.Client, error) {
		mu.Lock()
		called = true
		mu.Unlock()

		return nil, errOnboardingPostgresReached
	}

	t.Cleanup(func() { onboardingPostgresConnector = original })

	return func() bool {
		mu.Lock()
		defer mu.Unlock()

		return called
	}
}

type authGateLogEntry struct {
	level  int
	msg    string
	fields map[string]any
}

// authGateLogger records Log calls; With and WithGroup return the same
// recorder so entries from derived loggers are captured too.
type authGateLogger struct {
	mu      sync.Mutex
	entries []authGateLogEntry
}

func (l *authGateLogger) Log(_ context.Context, level int, msg string, fields ...any) {
	entry := authGateLogEntry{level: level, msg: msg, fields: map[string]any{}}

	for _, field := range fields {
		if typed, ok := field.(libLog.Field); ok {
			entry.fields[typed.Key] = typed.Value
		}
	}

	l.mu.Lock()
	l.entries = append(l.entries, entry)
	l.mu.Unlock()
}

func (l *authGateLogger) With(...any) libLog.Logger { return l }

func (l *authGateLogger) WithGroup(string) libLog.Logger { return l }

func (l *authGateLogger) Enabled(int) bool { return true }

func (l *authGateLogger) Sync(context.Context) error { return nil }

func (l *authGateLogger) at(level int) []authGateLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	matched := []authGateLogEntry{}

	for _, entry := range l.entries {
		if entry.level == level {
			matched = append(matched, entry)
		}
	}

	return matched
}

// warningsAbout returns the Warn entries whose config field names key.
func (l *authGateLogger) warningsAbout(key string) []authGateLogEntry {
	matched := []authGateLogEntry{}

	for _, entry := range l.at(libLog.LevelWarn) {
		if entry.fields["config"] == key {
			matched = append(matched, entry)
		}
	}

	return matched
}
