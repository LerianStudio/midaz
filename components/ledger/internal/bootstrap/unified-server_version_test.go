// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/buildinfo"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewUnifiedServer_VersionEndpointServesCompiledIdentity locks the /version
// contract: the route answers with the identity compiled into the binary and
// nothing else. The body carries exactly the seven identity keys — no
// dependencyManifest (that lives behind --version, off the API port) and no
// requestDate (a clock reading is not build identity, and it made every
// response uncacheable and every golden body unstable).
//
// service is the roster identity the binary also reports as the OTel
// service.name, so one name identifies the process across /version, traces and
// the streaming manifest.
func TestNewUnifiedServer_VersionEndpointServesCompiledIdentity(t *testing.T) {
	t.Parallel()

	server := NewUnifiedServer(":0", "ledger", newTestLogger(), &libOpentelemetry.Telemetry{}, nil, nil, nil)
	require.NotNil(t, server, "NewUnifiedServer should return a non-nil server")

	req, err := http.NewRequest(http.MethodGet, "/version", nil)
	require.NoError(t, err)

	resp, err := server.app.Test(req)
	require.NoError(t, err)

	defer func() {
		_ = resp.Body.Close()
	}()

	require.Equal(t, http.StatusOK, resp.StatusCode, "/version should answer 200 unauthenticated")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got), "/version should answer JSON; body=%s", body)

	want := buildinfo.Get()

	assert.Equal(t, "v1", got["schemaVersion"], "schemaVersion pins the body shape")
	assert.Equal(t, "ledger", got["service"], "service is the roster identity the server was built with")
	assert.Equal(t, want.Version, got["version"], "version comes from the compiled identity, never from env")
	assert.Equal(t, want.Revision, got["revision"], "revision comes from the compiled identity")
	assert.Equal(t, want.BuildTime, got["buildTime"], "buildTime comes from the compiled identity")
	assert.Equal(t, want.Modified, got["modified"], "modified reports the dirty-tree stamp")
	assert.Equal(t, want.GoVersion, got["goVersion"], "goVersion reports the toolchain")

	assert.ElementsMatch(t,
		[]string{"schemaVersion", "service", "version", "revision", "buildTime", "modified", "goVersion"},
		keysOf(got),
		"/version must carry exactly the seven identity keys: no dependencyManifest, no requestDate")
}
