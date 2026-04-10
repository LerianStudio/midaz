// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParentTransactionID(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when parent id is nil", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, buildParentTransactionID(uuid.Nil))
	})

	t.Run("returns string pointer when parent id exists", func(t *testing.T) {
		t.Parallel()

		parentID := uuid.New()
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

func TestReadPathParams(t *testing.T) {
	t.Parallel()

	t.Run("returns scope with transaction id when present", func(t *testing.T) {
		t.Parallel()

		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		app := fiber.New()
		app.Get("/scope", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())
			c.Locals("organization_id", organizationID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", transactionID)

			scope, err := readPathParams(c)
			if err != nil {
				return err
			}

			return c.JSON(scope)
		})

		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/scope", nil))
		require.NoError(t, err)
		defer resp.Body.Close()

		var scope transactionPathParams
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&scope))

		assert.Equal(t, organizationID, scope.OrganizationID)
		assert.Equal(t, ledgerID, scope.LedgerID)
		assert.Equal(t, transactionID, scope.TransactionID)
	})

	t.Run("returns nil transaction id when absent", func(t *testing.T) {
		t.Parallel()

		organizationID := uuid.New()
		ledgerID := uuid.New()

		app := fiber.New()
		app.Get("/scope", func(c *fiber.Ctx) error {
			c.Locals("organization_id", organizationID)
			c.Locals("ledger_id", ledgerID)

			scope, err := readPathParams(c)
			if err != nil {
				return err
			}

			return c.JSON(scope)
		})

		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/scope", nil))
		require.NoError(t, err)
		defer resp.Body.Close()

		var scope transactionPathParams
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&scope))

		assert.Equal(t, organizationID, scope.OrganizationID)
		assert.Equal(t, ledgerID, scope.LedgerID)
		assert.Equal(t, uuid.Nil, scope.TransactionID)
	})
}

func TestApplyDefaultBalanceKeys(t *testing.T) {
	t.Parallel()

	entries := []pkgTransaction.FromTo{
		{AccountAlias: "@origin", BalanceKey: ""},
		{AccountAlias: "@destination", BalanceKey: "custom-key"},
	}

	pkgTransaction.ApplyDefaultBalanceKeys(entries)

	assert.Equal(t, constant.DefaultBalanceKey, entries[0].BalanceKey)
	assert.Equal(t, "custom-key", entries[1].BalanceKey)
}

func TestBuildBalanceOperations(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("ad0032e5-ccf5-45f4-a3b2-12045e71b38a")
	ledgerID := uuid.MustParse("5d8ac48a-af68-4544-9bf8-80c3cc0715f4")

	t.Run("builds operations from From entries", func(t *testing.T) {
		t.Parallel()

		fromAmount := pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromFloat(50),
			Operation: constant.DEBIT,
		}

		validate := &pkgTransaction.Responses{
			Aliases: []string{"alias1#default"},
			From: map[string]pkgTransaction.Amount{
				"0#alias1#default": fromAmount,
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
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

		ops := buildBalanceOperations(organizationID, ledgerID, validate, balances)
		require.Len(t, ops, 1)
		assert.Equal(t, "0#alias1#default", ops[0].Alias)
		assert.Equal(t, constant.DEBIT, ops[0].Amount.Operation)
	})

	t.Run("builds double-entry split operations", func(t *testing.T) {
		t.Parallel()

		fromAmount := pkgTransaction.Amount{
			Asset:                  "USD",
			Value:                  decimal.NewFromFloat(50),
			Operation:              constant.ONHOLD,
			TransactionType:        constant.PENDING,
			RouteValidationEnabled: true,
		}

		validate := &pkgTransaction.Responses{
			Aliases: []string{"alias1#default"},
			From: map[string]pkgTransaction.Amount{
				"0#alias1#default": fromAmount,
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
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

		ops := buildBalanceOperations(organizationID, ledgerID, validate, balances)
		require.Len(t, ops, 2)
		assert.Equal(t, ops[0].Alias, ops[1].Alias, "both operations should reference the same alias")
	})

	t.Run("sorts operations by internal key", func(t *testing.T) {
		t.Parallel()

		validate := &pkgTransaction.Responses{
			Aliases: []string{"zeta#default", "alpha#default"},
			From: map[string]pkgTransaction.Amount{
				"0#zeta#default": {
					Asset:     "USD",
					Value:     decimal.NewFromFloat(10),
					Operation: constant.DEBIT,
				},
			},
			To: map[string]pkgTransaction.Amount{
				"0#alpha#default": {
					Asset:     "USD",
					Value:     decimal.NewFromFloat(10),
					Operation: constant.CREDIT,
				},
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:    uuid.New().String(),
				Alias: "zeta",
				Key:   "default",
			},
			{
				ID:    uuid.New().String(),
				Alias: "alpha",
				Key:   "default",
			},
		}

		ops := buildBalanceOperations(organizationID, ledgerID, validate, balances)
		require.Len(t, ops, 2)

		// Verify sorted by internal key
		for i := 0; i < len(ops)-1; i++ {
			assert.True(t, ops[i].InternalKey <= ops[i+1].InternalKey,
				"operations should be sorted by internal key")
		}
	})
}
