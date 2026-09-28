// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import "testing"

func TestEngineWriteBehindAuditEmission(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name                  string
		dispatched, projected bool
		want                  bool
	}{
		{name: "confirmed dispatch", dispatched: true, want: true},
		{name: "completed fallback", projected: true, want: true},
		{name: "deferred projection", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldEmitEngineWriteBehindAudit(test.dispatched, test.projected); got != test.want {
				t.Errorf("shouldEmitEngineWriteBehindAudit(%t, %t) = %t, want %t", test.dispatched, test.projected, got, test.want)
			}
		})
	}
}
