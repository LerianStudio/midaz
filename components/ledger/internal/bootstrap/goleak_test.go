// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// lib-commons v5.8.0 enforces TLS at the connection constructors
	// (commons.AllowInsecureTLS parses this as a bool). These bootstrap tests
	// exercise the Postgres connection builders against plaintext config, so
	// bypass the TLS gate for the package.
	os.Setenv("ALLOW_INSECURE_TLS", "true")

	goleak.VerifyTestMain(m, goleakIgnores()...)
}
