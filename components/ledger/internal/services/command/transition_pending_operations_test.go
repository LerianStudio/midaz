// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
)

func TestMergeTransactionOperations(t *testing.T) {
	priorOne := &operation.Operation{ID: "prior-1"}
	priorTwo := &operation.Operation{ID: "prior-2"}
	nextOne := &operation.Operation{ID: "next-1"}

	tests := []struct {
		name  string
		prior []*operation.Operation
		next  []*operation.Operation
		want  []*operation.Operation
	}{
		{
			name:  "appends new operations after prior operations",
			prior: []*operation.Operation{priorOne, priorTwo},
			next:  []*operation.Operation{nextOne},
			want:  []*operation.Operation{priorOne, priorTwo, nextOne},
		},
		{
			name:  "deduplicates by id while preserving first occurrence",
			prior: []*operation.Operation{priorOne, priorTwo},
			next:  []*operation.Operation{{ID: priorTwo.ID}, nextOne, {ID: priorOne.ID}},
			want:  []*operation.Operation{priorOne, priorTwo, nextOne},
		},
		{
			name:  "skips nil operations without disturbing stable order",
			prior: []*operation.Operation{nil, priorOne},
			next:  []*operation.Operation{nil, nextOne},
			want:  []*operation.Operation{priorOne, nextOne},
		},
		{
			name: "accepts nil prior operations",
			next: []*operation.Operation{nextOne},
			want: []*operation.Operation{nextOne},
		},
		{
			name:  "accepts nil next operations",
			prior: []*operation.Operation{priorOne},
			want:  []*operation.Operation{priorOne},
		},
		{
			name: "returns an empty slice for empty inputs",
			want: []*operation.Operation{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeTransactionOperations(tt.prior, tt.next)

			assert.Equal(t, tt.want, got)
		})
	}
}
