// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	// defaultTransactionGroupReconcileMinAge keeps the reconciler away from a
	// group whose coordinator may still be running: both the group and the latest
	// change to any member must be at least this old.
	defaultTransactionGroupReconcileMinAge = 5 * time.Minute

	// defaultTransactionGroupOrphanMinAge is how old a group with no member must be
	// before its intent is deleted. It is far beyond the recovery window on
	// purpose: a hold whose projection was deferred to recovery has no member row
	// yet, and deleting its intent would leave its origins uncommittable.
	defaultTransactionGroupOrphanMinAge = 24 * time.Hour

	// transactionGroupReconcilePageSize and maxTransactionGroupReconcilePages bound
	// one pass. What a pass does not reach is picked up by the next one.
	transactionGroupReconcilePageSize = 100
	maxTransactionGroupReconcilePages = 100
)

// Bounded results of one reconciled group, used as the metric label.
const (
	transactionGroupReconcileRepaired     = "repaired"
	transactionGroupReconcileDeleted      = "deleted"
	transactionGroupReconcileInconsistent = "inconsistent"
	transactionGroupReconcileSkipped      = "skipped"
	transactionGroupReconcileFailed       = "failed"
)

// TransactionGroupReconciliationStats reports one reconciliation pass by
// outcome.
type TransactionGroupReconciliationStats struct {
	// Scanned is how many PENDING groups the pass read.
	Scanned int
	// Repaired is how many groups it moved to the terminal status every member
	// already holds.
	Repaired int
	// Deleted is how many member-less intents it removed.
	Deleted int
	// Inconsistent is how many groups have members that agree on no single
	// state. They are reported and never written.
	Inconsistent int
	// Skipped is how many groups it left alone: still held, recently active, or
	// aligned by another writer first.
	Skipped int
	// Failed is how many reads or writes could not complete.
	Failed int
}

