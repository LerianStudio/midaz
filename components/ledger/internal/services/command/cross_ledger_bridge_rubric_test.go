// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const (
	rubricTestClientRoute = "77777777-7777-4777-8777-777777777777"
	rubricTestBridgeRoute = "88888888-8888-4888-8888-888888888888"
)

// bridgeRubricCache is the cache of a transaction route with one client route
// per side and a pure bridge route, as ToCache builds it.
func bridgeRubricCache() *mmodel.TransactionRouteCache {
	client := mmodel.OperationRouteCache{
		OperationType: constant.OperationRouteTypeBidirectional,
		AccountingEntries: &mmodel.AccountingEntries{
			Direct: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "DIRECT-D", Description: "Direct debit"},
				Credit: &mmodel.AccountingRubric{Code: "DIRECT-C", Description: "Direct credit"},
			},
			Commit: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "COMMIT-D", Description: "Commit debit"},
				Credit: &mmodel.AccountingRubric{Code: "COMMIT-C", Description: "Commit credit"},
			},
		},
	}
	bridge := mmodel.OperationRouteCache{
		OperationType: constant.OperationRouteTypeBidirectional,
		AccountingEntries: &mmodel.AccountingEntries{CrossLedger: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "XL-IN", Description: "Arriving from another ledger"},
			Credit: &mmodel.AccountingRubric{Code: "XL-OUT", Description: "Leaving to another ledger"},
		}},
	}

	return &mmodel.TransactionRouteCache{Actions: map[string]mmodel.ActionRouteCache{
		constant.ActionDirect:      {Bidirectional: map[string]mmodel.OperationRouteCache{rubricTestClientRoute: client}},
		constant.ActionCommit:      {Bidirectional: map[string]mmodel.OperationRouteCache{rubricTestClientRoute: client}},
		constant.ActionCrossLedger: {Bidirectional: map[string]mmodel.OperationRouteCache{rubricTestBridgeRoute: bridge}},
	}}
}

func TestTranslateEngineTransaction_BridgeLegResolvesTheCrossLedgerRubric(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	clientRoute := rubricTestClientRoute
	bridgeRoute := rubricTestBridgeRoute

	translate := func(t *testing.T, action, status string, from, to mtransaction.FromTo) []OperationRecordSpec {
		t.Helper()

		amount := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), TransactionType: status, RouteValidationEnabled: true}
		_, projection, err := TranslateEngineTransaction(EngineTranslationInput{
			TransactionID: transactionID, Action: action, TransactionStatus: status, RouteValidationEnabled: true,
			TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
				Asset:      "USD",
				Source:     mtransaction.Source{From: []mtransaction.FromTo{from}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{to}},
			}},
			Validate: &mtransaction.Responses{
				From:                map[string]mtransaction.Amount{from.AccountAlias: amount},
				To:                  map[string]mtransaction.Amount{to.AccountAlias: amount},
				OperationRoutesFrom: map[string]string{from.AccountAlias: *from.RouteID},
				OperationRoutesTo:   map[string]string{to.AccountAlias: *to.RouteID},
			},
			Balances: []*mmodel.Balance{
				translationBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", mtransaction.SplitAlias(from.AccountAlias), "default"),
				translationBalance(organizationID, ledgerID, "55555555-5555-4555-8555-555555555555", mtransaction.SplitAlias(to.AccountAlias), "default"),
			},
			RouteCache: bridgeRubricCache(),
		})
		require.NoError(t, err)

		return projection
	}

	t.Run("the origin part credits the bridge with the leaving rubric", func(t *testing.T) {
		t.Parallel()

		projection := translate(t, constant.ActionDirect, constant.CREATED,
			mtransaction.FromTo{AccountAlias: "0#@sender#default", IsFrom: true, RouteID: &clientRoute},
			mtransaction.FromTo{AccountAlias: "0#@external/USD#default", RouteID: &bridgeRoute})

		require.Len(t, projection, 2)
		assert.Equal(t, "DIRECT-D", projection[0].RouteCode, "a client leg keeps the transaction action's rubric")
		assert.Equal(t, constant.DirectionCredit, projection[1].Direction)
		assert.Equal(t, "XL-OUT", projection[1].RouteCode)
		assert.Equal(t, "Leaving to another ledger", projection[1].RouteDescription)
		assert.Equal(t, bridgeRoute, *projection[1].RouteID)
	})

	t.Run("the destination part debits the bridge with the arriving rubric", func(t *testing.T) {
		t.Parallel()

		projection := translate(t, constant.ActionDirect, constant.CREATED,
			mtransaction.FromTo{AccountAlias: "0#@external/USD#default", IsFrom: true, RouteID: &bridgeRoute},
			mtransaction.FromTo{AccountAlias: "0#@receiver#default", RouteID: &clientRoute})

		require.Len(t, projection, 2)
		assert.Equal(t, constant.DirectionDebit, projection[0].Direction)
		assert.Equal(t, "XL-IN", projection[0].RouteCode)
		assert.Equal(t, "DIRECT-C", projection[1].RouteCode)
	})

	t.Run("a committed origin still credits the bridge with the leaving rubric", func(t *testing.T) {
		t.Parallel()

		projection := translate(t, constant.ActionCommit, constant.APPROVED,
			mtransaction.FromTo{AccountAlias: "0#@sender#default", IsFrom: true, RouteID: &clientRoute},
			mtransaction.FromTo{AccountAlias: "0#@external/USD#default", RouteID: &bridgeRoute})

		require.Len(t, projection, 2)
		assert.Equal(t, "COMMIT-D", projection[0].RouteCode)
		assert.Equal(t, "XL-OUT", projection[1].RouteCode)
	})
}

