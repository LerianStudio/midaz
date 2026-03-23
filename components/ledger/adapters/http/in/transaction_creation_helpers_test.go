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
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
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

func TestTransactionHandler_ApplyDefaultBalanceKeys(t *testing.T) {
	t.Parallel()

	handler := &TransactionHandler{}

	entries := []pkgTransaction.FromTo{
		{AccountAlias: "@origin", BalanceKey: ""},
		{AccountAlias: "@destination", BalanceKey: "custom-key"},
	}

	handler.ApplyDefaultBalanceKeys(entries)

	assert.Equal(t, constant.DefaultBalanceKey, entries[0].BalanceKey)
	assert.Equal(t, "custom-key", entries[1].BalanceKey)
}

func TestReadTransactionScope(t *testing.T) {
	t.Parallel()

	t.Run("returns scope with parent id when present", func(t *testing.T) {
		t.Parallel()

		organizationID := uuid.New()
		ledgerID := uuid.New()
		parentID := uuid.New()

		app := fiber.New()
		app.Get("/scope", func(c *fiber.Ctx) error {
			c.SetUserContext(context.Background())
			c.Locals("organization_id", organizationID)
			c.Locals("ledger_id", ledgerID)
			c.Locals("transaction_id", parentID)

			scope, err := readTransactionScope(c)
			if err != nil {
				return err
			}

			return c.JSON(scope)
		})

		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/scope", nil))
		require.NoError(t, err)
		defer resp.Body.Close()

		var scope transactionScope
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&scope))

		assert.Equal(t, organizationID, scope.OrganizationID)
		assert.Equal(t, ledgerID, scope.LedgerID)
		assert.Equal(t, parentID, scope.ParentID)
	})

	t.Run("returns nil parent id when transaction id is absent", func(t *testing.T) {
		t.Parallel()

		organizationID := uuid.New()
		ledgerID := uuid.New()

		app := fiber.New()
		app.Get("/scope", func(c *fiber.Ctx) error {
			c.Locals("organization_id", organizationID)
			c.Locals("ledger_id", ledgerID)

			scope, err := readTransactionScope(c)
			if err != nil {
				return err
			}

			return c.JSON(scope)
		})

		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/scope", nil))
		require.NoError(t, err)
		defer resp.Body.Close()

		var scope transactionScope
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&scope))

		assert.Equal(t, organizationID, scope.OrganizationID)
		assert.Equal(t, ledgerID, scope.LedgerID)
		assert.Equal(t, uuid.Nil, scope.ParentID)
	})
}