// ReconcileTransactionGroups aligns the status of PENDING cross-ledger groups
// with their members.
//
// The members are the truth. A group row only labels them, and the label can
// lag when a commit or cancel applied its movement but did not reach the status
// update. For each group older than the minimum age the pass reads the members
// from the primary and concludes one of four things: every part approved or
// every origin canceled moves the row to that status and publishes the group
// fact the coordinator would have; a group still held, or touched recently, is
// left alone; a group with no member at all, old enough that no deferred
// projection can still produce one, loses its intent; anything else is reported
// as inconsistent and never written. Nothing here moves a balance.
func (uc *UseCase) ReconcileTransactionGroups(ctx context.Context) TransactionGroupReconciliationStats {
	var stats TransactionGroupReconciliationStats

	if uc.TransactionGroupRepo == nil {
		return stats
	}

	reader, ok := uc.TransactionReader.(TransactionGroupReader)
	if !ok {
		return stats
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.reconcile_transaction_groups")
	defer span.End()

	now := uc.transactionGroupEventTime()
	before := now.Add(-uc.transactionGroupReconcileMinAge())
	afterID := uuid.Nil

	for page := 0; page < maxTransactionGroupReconcilePages; page++ {
		if ctx.Err() != nil {
			break
		}

		groups, err := uc.TransactionGroupRepo.ListByStatusOlderThan(ctx, constant.PENDING, before, afterID, transactionGroupReconcilePageSize)
		if err != nil {
			stats.Failed++

			uc.recordTransactionGroupReconcileResult(ctx, logger, transactionGroupReconcileFailed)

			libOpentelemetry.HandleSpanError(span, "Failed to list pending cross-ledger transaction groups", err)
			logger.Log(ctx, libLog.LevelError, "Failed to list pending cross-ledger transaction groups", libLog.Err(err))

			break
		}

		for _, group := range groups {
			if group == nil {
				continue
			}

			stats.Scanned++

			result := uc.reconcileTransactionGroup(ctx, logger, reader, group, now)
			stats.count(result)
			uc.recordTransactionGroupReconcileResult(ctx, logger, result)
		}

		if len(groups) < transactionGroupReconcilePageSize {
			break
		}

		afterID = groups[len(groups)-1].ID
	}

	span.SetAttributes(
		attribute.Int("app.transaction_group.reconcile_scanned", stats.Scanned),
		attribute.Int("app.transaction_group.reconcile_repaired", stats.Repaired),
		attribute.Int("app.transaction_group.reconcile_deleted", stats.Deleted),
		attribute.Int("app.transaction_group.reconcile_inconsistent", stats.Inconsistent),
		attribute.Int("app.transaction_group.reconcile_failed", stats.Failed),
	)

	logger.Log(
		ctx, libLog.LevelDebug, "Cross-ledger transaction group reconciliation pass finished",
		libLog.Int("scanned", stats.Scanned),
		libLog.Int("repaired", stats.Repaired),
		libLog.Int("deleted", stats.Deleted),
		libLog.Int("inconsistent", stats.Inconsistent),
		libLog.Int("skipped", stats.Skipped),
		libLog.Int("failed", stats.Failed),
	)

	return stats
}

func (s *TransactionGroupReconciliationStats) count(result string) {
	switch result {
	case transactionGroupReconcileRepaired:
		s.Repaired++
	case transactionGroupReconcileDeleted:
		s.Deleted++
	case transactionGroupReconcileInconsistent:
		s.Inconsistent++
	case transactionGroupReconcileFailed:
		s.Failed++
	default:
		s.Skipped++
	}
}

func (uc *UseCase) reconcileTransactionGroup(
	ctx context.Context,
	logger libLog.Logger,
	reader TransactionGroupReader,
	group *transactiongroup.TransactionGroup,
	now time.Time,
) string {
	members, err := reader.FindTransactionsByGroupID(readrouting.WithPrimaryRead(ctx), group.ID)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to read cross-ledger transaction group members while reconciling",
			libLog.String("group_id", group.ID.String()), libLog.Err(err))

		return transactionGroupReconcileFailed
	}

	if len(members) == 0 {
		return uc.reconcileOrphanTransactionGroup(ctx, logger, group, now)
	}

	if now.Sub(latestTransactionGroupActivity(members)) < uc.transactionGroupReconcileMinAge() {
		return transactionGroupReconcileSkipped
	}

	intent, err := decodeCrossLedgerGroupIntent(group.Intent)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Cross-ledger transaction group intent is unreadable",
			libLog.String("group_id", group.ID.String()), libLog.Err(err))

		return transactionGroupReconcileInconsistent
	}

	roles := crossLedgerIntentRoles(*intent)
	target, held, consistent := classifyTransactionGroupMembers(*intent, roles, members)

	switch {
	case held:
		return transactionGroupReconcileSkipped
	case !consistent:
		logger.Log(ctx, libLog.LevelError, "Cross-ledger transaction group members disagree; left untouched",
			libLog.String("group_id", group.ID.String()), libLog.Int("member_count", len(members)),
			libLog.Int("part_count", len(intent.Parts)))

		return transactionGroupReconcileInconsistent
	}

	updated, err := uc.TransactionGroupRepo.UpdateStatus(ctx, group.ID, constant.PENDING, target)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to align cross-ledger transaction group status while reconciling",
			libLog.String("group_id", group.ID.String()), libLog.Err(err))

		return transactionGroupReconcileFailed
	}

	if !updated {
		return transactionGroupReconcileSkipped
	}

	uc.publishTransactionGroupEvent(ctx, crossLedgerGroupTransitionEvent(target), group.ID, nil, members,
		func(member *transaction.Transaction) string { return roles[crossLedgerMemberLedgerRef(member)] })

	return transactionGroupReconcileRepaired
}

func (uc *UseCase) reconcileOrphanTransactionGroup(
	ctx context.Context,
	logger libLog.Logger,
	group *transactiongroup.TransactionGroup,
	now time.Time,
) string {
	if now.Sub(group.CreatedAt) < uc.transactionGroupOrphanMinAge() {
		return transactionGroupReconcileSkipped
	}

	if err := uc.TransactionGroupRepo.Delete(ctx, group.ID); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to delete orphan cross-ledger transaction group while reconciling",
			libLog.String("group_id", group.ID.String()), libLog.Err(err))

		return transactionGroupReconcileFailed
	}

	logger.Log(ctx, libLog.LevelWarn, "Deleted cross-ledger transaction group intent that never produced a member",
		libLog.String("group_id", group.ID.String()))

	return transactionGroupReconcileDeleted
}

