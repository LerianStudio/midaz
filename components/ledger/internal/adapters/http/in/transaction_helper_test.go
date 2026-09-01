// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestBuildParentTransactionID(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when parent id is nil", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, buildParentTransactionID(uuid.Nil))
	})

	t.Run("returns string pointer when parent id exists", func(t *testing.T) {
		t.Parallel()

		parentID := uuid.Must(libCommons.GenerateUUIDv7())
		result := buildParentTransactionID(parentID)

		require.NotNil(t, result)
		assert.Equal(t, parentID.String(), *result)
	})
}

func TestGetAliasWithoutKey(t *testing.T) {
	t.Parallel()

	input := []string{
		"origin#asset-freeze",
		"destination",
		"third#asset#extra",
	}

	result := getAliasWithoutKey(input)

	assert.Equal(t, []string{"origin", "destination", "third"}, result)
}

func TestApplyDefaultBalanceKeys(t *testing.T) {
	t.Parallel()

	entries := []mtransaction.FromTo{
		{AccountAlias: "@origin", BalanceKey: ""},
		{AccountAlias: "@destination", BalanceKey: "custom-key"},
	}

	mtransaction.ApplyDefaultBalanceKeys(entries)

	assert.Equal(t, constant.DefaultBalanceKey, entries[0].BalanceKey)
	assert.Equal(t, "custom-key", entries[1].BalanceKey)
}

func TestBuildBalanceOperations(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("ad0032e5-ccf5-45f4-a3b2-12045e71b38a")
	ledgerID := uuid.MustParse("5d8ac48a-af68-4544-9bf8-80c3cc0715f4")

	t.Run("builds operations from From entries", func(t *testing.T) {
		t.Parallel()

		fromAmount := mtransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromFloat(50),
			Operation: constant.DEBIT,
		}

		validate := &mtransaction.Responses{
			Aliases: []string{"alias1#default"},
			From: map[string]mtransaction.Amount{
				"0#alias1#default": fromAmount,
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Key:            "default",
				Available:      decimal.NewFromFloat(100),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "USD",
			},
		}

		ops := buildBalanceOperations(context.Background(), organizationID, ledgerID, validate, balances)
		require.Len(t, ops, 1)
		assert.Equal(t, "0#alias1#default", ops[0].Alias)
		assert.Equal(t, constant.DEBIT, ops[0].Amount.Operation)
	})

	t.Run("builds double-entry split operations", func(t *testing.T) {
		t.Parallel()

		fromAmount := mtransaction.Amount{
			Asset:                  "USD",
			Value:                  decimal.NewFromFloat(50),
			Operation:              constant.ONHOLD,
			TransactionType:        constant.PENDING,
			RouteValidationEnabled: true,
		}

		validate := &mtransaction.Responses{
			Aliases: []string{"alias1#default"},
			From: map[string]mtransaction.Amount{
				"0#alias1#default": fromAmount,
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Key:            "default",
				Available:      decimal.NewFromFloat(100),
				OnHold:         decimal.NewFromFloat(50),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "USD",
			},
		}

		ops := buildBalanceOperations(context.Background(), organizationID, ledgerID, validate, balances)
		require.Len(t, ops, 2)
		assert.Equal(t, ops[0].Alias, ops[1].Alias, "both operations should reference the same alias")
	})

	t.Run("sorts operations by internal key", func(t *testing.T) {
		t.Parallel()

		validate := &mtransaction.Responses{
			Aliases: []string{"zeta#default", "alpha#default"},
			From: map[string]mtransaction.Amount{
				"0#zeta#default": {
					Asset:     "USD",
					Value:     decimal.NewFromFloat(10),
					Operation: constant.DEBIT,
				},
			},
			To: map[string]mtransaction.Amount{
				"0#alpha#default": {
					Asset:     "USD",
					Value:     decimal.NewFromFloat(10),
					Operation: constant.CREDIT,
				},
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:    uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias: "zeta",
				Key:   "default",
			},
			{
				ID:    uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias: "alpha",
				Key:   "default",
			},
		}

		ops := buildBalanceOperations(context.Background(), organizationID, ledgerID, validate, balances)
		require.Len(t, ops, 2)

		// Verify sorted by internal key
		for i := 0; i < len(ops)-1; i++ {
			assert.True(t, ops[i].InternalKey <= ops[i+1].InternalKey,
				"operations should be sorted by internal key")
		}
	})
}
