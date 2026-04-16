// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"testing"
)

// TestMain sets a valid AUTHORIZER_WAL_HMAC_KEY default for the duration of
// the bootstrap test suite so the many TestLoadConfig_* tests don't each have
// to set it themselves. Tests that specifically want to exercise the HMAC-key
// validation path override this with t.Setenv() before calling LoadConfig().
func TestMain(m *testing.M) {
	if _, ok := os.LookupEnv("AUTHORIZER_WAL_HMAC_KEY"); !ok {
		_ = os.Setenv("AUTHORIZER_WAL_HMAC_KEY", "BootstrapTestHMACKey32BytesLong1")
	}

	os.Exit(m.Run())
}
