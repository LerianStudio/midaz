// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming_test

import (
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/buildinfo"
	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// TestManifest_AdvertisesInjectedBuildIdentity is the only test in this package
// that injects a build identity instead of reading the one the test binary
// already has. TestNewManifestHandler_AdvertisesApplicationTopic compares
// appVersion to buildinfo.Get() evaluated in the same process, so it would
// still pass if the manifest hardcoded a constant — under `make test-unit`
// (GOFLAGS=-buildvcs=false) both sides are the unstamped fallback "dev". This
// one calls buildinfo.Set, so a hardcoded appVersion fails it.
//
// Sequential on purpose: buildinfo.Set mutates process-global state. Go runs
// every top-level sequential test to completion (cleanups included) before
// releasing the package's t.Parallel() tests, so the parallel readers here
// never observe the sentinel.
func TestManifest_AdvertisesInjectedBuildIdentity(t *testing.T) {
	const sentinelVersion = "9.9.9-rvi"

	buildinfo.Set(buildinfo.Build{Version: sentinelVersion})

	t.Cleanup(func() {
		// An empty Build clears the injection: buildinfo only overrides a
		// field when the injected value is non-empty.
		buildinfo.Set(buildinfo.Build{})
	})

	handler, err := pkgStreaming.NewManifestHandler("ledger", "ledger", sampleDefs())
	require.NoError(t, err)

	doc := serveManifest(t, handler)

	require.Equal(t, sentinelVersion, doc.Publisher.AppVersion,
		"manifest appVersion must be the identity compiled into the binary")
}
