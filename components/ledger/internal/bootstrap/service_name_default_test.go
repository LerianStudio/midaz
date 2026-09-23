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
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	applyConfigDefaults(cfg)

	server := NewUnifiedServer(":0", cfg.OtelServiceName, newTestLogger(), &libOpentelemetry.Telemetry{}, nil, nil, nil)
	require.NotNil(t, server)

	resp, err := server.app.Test(httptest.NewRequest(nethttp.MethodGet, "/version", nil))
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

	assert.Equal(t, "ledger", manifest.Publisher.ServiceName, "manifest names the roster identity")
	assert.Equal(t, manifest.Publisher.ServiceName, version.Service, "/version must name the process as the manifest does")
	assert.Equal(t, manifest.Publisher.ServiceName, cfg.OtelServiceName, "the OTel resource must name the process as the manifest does")
}
