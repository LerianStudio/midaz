// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingResponseWriter wraps an httptest.ResponseRecorder and forces the
// first Write call to fail. This simulates a closed connection mid-encode,
// which is the only realistic path through writeJSON's fallback branch.
type failingResponseWriter struct {
	rec       *httptest.ResponseRecorder
	failOnce  bool
	captured  []byte
	writeErr  error
	writeCall int
}

func (f *failingResponseWriter) Header() http.Header { return f.rec.Header() }

func (f *failingResponseWriter) WriteHeader(code int) { f.rec.WriteHeader(code) }

func (f *failingResponseWriter) Write(p []byte) (int, error) {
	f.writeCall++

	if f.failOnce && f.writeCall == 1 {
		return 0, f.writeErr
	}

	f.captured = append(f.captured, p...)

	return f.rec.Write(p)
}

// TestWriteJSONFallback_SchemaCompliant verifies that the fallback payload
// emitted by writeJSON when json.Encoder.Encode fails parses cleanly into
// the canonical Response struct. The fallback must mirror the documented
// /readyz response schema (status + checks + version + deployment_mode)
// rather than introducing an unexpected `error` field.
func TestWriteJSONFallback_SchemaCompliant(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	failing := &failingResponseWriter{
		rec:      rec,
		failOnce: true,
		writeErr: errors.New("simulated closed connection"),
	}

	// Pass a Response that would normally serialize fine — the failure is
	// in the underlying writer, not the marshaller. This forces writeJSON
	// down the fallback branch.
	writeJSON(failing, http.StatusServiceUnavailable, Response{
		Status:         "unhealthy",
		Checks:         map[string]DependencyCheck{},
		Version:        "1.2.3",
		DeploymentMode: "saas",
	})

	require.NotEmpty(t, failing.captured, "fallback bytes should have been written")

	// The fallback bytes MUST decode back into the canonical Response
	// struct without unknown fields. DisallowUnknownFields detects any
	// drift back to the legacy shape (e.g. `"error": ...`).
	dec := json.NewDecoder(bytes.NewReader(failing.captured))
	dec.DisallowUnknownFields()

	var got Response
	require.NoError(t, dec.Decode(&got),
		"fallback payload must parse as canonical Response (no unknown fields)")

	assert.Equal(t, "unhealthy", got.Status)
	assert.NotNil(t, got.Checks, "fallback must emit a non-nil checks map")
	assert.Empty(t, got.Checks, "fallback checks map should be empty")
	assert.Empty(t, got.Version)
	assert.Empty(t, got.DeploymentMode)
}
