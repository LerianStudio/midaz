// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferLegacyDirectionFromType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"DEBIT maps to debit", constant.DEBIT, constant.DirectionDebit},
		{"CREDIT maps to credit", constant.CREDIT, constant.DirectionCredit},
		{"ON_HOLD maps to debit", constant.ONHOLD, constant.DirectionDebit},
		{"RELEASE maps to credit", constant.RELEASE, constant.DirectionCredit},
		{"OVERDRAFT does not infer", constant.OVERDRAFT, ""},
		{"unknown does not infer", "WAT", ""},
		{"empty does not infer", "", ""},
		{"lowercase debit maps defensively", "debit", constant.DirectionDebit},
		{"mixed case On_Hold maps defensively", "On_Hold", constant.DirectionDebit},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, inferLegacyDirectionFromType(tt.input))
		})
	}
}

func TestOperationPostgreSQLModel_ToEntity_DirectionFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		modelType     string
		modelDir      string
		wantDirection string
	}{
		{"legacy DEBIT infers debit", constant.DEBIT, "", constant.DirectionDebit},
		{"legacy CREDIT infers credit", constant.CREDIT, "", constant.DirectionCredit},
		{"legacy ON_HOLD infers debit", constant.ONHOLD, "", constant.DirectionDebit},
		{"legacy RELEASE infers credit", constant.RELEASE, "", constant.DirectionCredit},
		{"legacy OVERDRAFT cannot infer", constant.OVERDRAFT, "", ""},
		{"modern debit passes through", constant.DEBIT, constant.DirectionDebit, constant.DirectionDebit},
		{"modern credit passes through", constant.CREDIT, constant.DirectionCredit, constant.DirectionCredit},
		{"modern mismatched direction passes through", constant.DEBIT, constant.DirectionCredit, constant.DirectionCredit},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := &OperationPostgreSQLModel{
				ID:              "op-1",
				TransactionID:   "tx-1",
				Type:            tt.modelType,
				Direction:       tt.modelDir,
				BalanceAffected: true,
			}

			entity := m.ToEntity()
			require.NotNil(t, entity)
			assert.Equal(t, tt.wantDirection, entity.Direction)
		})
	}
}
