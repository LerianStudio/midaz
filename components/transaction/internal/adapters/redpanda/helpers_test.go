// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSeedBrokers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: []string{}},
		{name: "single", raw: "localhost:9092", want: []string{"localhost:9092"}},
		{name: "multiple with spaces", raw: "localhost:9092, localhost:9093, ", want: []string{"localhost:9092", "localhost:9093"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ParseSeedBrokers(tt.raw))
		})
	}
}
