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
// goroutines are spawned by dependencies the CRM bootstrap package pulls in — the
// tenant-manager in-memory cache cleanup loop and the fasthttp server-date updater
// (via Fiber) — and are not leaks in the code under test. goleak's default filter
// already excludes testing and goleak internals, so those are not listed here.
func goleakIgnores() []goleak.Option {
	return []goleak.Option{
		goleak.IgnoreTopFunction("github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/cache.(*InMemoryCache).cleanupLoop"),
		goleak.IgnoreAnyFunction("github.com/valyala/fasthttp.updateServerDate.func1"),
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
