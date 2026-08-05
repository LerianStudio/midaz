// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/stretchr/testify/assert"
)

func TestResolveBalanceDirection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		explicit    string
		typeDefault string
		accountType string
		want        string
	}{
		// RF-03: explicit caller override always wins, regardless of the type
		// default or the account type.
		{
			name:        "explicit credit wins over debit type default",
			explicit:    constant.DirectionCredit,
			typeDefault: constant.DirectionDebit,
			accountType: constant.ExternalAccountType,
			want:        constant.DirectionCredit,
		},
		{
			name:        "explicit debit wins over credit type default",
			explicit:    constant.DirectionDebit,
			typeDefault: constant.DirectionCredit,
			accountType: "asset",
			want:        constant.DirectionDebit,
		},
		{
			name:        "explicit credit wins with no type default and external account",
			explicit:    constant.DirectionCredit,
			typeDefault: "",
			accountType: constant.ExternalAccountType,
			want:        constant.DirectionCredit,
		},
		{
			name:        "explicit debit wins with no type default and non-external account",
			explicit:    constant.DirectionDebit,
			typeDefault: "",
			accountType: "liability",
			want:        constant.DirectionDebit,
		},

		// RF-02: no explicit, so the type default decides.
		{
			name:        "type default credit used when no explicit",
			explicit:    "",
			typeDefault: constant.DirectionCredit,
			accountType: constant.ExternalAccountType,
			want:        constant.DirectionCredit,
		},
		{
			name:        "type default debit used when no explicit",
			explicit:    "",
			typeDefault: constant.DirectionDebit,
			accountType: "asset",
			want:        constant.DirectionDebit,
		},

		// RF-04: neither explicit nor type default set -> fall through to
		// defaultBalanceDirection (external -> debit, others -> credit).
		{
			name:        "fallback to debit for external account",
			explicit:    "",
			typeDefault: "",
			accountType: constant.ExternalAccountType,
			want:        constant.DirectionDebit,
		},
		{
			name:        "fallback to debit for external account case-insensitive",
			explicit:    "",
			typeDefault: "",
			accountType: "EXTERNAL",
			want:        constant.DirectionDebit,
		},
		{
			name:        "fallback to credit for non-external account",
			explicit:    "",
			typeDefault: "",
			accountType: "asset",
			want:        constant.DirectionCredit,
		},
		{
			name:        "fallback to credit for empty account type",
			explicit:    "",
			typeDefault: "",
			accountType: "",
			want:        constant.DirectionCredit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveBalanceDirection(tt.explicit, tt.typeDefault, tt.accountType)

			assert.Equal(t, tt.want, got)
		})
	}
}
