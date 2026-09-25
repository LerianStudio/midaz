// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
)

// Domain operations of the cross-ledger coordinators (D6 catalog, component
// "ledger").
const (
	crossLedgerOperationCreateTransaction = "create_cross_ledger_transaction"
	crossLedgerOperationCreateHold        = "create_cross_ledger_hold"
	crossLedgerOperationCommitGroup       = "commit_cross_ledger_group"
	crossLedgerOperationCancelGroup       = "cancel_cross_ledger_group"
	crossLedgerOperationRevertGroup       = "revert_cross_ledger_group"
)

// recordCrossLedgerGroupError records a coordinator failure on its span by error
// class. The coordinator is where a grouped request is accepted or refused as a
// whole, so both business and technical failures are logged once here at the
// level matching their error class.
func recordCrossLedgerGroupError(ctx context.Context, span trace.Span, logger libLog.Logger, message string, err error) {
	if err == nil {
		return
	}

	recordCommandError(ctx, span, logger, message, err)
}

// setCrossLedgerGroupShape records the request-derived shape of a group.
func setCrossLedgerGroupShape(span trace.Span, refs []atomicTransactionBatchLedgerRef) int {
	ledgers := countCrossLedgerGroupLedgers(refs)

	span.SetAttributes(
		attribute.Int("app.request.part_count", len(refs)),
		attribute.Int("app.request.ledger_count", ledgers),
	)

	return ledgers
}

func countCrossLedgerGroupLedgers(refs []atomicTransactionBatchLedgerRef) int {
	distinct := make(map[atomicTransactionBatchLedgerRef]struct{}, len(refs))
	for _, ref := range refs {
		distinct[ref] = struct{}{}
	}

	return len(distinct)
}

func crossLedgerIntentLedgerRefs(intent CrossLedgerGroupIntent) []atomicTransactionBatchLedgerRef {
	refs := make([]atomicTransactionBatchLedgerRef, len(intent.Parts))
	for index := range intent.Parts {
		refs[index] = atomicTransactionBatchLedgerRef{
			organizationID: intent.Parts[index].OrganizationID,
			ledgerID:       intent.Parts[index].LedgerID,
		}
	}

	return refs
}

func crossLedgerBatchLedgerRefs(items []CreateAtomicTransactionBatchV2ItemInput) []atomicTransactionBatchLedgerRef {
	refs := make([]atomicTransactionBatchLedgerRef, len(items))
	for index := range items {
		refs[index] = atomicTransactionBatchLedgerRef{organizationID: items[index].OrganizationID, ledgerID: items[index].LedgerID}
	}

	return refs
}

// crossLedgerMemberLedgerRefs maps persisted members to their scopes. A member
// whose scope does not parse contributes the nil scope, which still counts as one
// part without inventing a ledger.
func crossLedgerMemberLedgerRefs(members []*transaction.Transaction) []atomicTransactionBatchLedgerRef {
	refs := make([]atomicTransactionBatchLedgerRef, 0, len(members))

	for _, member := range members {
		if member == nil {
			continue
		}

		refs = append(refs, crossLedgerMemberLedgerRef(member))
	}

	return refs
}

func crossLedgerMemberLedgerRef(member *transaction.Transaction) atomicTransactionBatchLedgerRef {
	organizationID, _ := uuid.Parse(member.OrganizationID)
	ledgerID, _ := uuid.Parse(member.LedgerID)

	return atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: ledgerID}
}
