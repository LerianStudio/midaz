// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// accountTypeLeg is one operation in the fixture below, with the account type
// it was built from and the alias it carries.
type accountTypeLeg struct {
	alias       string
	accountType string
	opType      string
	direction   string
}

// The three account shapes a transaction can touch, and why each one matters:
//
//   - The per-asset account Midaz creates for itself is the only external
//     account whose alias carries constant.DefaultExternalAccountAliasPrefix.
//   - A client-created external account has type "external" and an alias
//     WITHOUT the prefix — mmodel.Account.Alias is validated with
//     prohibitedexternalaccountprefix, so a client cannot supply one. It is
//     invisible to anything that classifies accounts by alias prefix.
//   - An ordinary account is neither.
var accountTypeLegs = []accountTypeLeg{
	{alias: constant.DefaultExternalAccountAliasPrefix + "BRL", accountType: constant.ExternalAccountType, opType: constant.DEBIT, direction: constant.DirectionDebit},
	{alias: "@treasury_settlement", accountType: constant.ExternalAccountType, opType: constant.CREDIT, direction: constant.DirectionCredit},
	{alias: "@person1", accountType: "deposit", opType: constant.CREDIT, direction: constant.DirectionCredit},
}

// accountTypeTransactionFixture builds an APPROVED transaction whose operations
// cover all three account shapes.
func accountTypeTransactionFixture() *transaction.Transaction {
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	tranID := uuid.New().String()
	amount := decimal.NewFromInt(1500)
	statusCode := constant.APPROVED

	ops := make([]*operation.Operation, 0, len(accountTypeLegs))

	for _, leg := range accountTypeLegs {
		ops = append(ops, &operation.Operation{
			ID:             uuid.New().String(),
			TransactionID:  tranID,
			AccountID:      uuid.New().String(),
			AccountAlias:   leg.alias,
			AccountType:    leg.accountType,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			AssetCode:      "BRL",
			Direction:      leg.direction,
			Type:           leg.opType,
		})
	}

	return &transaction.Transaction{
		ID:             tranID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Description:    "account type fixture",
		Status:         transaction.Status{Code: statusCode, Description: &statusCode},
		Amount:         &amount,
		AssetCode:      "BRL",
		Operations:     ops,
	}
}

// emittedOperations emits the lifecycle event for tran and returns the decoded
// operations array from its payload.
func emittedOperations(t *testing.T, tran *transaction.Transaction) []map[string]any {
	t.Helper()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, mockEmitter)

	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)

	var payload struct {
		Operations []map[string]any `json:"operations"`
	}

	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	require.Len(t, payload.Operations, len(tran.Operations))

	return payload.Operations
}

// TestSendTransactionEvents_EveryOperationCarriesAccountType is the reason this
// field exists. The operation row carries accountId and accountAlias but no
// account type — Operation.Type is the DEBIT/CREDIT movement — so a consumer
// that must exclude external accounts has nothing to apply the criterion to and
// falls back to matching the alias prefix. That misses every client-created
// external account, which has the type and not the prefix.
//
// The key is on EVERY operation, external ones included: filtering external
// legs out at the source would take with them the evidence that the leg
// existed, and the consumer could no longer reconcile the transaction.
func TestSendTransactionEvents_EveryOperationCarriesAccountType(t *testing.T) {
	tran := accountTypeTransactionFixture()

	operations := emittedOperations(t, tran)

	for i, op := range operations {
		leg := accountTypeLegs[i]

		require.Containsf(t, op, "accountType",
			"operation %d (%s) must carry accountType", i, leg.alias)
		assert.Equalf(t, leg.accountType, op["accountType"],
			"operation %d (%s) must carry the account's own type", i, leg.alias)
		assert.Equalf(t, leg.opType, op["type"],
			"operation %d (%s) keeps type as the ledger movement", i, leg.alias)
	}
}

// TestSendTransactionEvents_AccountTypeSeesPrefixlessExternalAccount pins the
// reachable class the alias-prefix approximation cannot see: type "external",
// alias without constant.DefaultExternalAccountAliasPrefix.
func TestSendTransactionEvents_AccountTypeSeesPrefixlessExternalAccount(t *testing.T) {
	tran := accountTypeTransactionFixture()

	operations := emittedOperations(t, tran)

	const prefixless = 1

	require.Equal(t, "@treasury_settlement", operations[prefixless]["accountAlias"])
	assert.NotContains(t, operations[prefixless]["accountAlias"], constant.DefaultExternalAccountAliasPrefix,
		"fixture must exercise an external account whose alias lacks the prefix")
	assert.Equal(t, constant.ExternalAccountType, operations[prefixless]["accountType"],
		"the account type must expose an external account the alias prefix cannot")
}

// TestSendTransactionEvents_OperationWireIsAdditive proves the field is purely
// additive: every key the operation marshalled to before is still present,
// under the same name and with the same value, and accountType is the only
// addition. A consumer that ignores accountType reads an unchanged document.
func TestSendTransactionEvents_OperationWireIsAdditive(t *testing.T) {
	tran := accountTypeTransactionFixture()

	operations := emittedOperations(t, tran)

	for i, op := range tran.Operations {
		raw, err := json.Marshal(op)
		require.NoError(t, err)

		var before map[string]any
		require.NoError(t, json.Unmarshal(raw, &before))

		emitted := operations[i]

		for key, want := range before {
			assert.Equalf(t, want, emitted[key],
				"operation %d: key %q must reach the wire unchanged", i, key)
		}

		added := make([]string, 0, 1)

		for key := range emitted {
			if _, ok := before[key]; !ok {
				added = append(added, key)
			}
		}

		assert.Equalf(t, []string{"accountType"}, added,
			"operation %d: accountType must be the only added key", i)
	}
}

// TestSendTransactionEvents_AccountTypeKeyIsPresentWhenUnknown pins the key as
// always present, which is why it carries no omitempty. An operation can reach
// the emit without a type — an in-flight queue payload produced before the field
// existed — and the key has to be there for a consumer to tell "the producer did
// not know" apart from a real type. Under omitempty both cases collapse into
// absence.
func TestSendTransactionEvents_AccountTypeKeyIsPresentWhenUnknown(t *testing.T) {
	tran := accountTypeTransactionFixture()

	for _, op := range tran.Operations {
		op.AccountType = ""
	}

	operations := emittedOperations(t, tran)

	for i, op := range operations {
		require.Containsf(t, op, "accountType",
			"operation %d must carry accountType even when the type is unknown", i)
		assert.Emptyf(t, op["accountType"],
			"operation %d: an unknown type is the empty string, never a fabricated one", i)
	}
}
