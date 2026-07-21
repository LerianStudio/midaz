// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCRMApplicationNameConstant(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "ApplicationName has correct value",
			expected: "midaz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ApplicationName,
				"ApplicationName constant must equal %q", tt.expected)
		})
	}
}
