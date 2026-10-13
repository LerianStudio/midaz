//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// optionalFeeTemplate is a transaction route whose source and destination are
// required and whose fee destination is optional, on a ledger that validates
// routes.
type optionalFeeTemplate struct {
	transaction, source, destination, fee uuid.UUID
}

// seedOptionalFeeTemplate links the three routes, marking the fee link optional
// in the table. The ledger holds no cache entry for the fresh route yet, so the
// first validation loads it, flag included, from the database.
func (h *feeHarness) seedOptionalFeeTemplate(t *testing.T, operationType string, actions ...string) optionalFeeTemplate {
	t.Helper()

	postgrestestutil.SetLedgerSettings(t, h.db, h.ledgerID, map[string]any{
		"accounting": map[string]any{"validateRoutes": true},
	})

	template := optionalFeeTemplate{transaction: postgrestestutil.CreateTestTransactionRouteSimple(t, h.db, h.orgID, h.ledgerID, "transfer with optional fee")}

	sourceType, destinationType := "source", "destination"
	if operationType != "" {
		sourceType, destinationType = operationType, operationType
	}

	template.source = h.seedRouteWithEntries(t, "transfer source", sourceType, actions...)
	template.destination = h.seedRouteWithEntries(t, "transfer destination", destinationType, actions...)
	template.fee = h.seedRouteWithEntries(t, "fee revenue", destinationType, actions...)

	for _, id := range []uuid.UUID{template.source, template.destination, template.fee} {
		postgrestestutil.CreateTestOperationTransactionRouteLink(t, h.db, id, template.transaction)
	}

	res, err := h.db.Exec(`UPDATE operation_transaction_route SET optional = true WHERE transaction_route_id = $1 AND operation_route_id = $2`, template.transaction, template.fee)
	require.NoError(t, err)

	affected, err := res.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)

	return template
}

// seedRouteWithEntries creates an operation route with a debit and a credit
// rubric for each action.
func (h *feeHarness) seedRouteWithEntries(t *testing.T, title, operationType string, actions ...string) uuid.UUID {
	t.Helper()

	id := postgrestestutil.CreateTestOperationRouteSimple(t, h.db, h.orgID, h.ledgerID, title, operationType)

	entries := make([]string, 0, len(actions))
	for _, action := range actions {
		entries = append(entries, fmt.Sprintf(`"%s":{"debit":{"code":"%s-D","description":"%s"},"credit":{"code":"%s-C","description":"%s"}}`,
			action, action, title, action, title))
	}

	_, err := h.db.Exec(`UPDATE operation_route SET accounting_entries=$1::jsonb WHERE id=$2`, "{"+strings.Join(entries, ",")+"}", id)
	require.NoError(t, err)

	return id
}

func (h *feeHarness) seedOptionalFeeBalances(t *testing.T) {
	t.Helper()

	h.seedBalance(t, "@route-payer", "BRL", decimal.NewFromInt(10000), "deposit")
	h.seedBalance(t, "@route-receiver", "BRL", decimal.Zero, "deposit")
	h.seedBalance(t, "@route-fee", "BRL", decimal.Zero, "deposit")
}

// transferBody is a /v2 direct S→D of amount, with no fee leg.
func (h *feeHarness) transferBody(template optionalFeeTemplate, description, amount string) string {
	return h.v2RoutedBody(description, "BRL", amount, template.transaction,
		[]string{h.v2RoutedLeg("@route-payer", amount, template.source)},
		[]string{h.v2RoutedLeg("@route-receiver", amount, template.destination)})
}

// routeUseCase is a command use case that can write transaction routes and
// their cache, sharing the harness's repositories.
func (h *feeHarness) routeUseCase() *command.UseCase {
	return &command.UseCase{
		TransactionRouteRepo:    h.queryUC.TransactionRouteRepo,
		OperationRouteRepo:      h.queryUC.OperationRouteRepo,
		TransactionMetadataRepo: h.metaRepo,
		TransactionRedisRepo:    h.queryUC.TransactionRedisRepo,
	}
}

