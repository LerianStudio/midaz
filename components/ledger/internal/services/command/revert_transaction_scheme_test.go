// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

// TestPrepareRevertTransaction_InheritsOriginScheme locks the reversal's scheme to the
// original's: the body column is NULL on an APPROVED row and TransactionRevert copies
// no scheme, so only the entity column can carry it onto the reversal. Both the direct
// and the group revert build their reversal through this gate.
func TestPrepareRevertTransaction_InheritsOriginScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		originScheme string
		wantScheme   string
	}{
		{name: "original declared PIX", originScheme: "PIX", wantScheme: "PIX"},
		{name: "original declared no scheme", originScheme: "", wantScheme: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			amount := decimal.NewFromInt(500)
			in := revertInput()
			origin := approvedOrigin(in, amount, newCommittedHold(amount).allRows())
			origin.Scheme = tt.originScheme

			reader := &revertIndexedOriginReader{revertReader: &revertReader{}, indexed: origin, primary: origin}
			uc := &UseCase{TransactionReader: reader}

			reversal, _, err := uc.prepareRevertTransaction(context.Background(), trace.SpanFromContext(context.Background()), in)
			require.NoError(t, err)

			assert.Equal(t, tt.wantScheme, reversal.Scheme)
		})
	}
}
