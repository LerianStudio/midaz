// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"testing"

	"go.uber.org/goleak"
)

// goleakIgnores returns the goleak ignore options for this package. These
// goroutines originate from test infrastructure and external dependencies (the
// tenant-manager in-memory cache cleanup loop, the OpenCensus stats worker, the
// fasthttp server-date updater, and goleak/testing internals), not from the code
// under test.
func goleakIgnores() []goleak.Option {
	return []goleak.Option{
		goleak.IgnoreTopFunction("github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/cache.(*InMemoryCache).cleanupLoop"),
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreAnyFunction("github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/cache.(*InMemoryCache).cleanupLoop"),
		goleak.IgnoreAnyFunction("github.com/valyala/fasthttp.updateServerDate.func1"),
		goleak.IgnoreAnyFunction("testing.tRunner"),
		goleak.IgnoreAnyFunction("testing.tRunner.func1"),
		goleak.IgnoreAnyFunction("testing.(*T).Run"),
		goleak.IgnoreAnyFunction("testing.runTests"),
		goleak.IgnoreAnyFunction("testing.(*M).Run"),
		goleak.IgnoreAnyFunction("go.uber.org/goleak.(*opts).retry"),
		goleak.IgnoreAnyFunction("go.uber.org/goleak.Find"),
	}
}

func TestMain(m *testing.M) {
	// lib-commons v5 enforces TLS at the mongo/redis connection constructors
	// (commons.AllowInsecureTLS parses this as a bool). These bootstrap tests
	// exercise the connection builders against plaintext config, so bypass the
	// TLS gate for the package.
	os.Setenv("ALLOW_INSECURE_TLS", "true")

	goleak.VerifyTestMain(m, goleakIgnores()...)
}
