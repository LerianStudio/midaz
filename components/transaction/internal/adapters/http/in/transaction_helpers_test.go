// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

func TestGetAliasWithoutKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "empty slice",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "single alias without key",
			input: []string{"@alice"},
			want:  []string{"@alice"},
		},
		{
			name:  "single alias with key",
			input: []string{"@alice#savings"},
			want:  []string{"@alice"},
		},
		{
			name:  "mixed aliases preserves positional order",
			input: []string{"@alice#savings", "@bob", "@carol#checking", "@dave"},
			want:  []string{"@alice", "@bob", "@carol", "@dave"},
		},
		{
			name:  "alias with multiple hash segments keeps first",
			input: []string{"@alice#savings#usd"},
			want:  []string{"@alice"},
		},
		{
			name:  "alias starting with hash gives empty first part",
			input: []string{"#orphan"},
			want:  []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := getAliasWithoutKey(tt.input)
			assert.Equal(t, tt.want, got)
			// The function must return a NEW slice of the same length.
			assert.Len(t, got, len(tt.input))
		})
	}
}

func TestTransactionHandler_CheckTransactionDate(t *testing.T) {
	t.Parallel()

	var logger libLog.Logger = &libLog.NoneLogger{}

	handler := &TransactionHandler{}

	pastTD := pkgTransaction.TransactionDate(time.Now().UTC().Add(-2 * time.Hour))
	zeroTD := pkgTransaction.TransactionDate(time.Time{})
	futureTD := pkgTransaction.TransactionDate(time.Now().UTC().Add(48 * time.Hour))

	t.Run("nil TransactionDate falls back to now", func(t *testing.T) {
		t.Parallel()

		before := time.Now().UTC()
		got, err := handler.checkTransactionDate(logger, pkgTransaction.Transaction{}, "")
		after := time.Now().UTC()

		require.NoError(t, err)
		assert.False(t, got.IsZero())
		assert.False(t, got.Before(before.Add(-time.Second)))
		assert.False(t, got.After(after.Add(time.Second)))
	})

	t.Run("zero TransactionDate falls back to now", func(t *testing.T) {
		t.Parallel()

		input := pkgTransaction.Transaction{TransactionDate: &zeroTD}

		before := time.Now().UTC()
		got, err := handler.checkTransactionDate(logger, input, "")
		after := time.Now().UTC()

		require.NoError(t, err)
		assert.False(t, got.Before(before.Add(-time.Second)))
		assert.False(t, got.After(after.Add(time.Second)))
	})

	t.Run("past TransactionDate is accepted", func(t *testing.T) {
		t.Parallel()

		input := pkgTransaction.Transaction{TransactionDate: &pastTD}

		got, err := handler.checkTransactionDate(logger, input, "")
		require.NoError(t, err)
		assert.Equal(t, pastTD.Time(), got)
	})

	t.Run("future TransactionDate is rejected", func(t *testing.T) {
		t.Parallel()

		input := pkgTransaction.Transaction{TransactionDate: &futureTD}

		got, err := handler.checkTransactionDate(logger, input, "")
		require.Error(t, err)
		assert.True(t, got.IsZero())
	})

	t.Run("PENDING with TransactionDate is rejected", func(t *testing.T) {
		t.Parallel()

		input := pkgTransaction.Transaction{TransactionDate: &pastTD}

		got, err := handler.checkTransactionDate(logger, input, cn.PENDING)
		require.Error(t, err)
		assert.True(t, got.IsZero())
	})
}
