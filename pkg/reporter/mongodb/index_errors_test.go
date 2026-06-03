// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsIndexAlreadyExistsError covers the index-creation error classification
// that EnsureIndexes relies on to stay idempotent. It replaces the former
// mtest-based mock-client tests (mtest has no mongo-driver/v2 equivalent); the
// behavior under test is the pure error-string classification, not the driver
// round-trip, so it is exercised directly here.
func TestIsIndexAlreadyExistsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "IndexOptionsConflict swallowed", err: errors.New("IndexOptionsConflict: index already exists with different options"), want: true},
		{name: "already exists swallowed", err: errors.New("index already exists with a different name"), want: true},
		{name: "generic error propagated", err: errors.New("some other error"), want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsIndexAlreadyExistsError(tt.err))
		})
	}
}
