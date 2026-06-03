// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	clog "github.com/LerianStudio/lib-observability/log"
	libZap "github.com/LerianStudio/lib-observability/zap"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/pongo"
)

// loadConfigAndLogger loads environment config, registers pongo2 filters, and initializes the logger.
func loadConfigAndLogger() (*Config, clog.Logger, error) {
	cfg := &Config{}
	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, nil, fmt.Errorf("failed to load config from env vars: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}

	if err := pongo.RegisterAll(); err != nil {
		return nil, nil, fmt.Errorf("failed to register pongo2 filters and tags: %w", err)
	}

	logger, err := libZap.New(libZap.Config{
		Environment:     resolveZapEnvironment(cfg.EnvName),
		Level:           cfg.LogLevel,
		OTelLibraryName: cfg.OtelLibraryName,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	return cfg, logger, nil
}

func resolveZapEnvironment(envName string) libZap.Environment {
	switch strings.ToLower(strings.TrimSpace(envName)) {
	case "production", "prod":
		return libZap.EnvironmentProduction
	case "staging":
		return libZap.EnvironmentStaging
	case "uat":
		return libZap.EnvironmentUAT
	case "development", "dev":
		return libZap.EnvironmentDevelopment
	default:
		return libZap.EnvironmentLocal
	}
}
