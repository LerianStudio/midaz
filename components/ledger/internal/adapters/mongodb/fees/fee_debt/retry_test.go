// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee_debt

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestRetryDuplicateKeyOnce(t *testing.T) {
	t.Parallel()

	duplicate := mongo.WriteException{WriteErrors: []mongo.WriteError{{Code: 11000}}}
	down := errors.New("connection reset")

	tests := []struct {
		name      string
		results   []error
		wantErr   error
		wantCalls int
	}{
		{name: "applied first time", results: []error{nil}, wantCalls: 1},
		{name: "concurrent insert then applied", results: []error{duplicate, nil}, wantCalls: 2},
		{name: "second duplicate means already applied", results: []error{duplicate, duplicate}, wantCalls: 2},
		{name: "failure is returned without retry", results: []error{down}, wantErr: down, wantCalls: 1},
		{name: "failure on the retry is returned", results: []error{duplicate, down}, wantErr: down, wantCalls: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			err := retryDuplicateKeyOnce(func() error {
				calls++

				return tt.results[calls-1]
			})

			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantCalls, calls)
		})
	}
}
