// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := RegisterAll(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register pongo2 filters and tags: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}
