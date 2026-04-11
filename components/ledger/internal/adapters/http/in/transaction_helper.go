// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"sort"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// transactionPathParams holds the IDs extracted from URL path parameters.
// TransactionID is uuid.Nil when the route has no :transaction_id segment.
type transactionPathParams struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	TransactionID  uuid.UUID
}

// readPathParams extracts organization, ledger, and (optional) transaction
// IDs from Fiber locals populated by the UUID-parsing middleware.
func readPathParams(c *fiber.Ctx) (*transactionPathParams, error) {
	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return nil, err
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return nil, err
	}

	transactionID := uuid.Nil
	if c.Locals("transaction_id") != nil {
		transactionID, err = http.GetUUIDFromLocals(c, "transaction_id")
		if err != nil {
			return nil, err
		}
	}

	return &transactionPathParams{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		TransactionID:  transactionID,
	}, nil
}

// buildParentTransactionID converts a parent UUID to a string pointer,
// returning nil when the parent is uuid.Nil (no parent).
func buildParentTransactionID(parentID uuid.UUID) *string {
	if parentID == uuid.Nil {
		return nil
	}

	s := parentID.String()

	return &s
}

// getAliasWithoutKey strips the "#key" suffix from alias strings,
// returning only the alias portion before the first "#".
func getAliasWithoutKey(array []string) []string {
	result := make([]string, len(array))

	for i, str := range array {
		parts := strings.Split(str, "#")
		result[i] = parts[0]
	}

	return result
}

// balanceRef holds a pre-computed balance reference for O(1) lookup by aliasKey.
type balanceRef struct {
	balance     *mmodel.Balance
	internalKey string
}

// buildBalanceOperations constructs and sorts balance operations from the
// validated transaction entries. This is pure logic with no I/O dependencies.
// Operations are sorted by internal key to prevent deadlocks in the Lua script.
//
// Alias format arriving from the DSL parser: "index#alias#balanceKey"
// (e.g. "0#@sender#default", "1#@sender#default" for same account appearing twice).
// SplitAliasWithKey strips the index prefix, returning "alias#balanceKey" for balance lookup.
func buildBalanceOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, validate *mtransaction.Responses, balances []*mmodel.Balance) []mmodel.BalanceOperation {
	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)

	// Index balances by aliasKey for O(1) lookup instead of O(balances * entries).
	balanceByAliasKey := make(map[string]balanceRef, len(balances))

	for _, b := range balances {
		key := b.Key
		if key == "" {
			key = constant.DefaultBalanceKey
		}

		aliasKey := b.Alias + "#" + key

		balanceByAliasKey[aliasKey] = balanceRef{
			balance:     b,
			internalKey: utils.BalanceInternalKey(organizationID, ledgerID, aliasKey),
		}
	}

	logger.Log(ctx, libLog.LevelDebug, "Building balance operations",
		libLog.Int("balances_indexed", len(balanceByAliasKey)),
		libLog.Int("from_entries", len(validate.From)),
		libLog.Int("to_entries", len(validate.To)))

	ops := make([]mmodel.BalanceOperation, 0, len(validate.From)+len(validate.To))

	for alias, amount := range validate.From {
		resolvedKey := mtransaction.SplitAliasWithKey(alias)

		ref, ok := balanceByAliasKey[resolvedKey]
		if !ok {
			logger.Log(ctx, libLog.LevelDebug, "From entry has no matching balance, skipping",
				libLog.String("raw_alias", alias),
				libLog.String("alias_balance", resolvedKey))

			continue
		}

		logger.Log(ctx, libLog.LevelDebug, "Matched From entry to balance",
			libLog.String("raw_alias", alias),
			libLog.String("alias_balance", resolvedKey),
			libLog.String("direction", amount.Direction),
			libLog.String("operation", amount.Operation),
			libLog.Bool("double_entry", mtransaction.IsDoubleEntrySource(amount)))

		if mtransaction.IsDoubleEntrySource(amount) {
			op1, op2 := mtransaction.SplitDoubleEntryOps(amount)

			ops = append(ops,
				mmodel.BalanceOperation{Balance: ref.balance, Alias: alias, Amount: op1, InternalKey: ref.internalKey},
				mmodel.BalanceOperation{Balance: ref.balance, Alias: alias, Amount: op2, InternalKey: ref.internalKey},
			)
		} else {
			ops = append(ops, mmodel.BalanceOperation{Balance: ref.balance, Alias: alias, Amount: amount, InternalKey: ref.internalKey})
		}
	}

	for alias, amount := range validate.To {
		resolvedKey := mtransaction.SplitAliasWithKey(alias)

		ref, ok := balanceByAliasKey[resolvedKey]
		if !ok {
			logger.Log(ctx, libLog.LevelDebug, "To entry has no matching balance, skipping",
				libLog.String("raw_alias", alias),
				libLog.String("alias_balance", resolvedKey))

			continue
		}

		logger.Log(ctx, libLog.LevelDebug, "Matched To entry to balance",
			libLog.String("raw_alias", alias),
			libLog.String("alias_balance", resolvedKey),
			libLog.String("direction", amount.Direction))

		ops = append(ops, mmodel.BalanceOperation{Balance: ref.balance, Alias: alias, Amount: amount, InternalKey: ref.internalKey})
	}

	sort.Slice(ops, func(i, j int) bool {
		return ops[i].InternalKey < ops[j].InternalKey
	})

	logger.Log(ctx, libLog.LevelDebug, "Balance operations built",
		libLog.Int("total_ops", len(ops)))

	return ops
}
