// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// transactionGroupEventKind selects which transaction_group fact a grouped
// operation publishes.
type transactionGroupEventKind string

const (
	transactionGroupEventPosted    transactionGroupEventKind = "posted"
	transactionGroupEventCommitted transactionGroupEventKind = "committed"
	transactionGroupEventCanceled  transactionGroupEventKind = "canceled"
	transactionGroupEventReverted  transactionGroupEventKind = "reverted"
)

// publishTransactionGroupEvent dispatches one transaction_group fact after a
// grouped operation has been applied and completed. Like the per-part lifecycle
// events it runs detached, with its own timeout, so broker latency never holds
// the response; build and emit failures are recorded and never returned.
//
// roleOf names each part's role. Transactions composed from engine evidence
// carry their legs and use crossLedgerGroupRole; rows read back from the
// repository carry no legs, so their caller resolves roles from the group intent.
func (uc *UseCase) publishTransactionGroupEvent(
	ctx context.Context,
	kind transactionGroupEventKind,
	groupID uuid.UUID,
	revertedGroupID *uuid.UUID,
	transactions []*transaction.Transaction,
	roleOf func(*transaction.Transaction) string,
) {
	if uc.Streaming == nil || groupID == uuid.Nil || len(transactions) == 0 || roleOf == nil {
		return
	}

	src := buildTransactionGroupEventSource(kind, groupID, revertedGroupID, transactions, roleOf, uc.transactionGroupEventTime())

	go func() {
		emitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncOperationTimeout)
		defer cancel()

		logger, tracer, _, _ := libObservability.NewTrackingFromContext(emitCtx)

		emitCtx, span := tracer.Start(emitCtx, "command.send_transaction_group_event_async")
		defer span.End()

		uc.emitTransactionGroupEvent(emitCtx, span, logger, kind, src)
	}()
}

func (uc *UseCase) emitTransactionGroupEvent(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	kind transactionGroupEventKind,
	src events.TransactionGroupSource,
) {
	payload := events.NewTransactionGroup(src)

	var (
		definitionKey string
		build         func(events.TransactionGroupPayload, string, time.Time) (libStreaming.EmitRequest, error)
	)

	switch kind {
	case transactionGroupEventPosted:
		definitionKey, build = events.TransactionGroupPostedDefinition.Key(), events.TransactionGroupPayload.ToEmitRequestPosted
	case transactionGroupEventCommitted:
		definitionKey, build = events.TransactionGroupCommittedDefinition.Key(), events.TransactionGroupPayload.ToEmitRequestCommitted
	case transactionGroupEventCanceled:
		definitionKey, build = events.TransactionGroupCanceledDefinition.Key(), events.TransactionGroupPayload.ToEmitRequestCanceled
	case transactionGroupEventReverted:
		definitionKey, build = events.TransactionGroupRevertedDefinition.Key(), events.TransactionGroupPayload.ToEmitRequestReverted
	default:
		return
	}

	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, definitionKey,
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return build(payload, tenantID, src.OccurredAt)
		})
}

func (uc *UseCase) transactionGroupEventTime() time.Time {
	if uc.Clock != nil {
		if now := uc.Clock(); !now.IsZero() {
			return now.UTC()
		}
	}

	return time.Now().UTC()
}

func buildTransactionGroupEventSource(
	kind transactionGroupEventKind,
	groupID uuid.UUID,
	revertedGroupID *uuid.UUID,
	transactions []*transaction.Transaction,
	roleOf func(*transaction.Transaction) string,
	occurredAt time.Time,
) events.TransactionGroupSource {
	status := constant.APPROVED
	if kind == transactionGroupEventCanceled {
		status = constant.CANCELED
	}

	src := events.TransactionGroupSource{
		GroupID:    groupID.String(),
		Status:     status,
		Parts:      make([]events.TransactionGroupPartSource, 0, len(transactions)),
		OccurredAt: occurredAt,
	}

	if revertedGroupID != nil {
		reverted := revertedGroupID.String()
		src.RevertedGroupID = &reverted
	}

	for _, tran := range transactions {
		if tran == nil {
			continue
		}

		if src.AssetCode == "" {
			src.AssetCode = tran.AssetCode
		}

		partStatus := tran.Status.Code
		if kind == transactionGroupEventPosted && partStatus == constant.CREATED {
			partStatus = constant.APPROVED
		}

		src.Parts = append(src.Parts, events.TransactionGroupPartSource{
			TransactionID:  tran.ID,
			OrganizationID: tran.OrganizationID,
			LedgerID:       tran.LedgerID,
			Role:           roleOf(tran),
			Status:         partStatus,
		})
	}

	return src
}

// crossLedgerGroupRole classifies a group member from its persisted legs with the
// same rule the group intent applies to its normalized parts. It returns "" for
// a transaction outside a group.
func crossLedgerGroupRole(tran *transaction.Transaction) string {
	if tran == nil || tran.GroupID == nil || tran.AssetCode == "" {
		return ""
	}

	bridge := "@external/" + tran.AssetCode

	role, err := classifyCrossLedgerGroupRole(
		crossLedgerAliasesContain(tran.Destination, bridge),
		crossLedgerAliasesContain(tran.Source, bridge),
		len(tran.Source) > 0,
	)
	if err != nil {
		return ""
	}

	return role
}

// crossLedgerAliasesContain matches the bridge against persisted aliases, which
// may still carry an entry index or a balance key around the alias itself.
func crossLedgerAliasesContain(aliases []string, bridge string) bool {
	for _, alias := range aliases {
		for _, segment := range strings.Split(alias, "#") {
			if segment == bridge {
				return true
			}
		}
	}

	return false
}
