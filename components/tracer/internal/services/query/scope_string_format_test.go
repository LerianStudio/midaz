// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestFormatScopeString_RendersTheParenthesizedShape pins the shape published as
// the scope field on a limit decision, because that shape is what the API
// reference's example advertises.
//
// The example used to read "account:<uuid>" with no parentheses. Every scoped
// rendering is parenthesized, and several scopes join with " OR ", so a reader
// who wrote a parser against the advertised form mishandled every scoped limit.
// Only the unscoped case is a bare word.
func TestFormatScopeString_RendersTheParenthesizedShape(t *testing.T) {
	account := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	segment := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	cardType := model.TransactionType("CARD")

	tests := []struct {
		name   string
		scopes []model.Scope
		want   string
	}{
		{
			name:   "unscoped limit is the only bare form",
			scopes: nil,
			want:   "global",
		},
		{
			name:   "one account is parenthesized",
			scopes: []model.Scope{{AccountID: &account}},
			want:   "(account:11111111-1111-1111-1111-111111111111)",
		},
		{
			name:   "fields in one group are comma-joined inside the parentheses",
			scopes: []model.Scope{{SegmentID: &segment, TransactionType: &cardType}},
			want:   "(segment:22222222-2222-2222-2222-222222222222,transactionType:CARD)",
		},
		{
			name:   "alternative groups join with OR",
			scopes: []model.Scope{{AccountID: &account}, {SegmentID: &segment}},
			want:   "(account:11111111-1111-1111-1111-111111111111) OR (segment:22222222-2222-2222-2222-222222222222)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, formatScopeString(tt.scopes),
				"the published example for this field must match what the field actually carries")
		})
	}
}
