// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import (
	"testing"
)

func TestErrMountNotFound_Sentinel(t *testing.T) {
	if ErrMountNotFound == nil {
		t.Fatal("expected ErrMountNotFound to be non-nil")
	}

	if got, want := ErrMountNotFound.Error(), "vault transit mount not found"; got != want {
		t.Errorf("unexpected error message: got %q, want %q", got, want)
	}
}
