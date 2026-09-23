// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// TestServiceName_UnsetEnvNamesTheProcessOnEverySurface locks the process name
// for a deploy that sets no OTEL_RESOURCE_SERVICE_NAME: /version, the OTel
// resource and the streaming manifest must all report the same non-empty name.
func TestServiceName_UnsetEnvNamesTheProcessOnEverySurface(t *testing.T) {
	// Not parallel: t.Setenv.
	t.Setenv("OTEL_RESOURCE_SERVICE_NAME", "")
	require.NoError(t, os.Unsetenv("OTEL_RESOURCE_SERVICE_NAME"))

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))
	ApplyMultiTenantDefaults(cfg)

	logger := libLog.NewNop()
	ctrl := gomock.NewController(t)
	app, err := in.NewRoutes(in.RoutesDeps{
		Logger:                       logger,
		Telemetry:                    &libOtel.Telemetry{TelemetryConfig: libOtel.TelemetryConfig{Logger: logger}},
		HealthChecker:                &in.HealthChecker{},
		Cfg:                          &in.RouteConfig{},
		RuleService:                  in.NewMockRuleService(ctrl),
		LimitService:                 in.NewMockLimitService(ctrl),
		ValidationService:            mocks.NewMockValidationService(ctrl),
		TransactionValidationService: mocks.NewMockTransactionValidationService(ctrl),
		AuditEventService:            in.NewMockAuditEventService(ctrl),
		Guard:                        middleware.NewAuthGuard(middleware.AuthGuardConfig{}, nil),
		Clock:                        clock.New(),
		ServiceName:                  cfg.OtelServiceName,
	})
	require.NoError(t, err)

	resp, err := app.Test(httptest.NewRequest(nethttp.MethodGet, "/version", nil), fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var version struct {
		Service string `json:"service"`
	}
	require.NoError(t, json.Unmarshal(body, &version), "body=%s", body)

	manifestHandler, err := BuildStreamingManifestHandler(cfg)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	manifestHandler.ServeHTTP(rec, httptest.NewRequest(nethttp.MethodGet, pkgStreaming.ManifestRoutePath, nil))
	require.Equal(t, nethttp.StatusOK, rec.Code)

	var manifest struct {
		Publisher struct {
			ServiceName string `json:"serviceName"`
		} `json:"publisher"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &manifest))

	assert.Equal(t, "tracer", manifest.Publisher.ServiceName, "manifest names the roster identity")
	assert.Equal(t, manifest.Publisher.ServiceName, version.Service, "/version must name the process as the manifest does")
	assert.Equal(t, manifest.Publisher.ServiceName, telemetryConfig(cfg, logger).ServiceName, "the OTel resource must name the process as the manifest does")
}