// classifyTransactionGroupMembers decides what the members of a PENDING group
// agree on. held reports a hold still waiting for its commit or cancel; target is
// the terminal status every member holds, valid only when consistent is true.
// Every member must belong to one intent part's scope, at most once. The role
// comes from the intent: a row read back from the repository carries no legs.
func classifyTransactionGroupMembers(
	intent CrossLedgerGroupIntent,
	roles map[atomicTransactionBatchLedgerRef]string,
	members []*transaction.Transaction,
) (target string, held, consistent bool) {
	origins := 0

	for index := range intent.Parts {
		if intent.Parts[index].Role == CrossLedgerGroupRoleOrigin {
			origins++
		}
	}

	seen := make(map[atomicTransactionBatchLedgerRef]struct{}, len(members))
	statuses := make(map[string]int, 2)

	for _, member := range members {
		if member == nil {
			return "", false, false
		}

		ref := crossLedgerMemberLedgerRef(member)
		if _, duplicate := seen[ref]; duplicate {
			return "", false, false
		}

		seen[ref] = struct{}{}

		if _, ok := roles[ref]; !ok {
			return "", false, false
		}

		statuses[member.Status.Code]++
	}

	switch {
	case statuses[constant.PENDING] == len(members) && len(members) == origins:
		return "", true, true
	case statuses[constant.APPROVED] == len(members) && len(members) == len(intent.Parts):
		return constant.APPROVED, false, true
	case statuses[constant.CANCELED] == len(members) && len(members) == origins:
		return constant.CANCELED, false, true
	default:
		return "", false, false
	}
}

// crossLedgerIntentRoles indexes the intent's roles by ledger scope. A hold
// group has at most one part per ledger, which the intent builder guarantees by
// decomposing per ledger.
func crossLedgerIntentRoles(intent CrossLedgerGroupIntent) map[atomicTransactionBatchLedgerRef]string {
	roles := make(map[atomicTransactionBatchLedgerRef]string, len(intent.Parts))
	for index := range intent.Parts {
		part := intent.Parts[index]
		roles[atomicTransactionBatchLedgerRef{organizationID: part.OrganizationID, ledgerID: part.LedgerID}] = part.Role
	}

	return roles
}

func latestTransactionGroupActivity(members []*transaction.Transaction) time.Time {
	var latest time.Time

	for _, member := range members {
		if member == nil {
			continue
		}

		for _, instant := range []time.Time{member.CreatedAt, member.UpdatedAt} {
			if instant.After(latest) {
				latest = instant
			}
		}
	}

	return latest
}

func (uc *UseCase) transactionGroupReconcileMinAge() time.Duration {
	if uc.TransactionGroupReconcileMinAge > 0 {
		return uc.TransactionGroupReconcileMinAge
	}

	return defaultTransactionGroupReconcileMinAge
}

func (uc *UseCase) transactionGroupOrphanMinAge() time.Duration {
	if uc.TransactionGroupOrphanMinAge > 0 {
		return uc.TransactionGroupOrphanMinAge
	}

	return defaultTransactionGroupOrphanMinAge
}

func (uc *UseCase) recordTransactionGroupReconcileResult(ctx context.Context, logger libLog.Logger, result string) {
	if uc.MetricsFactory == nil {
		return
	}

	if err := uc.MetricsFactory.AddCounter(
		ctx,
		"cross_ledger_group_reconcile_total",
		"Cross-ledger transaction groups read by the reconciler, by bounded result.",
		"1",
		map[string]string{"result": transactionGroupReconcileMetricResult(result)},
		1,
	); err != nil && logger != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit cross-ledger group reconcile metric", libLog.Err(err))
	}
}

func transactionGroupReconcileMetricResult(value string) string {
	switch value {
	case transactionGroupReconcileRepaired,
		transactionGroupReconcileDeleted,
		transactionGroupReconcileInconsistent,
		transactionGroupReconcileSkipped,
		transactionGroupReconcileFailed:
		return value
	default:
		return transactionGroupReconcileFailed
	}
}
