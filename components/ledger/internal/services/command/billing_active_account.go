// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"strings"

	"github.com/LerianStudio/lib-streaming/v2/billing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// activeAccountMetric is the Lago billable metric code emitted once per unique
// internal account that participated in an approved transaction.
const activeAccountMetric = "active_account"

// buildActiveAccountBillingPayloads derives the active-account billable payloads
// for a committed transaction. It returns one payload per unique internal
// account referenced by the transaction's operations, preserving first-seen
// order. External accounts (alias prefixed with constant.DefaultExternalAccountAliasPrefix)
// are excluded, and a transaction that is nil or not APPROVED yields nothing.
func buildActiveAccountBillingPayloads(tran *transaction.Transaction) []billing.BillablePayload {
	if tran == nil || tran.Status.Code != constant.APPROVED {
		return nil
	}

	var payloads []billing.BillablePayload

	seen := make(map[string]struct{})

	for _, op := range tran.Operations {
		if op == nil {
			continue
		}

		if strings.HasPrefix(op.AccountAlias, constant.DefaultExternalAccountAliasPrefix) {
			continue
		}

		if _, ok := seen[op.AccountID]; ok {
			continue
		}

		seen[op.AccountID] = struct{}{}

		payloads = append(payloads, billing.BillablePayload{
			Metric:         activeAccountMetric,
			SubscriptionId: op.AccountID,
			Properties: map[string]*billing.PropertyValue{
				"account_id":     billing.StringProperty(op.AccountID),
				"transaction_id": billing.StringProperty(tran.ID),
			},
		})
	}

	return payloads
}
