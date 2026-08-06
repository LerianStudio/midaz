// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"

	libObs "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libStreaming "github.com/LerianStudio/lib-streaming/v2"
	"github.com/LerianStudio/lib-streaming/v2/billing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// activeAccountMetric is the Lago billable metric code emitted once per unique
// internal account that participated in an approved transaction.
const activeAccountMetric = "active_account"

// SendActiveAccountBillingEvents emits one billing_recorded event per unique
// internal account that participated in an approved transaction, encoding each
// payload through the billing serializer's Confluent-Protobuf wire format.
//
// Fire-and-forget: a nil emitter or nil serializer means billing is disabled and
// the call is a clean no-op, and serialize/emit failures are warn-logged and
// swallowed — this MUST NOT fail the parent transaction. Billing does not use
// pkgStreaming.EmitImportant/ToEmitRequest (the JSON path); it emits the raw
// Protobuf bytes directly through the Emitter seam.
func (uc *UseCase) SendActiveAccountBillingEvents(ctx context.Context, tran *transaction.Transaction) {
	logger, _, _, _ := libObs.NewTrackingFromContext(ctx)

	if uc.Streaming == nil || uc.BillingSerializer == nil {
		return
	}

	tenantID := pkgStreaming.ResolveTenantID(ctx)

	payloads := buildActiveAccountBillingPayloads(tran)

	for i := range payloads {
		// &payloads[i] rather than a copy: BillablePayload is a protobuf message
		// embedding a sync.Mutex, so copying it by value trips govet copylocks.
		p := &payloads[i]

		raw, err := uc.BillingSerializer.Serialize(p)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, "billing serialize failed; skipping emit",
				libLog.String("metric", p.GetMetric()),
				libLog.String("subscription_id", p.GetSubscriptionId()),
				libLog.String("tenant_id", tenantID),
				libLog.Err(err))

			continue
		}

		if err := uc.Streaming.Emit(ctx, libStreaming.EmitRequest{
			DefinitionKey: billing.Definition().Key,
			TenantID:      tenantID,
			Subject:       p.GetSubscriptionId(),
			Payload:       raw,
		}); err != nil {
			logger.Log(ctx, libLog.LevelWarn, "billing emit failed",
				libLog.String("metric", p.GetMetric()),
				libLog.String("subscription_id", p.GetSubscriptionId()),
				libLog.String("tenant_id", tenantID),
				libLog.Err(err))

			continue
		}
	}
}

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