func TestIntegration_OptionalOperationRoutes_Transactions(t *testing.T) {
	t.Run("a fee that does not apply leaves the optional fee route unused", func(t *testing.T) {
		h := setupFeeHarness(t)
		template := h.seedOptionalFeeTemplate(t, "", "direct")
		h.seedOptionalFeeBalances(t)

		fee := flatFee("route_fee", "@route-fee", "5", false)
		fee.routeTo = routeString(template.fee)
		h.seedPackage(t, packageSpec{label: "small_transfers", maxAmount: decimal.NewFromInt(1000), fees: []feeSpec{fee}})

		resp := h.createV2Direct(t, h.newV2App(), h.transferBody(template, "out of the package range", "5000"), nil)
		require.Equalf(t, 201, resp.status, "a transfer whose fee does not apply must post: %s", string(resp.rawBody))

		legs := loadLegs(t, h.db, mustTxID(t, resp))
		require.Len(t, legs, 2)
		requireLeg(t, legs, "@route-payer", "DEBIT", "5000", template.source, "default")
		requireLeg(t, legs, "@route-receiver", "CREDIT", "5000", template.destination, "default")
		assertLiveBalance(t, h, "@route-fee", "default", "0")
	})

	t.Run("a fee that applies posts on the optional fee route", func(t *testing.T) {
		h := setupFeeHarness(t)
		template := h.seedOptionalFeeTemplate(t, "", "direct")
		h.seedOptionalFeeBalances(t)

		fee := flatFee("route_fee", "@route-fee", "5", false)
		fee.routeTo = routeString(template.fee)
		h.seedPackage(t, packageSpec{label: "every_transfer", fees: []feeSpec{fee}})

		resp := h.createV2Direct(t, h.newV2App(), h.transferBody(template, "fee applies", "100"), nil)
		require.Equalf(t, 201, resp.status, "a transfer whose fee applies must post: %s", string(resp.rawBody))

		legs := loadLegs(t, h.db, mustTxID(t, resp))
		require.Len(t, legs, 4)
		requireBalanced(t, legs, "fee on the optional route")
		requireLeg(t, legs, "@route-fee", "CREDIT", "5", template.fee, "default")
		assertLiveBalance(t, h, "@route-fee", "default", "5")
	})

	t.Run("a /v1 transfer may leave the optional route unused", func(t *testing.T) {
		h := setupFeeHarness(t)
		template := h.seedOptionalFeeTemplate(t, "", "direct")
		h.seedOptionalFeeBalances(t)

		body := `{"routeId":"` + template.transaction.String() + `","send":{"asset":"BRL","value":"100",` +
			`"source":{"from":[{"accountAlias":"@route-payer","amount":{"asset":"BRL","value":"100"},"routeId":"` + template.source.String() + `"}]},` +
			`"distribute":{"to":[{"accountAlias":"@route-receiver","amount":{"asset":"BRL","value":"100"},"routeId":"` + template.destination.String() + `"}]}}}`

		resp := h.createJSON(t, h.newApp(), body, nil)
		require.Equalf(t, 201, resp.status, "a /v1 transfer without the optional leg must post: %s", string(resp.rawBody))

		legs := loadLegs(t, h.db, mustTxID(t, resp))
		require.Len(t, legs, 2)
	})

	t.Run("a missing required route is refused even when the optional one is used", func(t *testing.T) {
		h := setupFeeHarness(t)
		template := h.seedOptionalFeeTemplate(t, "", "direct")
		h.seedOptionalFeeBalances(t)

		body := h.v2RoutedBody("no destination leg", "BRL", "100", template.transaction,
			[]string{h.v2RoutedLeg("@route-payer", "100", template.source)},
			[]string{h.v2RoutedLeg("@route-fee", "100", template.fee)})

		resp := h.createV2Direct(t, h.newV2App(), body, nil)
		require.Equalf(t, 422, resp.status, "a transfer without the required destination must be refused: %s", string(resp.rawBody))
		assert.Equal(t, "0116", resp.body["code"])
		assertLiveBalance(t, h, "@route-payer", "default", "10000")
		assertLiveBalance(t, h, "@route-fee", "default", "0")
	})

	t.Run("a hold and its commit leave the optional route unused", func(t *testing.T) {
		h := setupFeeHarness(t)
		template := h.seedOptionalFeeTemplate(t, "", "direct", "hold", "commit", "cancel")
		h.seedOptionalFeeBalances(t)

		app := h.newV2App()

		hold := h.createV2Hold(t, app, h.transferBody(template, "held transfer", "100"), nil)
		require.Equalf(t, 201, hold.status, "a hold without the optional leg must be accepted: %s", string(hold.rawBody))

		commit := h.post(t, app, h.v2StatePath(mustTxID(t, hold), "commit"), `{}`, nil)
		require.Equalf(t, 201, commit.status, "its commit must be accepted: %s", string(commit.rawBody))

		assertLiveBalance(t, h, "@route-receiver", "default", "100")
	})

	t.Run("a revert leaves the optional route unused", func(t *testing.T) {
		h := setupFeeHarness(t)
		template := h.seedOptionalFeeTemplate(t, "bidirectional", "direct", "revert")
		h.seedOptionalFeeBalances(t)

		app := h.newV2App()

		created := h.createV2Direct(t, app, h.transferBody(template, "reverted transfer", "100"), nil)
		require.Equalf(t, 201, created.status, "the transfer must post: %s", string(created.rawBody))

		reverted := h.post(t, app, h.v2StatePath(mustTxID(t, created), "revert"), `{}`, nil)
		require.Equalf(t, 201, reverted.status, "its revert must post without the optional leg: %s", string(reverted.rawBody))

		assertLiveBalance(t, h, "@route-payer", "default", "10000")
		assertLiveBalance(t, h, "@route-receiver", "default", "0")
	})
}

