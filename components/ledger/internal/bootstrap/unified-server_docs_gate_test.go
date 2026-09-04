// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"net/http"
	"os"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unsetDocsGate removes OPENAPI_DOCS_ENABLED for the duration of the test.
// t.Setenv snapshots the original value and restores it on cleanup; the Unsetenv
// that follows leaves the variable genuinely absent, which is the deployment
// posture the default covers.
func unsetDocsGate(t *testing.T) {
	t.Helper()

	t.Setenv("OPENAPI_DOCS_ENABLED", "")
	require.NoError(t, os.Unsetenv("OPENAPI_DOCS_ENABLED"))
}

// TestOpenAPIDocsEnabled pins the docs-gate semantics: the spec + Scalar surface
// stays OFF unless an operator explicitly opts in. Absent and unparseable values
// fall back to the default rather than exposing the contract.
func TestOpenAPIDocsEnabled(t *testing.T) {
	tests := []struct {
		name  string
		value string
		unset bool
		want  bool
	}{
		{name: "unset defaults to disabled", unset: true, want: false},
		{name: "empty defaults to disabled", value: "", want: false},
		{name: "unparseable defaults to disabled", value: "yes-please", want: false},
		{name: "1 enables", value: "1", want: true},
		{name: "t enables", value: "t", want: true},
		{name: "T enables", value: "T", want: true},
		{name: "true enables", value: "true", want: true},
		{name: "TRUE enables", value: "TRUE", want: true},
		{name: "True enables", value: "True", want: true},
		{name: "0 disables", value: "0", want: false},
		{name: "f disables", value: "f", want: false},
		{name: "F disables", value: "F", want: false},
		{name: "false disables", value: "false", want: false},
		{name: "FALSE disables", value: "FALSE", want: false},
		{name: "False disables", value: "False", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv precludes t.Parallel here.
			if tt.unset {
				unsetDocsGate(t)
			} else {
				t.Setenv("OPENAPI_DOCS_ENABLED", tt.value)
			}

			assert.Equal(t, tt.want, openAPIDocsEnabled())
		})
	}
}

// TestNewUnifiedServer_SpecNotServedWhenDocsGateUnset asserts the default reaches the
// assembled document, not just the gate helper: with OPENAPI_DOCS_ENABLED absent, a
// document is assembled (one registrar) but ServeSpec never runs, so none of the spec
// routes it owns answer at the root. Distinct from the explicit-"false" case — an
// absent variable is the real deployment posture.
func TestNewUnifiedServer_SpecNotServedWhenDocsGateUnset(t *testing.T) {
	// t.Setenv (inside unsetDocsGate) precludes t.Parallel here.
	unsetDocsGate(t)

	server := newV2DirectServer(t, &middleware.AuthClient{Enabled: false})

	for _, path := range []string{"/openapi.json", "/openapi.yaml", "/docs"} {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, path, nil)
			require.NoError(t, err)

			resp, err := server.app.Test(req)
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode,
				"%s must not be served when OPENAPI_DOCS_ENABLED is unset", path)
		})
	}
}
