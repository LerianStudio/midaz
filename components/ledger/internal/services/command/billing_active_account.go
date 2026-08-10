// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v2"
	"github.com/LerianStudio/lib-streaming/v2/billing"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// activeAccountMetric is the Lago billable metric code emitted once per unique
// internal account that participated in an approved transaction.
const activeAccountMetric = "active_account"

// SendActiveAccountBillingEvents emits one billing_recorded event per unique
// internal account that participated in an approved transaction, encoding each
// payload through the billing serializer's Confluent-Protobuf wire format.
//
// phase is the lifecycle phase resolved by CreateOrUpdateTransaction. A
// TransactionLifecyclePhaseNoop phase means the caller observed no state change
// (e.g. a unique-violation redelivery with no eligible status transition), so
// billing early-returns without emitting — this keeps billing at idempotency
// parity with SendTransactionEvents, which skips the same phase, preventing a
// double-emit on reprocessing.
//
// Fire-and-forget: a nil emitter or nil serializer means billing is disabled and
// the call is a clean no-op, and serialize/emit failures are span-recorded,
// warn-logged and swallowed — this MUST NOT fail the parent transaction. Billing
// does not use pkgStreaming.EmitImportant/ToEmitRequest (the JSON path); it emits
// the raw Protobuf bytes directly through the Emitter seam.
func (uc *UseCase) SendActiveAccountBillingEvents(ctx context.Context, tran *transaction.Transaction, phase string) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	if phase == TransactionLifecyclePhaseNoop {
		return
	}

	if uc.Streaming == nil || uc.BillingSerializer == nil {
		return
	}

	if ctx.Err() != nil {
		return
	}

	if tran == nil {
		return
	}

	ctxSend, span := tracer.Start(ctx, "command.send_active_account_billing_events_async")
	defer span.End()

	tenantID := pkgStreaming.ResolveTenantID(ctxSend)

	// SubscriptionId is the billing customer: the resolved tenant when
	// multi-tenant is enabled, otherwise the transaction's organization.
	subscriptionID := tran.OrganizationID
	if uc.MultiTenantEnabled {
		subscriptionID = tenantID
	}

	billables := buildActiveAccountBillingPayloads(tran, subscriptionID)

	for i := range billables {
		if ctxSend.Err() != nil {
			return
		}

		// &billables[i].Payload rather than a copy: BillablePayload is a protobuf
		// message embedding a sync.Mutex, so copying it by value trips govet
		// copylocks. The account ID is the ce-subject; SubscriptionId stays the
		// billing customer.
		uc.emitActiveAccountBillingEvent(ctxSend, span, logger, tenantID, billables[i].AccountID, &billables[i].Payload, tran.CreatedAt)
	}
}

// emitActiveAccountBillingEvent serializes one billable payload through the
// billing serializer and emits it via the streaming seam. subject is the
// ce-subject (the internal account ID), kept distinct from the payload's
// SubscriptionId (the billing customer). Serialize and emit failures are
// span-recorded, warn-logged and swallowed — billing is best-effort and MUST
// NOT fail the parent transaction.
func (uc *UseCase) emitActiveAccountBillingEvent(ctx context.Context, span trace.Span, logger libLog.Logger, tenantID, subject string, p *billing.BillablePayload, ts time.Time) {
	raw, err := uc.BillingSerializer.Serialize(p)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "billing serialize failed; skipping emit", err)

		logger.Log(ctx, libLog.LevelWarn, "billing serialize failed; skipping emit",
			libLog.String("metric", p.GetMetric()),
			libLog.String("subject", subject),
			libLog.String("subscription_id", p.GetSubscriptionId()),
			libLog.Err(err))

		return
	}

	if err := uc.Streaming.Emit(ctx, libStreaming.EmitRequest{
		DefinitionKey: billing.Definition().Key,
		TenantID:      tenantID,
		Subject:       subject,
		Timestamp:     ts,
		Payload:       raw,
	}); err != nil {
		libOpentelemetry.HandleSpanError(span, "billing emit failed", err)

		logger.Log(ctx, libLog.LevelWarn, "billing emit failed",
			libLog.String("metric", p.GetMetric()),
			libLog.String("subject", subject),
			libLog.String("subscription_id", p.GetSubscriptionId()),
			libLog.Err(err))

		return
	}
}

// buildActiveAccountBillingPayloads derives the active-account billables for a
// committed transaction. It returns one entry per unique internal account
// referenced by the transaction's operations, preserving first-seen order. Each
// payload's SubscriptionId is set to subscriptionID (the billing customer);
// account_id and transaction_id remain on Properties. External accounts (alias
// prefixed with constant.DefaultExternalAccountAliasPrefix) are excluded, and a
// transaction that is nil or not APPROVED yields nothing.
func buildActiveAccountBillingPayloads(tran *transaction.Transaction, subscriptionID string) []events.ActiveAccountBillable {
	if tran == nil || tran.Status.Code != constant.APPROVED {
		return nil
	}

	var billables []events.ActiveAccountBillable

	seen := make(map[string]struct{})

	for _, op := range tran.Operations {
		if op == nil {
			continue
		}

		if op.AccountID == "" {
			continue
		}

		if strings.HasPrefix(op.AccountAlias, constant.DefaultExternalAccountAliasPrefix) {
			continue
		}

		if _, ok := seen[op.AccountID]; ok {
			continue
		}

		seen[op.AccountID] = struct{}{}

		// Constructed inline (not copied) so the embedded sync.Mutex is never
		// moved by value; the emit loop then takes &billables[i].Payload.
		billables = append(billables, events.ActiveAccountBillable{
			AccountID: op.AccountID,
			Payload: billing.BillablePayload{
				Metric:         activeAccountMetric,
				SubscriptionId: subscriptionID,
				Properties: map[string]*billing.PropertyValue{
					"account_id":     billing.StringProperty(op.AccountID),
					"transaction_id": billing.StringProperty(tran.ID),
				},
			},
		})
	}

	return billables
}
