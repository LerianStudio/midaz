// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import "testing"

func TestBlockUnblockOperationTypes(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "BLOCK", got: BLOCK, want: "BLOCK"},
		{name: "UNBLOCK", got: UNBLOCK, want: "UNBLOCK"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}
