// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs the package's goroutine-leak check. This package's tests exercise
// only the SD Manager, a stub registry, and a no-op logger — none of which spawn
// the tenant-manager cache-cleanup or fasthttp date-updater goroutines that the
// component bootstrap packages must ignore. goleak's default filter already
// excludes testing and goleak internals, so no extra ignores are needed.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