// The cache of a transaction route is rewritten from what an update returns. A
// title-only update keeps the optional flag in that rewrite, and moving the fee
// route to the required list makes a fee-less transfer incomplete again.
func TestIntegration_OptionalOperationRoutes_UpdatesRewriteTheCacheWithTheFlag(t *testing.T) {
	h := setupFeeHarness(t)
	template := h.seedOptionalFeeTemplate(t, "", "direct")
	h.seedOptionalFeeBalances(t)

	app := h.newV2App()
	uc := h.routeUseCase()

	renamed, err := uc.UpdateTransactionRoute(h.ctx(), h.orgID, template.transaction, &mmodel.UpdateTransactionRouteInput{Title: "renamed"}, command.LinksMergePatchV2)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{template.fee}, renamed.OptionalOperationRouteIDs)
	require.NoError(t, uc.CreateAccountingRouteCache(h.ctx(), renamed))

	first := h.createV2Direct(t, app, h.transferBody(template, "after the rename", "100"), nil)
	require.Equalf(t, 201, first.status, "the rename must keep the fee route optional: %s", string(first.rawBody))

	required := []uuid.UUID{template.source, template.destination, template.fee}
	none := []uuid.UUID{}

	retagged, err := uc.UpdateTransactionRoute(h.ctx(), h.orgID, template.transaction, &mmodel.UpdateTransactionRouteInput{
		OperationRoutes: &required, OptionalOperationRoutes: &none,
	}, command.LinksMergePatchV2)
	require.NoError(t, err)
	assert.Empty(t, retagged.OptionalOperationRouteIDs)
	require.NoError(t, uc.CreateAccountingRouteCache(h.ctx(), retagged))

	second := h.createV2Direct(t, app, h.transferBody(template, "after the fee became required", "100"), nil)
	require.Equalf(t, 422, second.status, "a required fee route must be used: %s", string(second.rawBody))
	assert.Equal(t, "0116", second.body["code"])
}