func TestResolveRouteCodesFromCache_BridgeOperationResolvesTheCrossLedgerRubric(t *testing.T) {
	t.Parallel()

	clientRoute := rubricTestClientRoute
	bridgeRoute := rubricTestBridgeRoute

	client := &operation.Operation{ID: "client", RouteID: &clientRoute, Direction: constant.DirectionDebit, BalanceKey: constant.DefaultBalanceKey}
	leaving := &operation.Operation{ID: "leaving", RouteID: &bridgeRoute, Direction: constant.DirectionCredit, BalanceKey: constant.DefaultBalanceKey}
	arriving := &operation.Operation{ID: "arriving", RouteID: &bridgeRoute, Direction: constant.DirectionDebit, BalanceKey: constant.DefaultBalanceKey}

	resolveRouteCodesFromCache([]*operation.Operation{client, leaving, arriving}, bridgeRubricCache(), constant.ActionDirect, "")

	require.NotNil(t, client.RouteCode)
	assert.Equal(t, "DIRECT-D", *client.RouteCode)
	require.NotNil(t, leaving.RouteCode)
	assert.Equal(t, "XL-OUT", *leaving.RouteCode)
	require.NotNil(t, arriving.RouteCode)
	assert.Equal(t, "XL-IN", *arriving.RouteCode)
	require.NotNil(t, arriving.RouteDescription)
	assert.Equal(t, "Arriving from another ledger", *arriving.RouteDescription)
}

func TestResolveAccountingRubric_CrossLedger(t *testing.T) {
	t.Parallel()

	entries := &mmodel.AccountingEntries{CrossLedger: &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "XL-IN"},
		Credit: &mmodel.AccountingRubric{Code: "XL-OUT"},
	}}

	require.NotNil(t, resolveAccountingRubric(entries, constant.ActionCrossLedger, constant.DirectionDebit))
	assert.Equal(t, "XL-IN", resolveAccountingRubric(entries, constant.ActionCrossLedger, constant.DirectionDebit).Code)
	assert.Equal(t, "XL-OUT", resolveAccountingRubric(entries, constant.ActionCrossLedger, constant.DirectionCredit).Code)
	assert.Nil(t, resolveAccountingRubric(entries, constant.ActionDirect, constant.DirectionDebit))
}
