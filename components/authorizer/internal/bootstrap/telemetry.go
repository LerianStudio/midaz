// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"fmt"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// ErrNilAuthorizerConfig indicates that the authorizer config passed to InitTelemetry is nil.
var ErrNilAuthorizerConfig = errors.New("authorizer config is nil")

// InitTelemetry initializes OpenTelemetry for the authorizer component using the provided config.
func InitTelemetry(cfg *Config, logger libLog.Logger) (*libOpentelemetry.Telemetry, error) {
	if cfg == nil {
		return nil, ErrNilAuthorizerConfig
	}

	telemetry, err := libOpentelemetry.InitializeTelemetryWithError(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize authorizer telemetry: %w", err)
	}

	return telemetry, nil
}
