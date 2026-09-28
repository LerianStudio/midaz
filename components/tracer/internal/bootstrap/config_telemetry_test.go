// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/buildinfo"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
)

// TestTelemetryConfig_CarriesCompiledIdentity pins the OTel resource to the
// identity linked into the binary: service.version and vcs.ref.head.revision
// come from buildinfo, never from a config field or env var.
//
// It injects sentinels no real build can produce rather than comparing against
// buildinfo.Get(), which would compare telemetryConfig's answer to the same
// process global it read — green even if the fields were hardcoded (under
// `make test-unit`, GOFLAGS=-buildvcs=false, both sides are "dev"/"unknown").
//
// Sequential on purpose: buildinfo.Set mutates process-global state. Go runs
// every top-level sequential test to completion (cleanups included) before
// releasing the package's t.Parallel() tests, so the parallel tests here never
// observe the sentinels.
func TestTelemetryConfig_CarriesCompiledIdentity(t *testing.T) {
	const (
		sentinelVersion  = "9.9.9-rvi"
		sentinelRevision = "0123456789abcdef0123456789abcdef01234567"
	)

	buildinfo.Set(buildinfo.Build{Version: sentinelVersion, Revision: sentinelRevision})

	t.Cleanup(func() {
		// An empty Build clears the injection: buildinfo only overrides a
		// field when the injected value is non-empty.
		buildinfo.Set(buildinfo.Build{})
	})

	tc := telemetryConfig(&Config{
		OtelServiceName: "tracer",
		OtelLibraryName: "tracer",
	}, libLog.NewNop())

	assert.Equal(t, "tracer", tc.ServiceName)
	assert.Equal(t, sentinelVersion, tc.ServiceVersion, "service.version must carry the injected version")
	assert.Equal(t, sentinelRevision, tc.ServiceRevision, "vcs.ref.head.revision must carry the injected revision")
}
