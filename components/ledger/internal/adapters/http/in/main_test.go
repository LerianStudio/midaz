// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"os"
	"testing"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TestMain puts this package's tests in the mode the ledger binary actually runs
// in. NewUnifiedServer calls DisableHighStatusScrub at construction, but these
// tests build their own Fiber apps and never reach bootstrap, so without this they
// would assert a >=500 body no deployed ledger produces.
func TestMain(m *testing.M) {
	pkgHTTP.DisableHighStatusScrub()

	os.Exit(m.Run())
}
