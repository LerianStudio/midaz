// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const (
	atomicTransactionBatchMaxExpandedPostings       = 100
	atomicTransactionBatchMaxExecutionBalances      = 150
	atomicTransactionBatchMaxCompletionPlanBytes    = 256 * 1024
	atomicTransactionBatchMaxAccountingRequestBytes = 256 * 1024
	atomicTransactionBatchMaxPreparedResponseBytes  = 1024 * 1024
	atomicTransactionBatchMaxRecoveryBytes          = 512 * 1024
	atomicTransactionBatchMaxCachedResponseBytes    = 1024 * 1024

	atomicTransactionBatchMovementSafetyBytes = 256
	atomicTransactionBatchSnapshotSafetyBytes = 128
)

const (
	atomicTransactionBatchBudgetExpandedPostings       = "expandedPostings"
	atomicTransactionBatchBudgetExecutionBalances      = "executionBalances"
	atomicTransactionBatchBudgetCompletionPlanBytes    = "completionPlanBytes"
	atomicTransactionBatchBudgetAccountingRequestBytes = "accountingRequestBytes"
	atomicTransactionBatchBudgetPreparedResponseBytes  = "preparedResponseBytes"
	atomicTransactionBatchBudgetRecoveryBytes          = "recoveryBytes"
	atomicTransactionBatchBudgetCachedResponseBytes    = "cachedResponseBytes"
)

type atomicTransactionBatchBudgetLimits struct {
	expandedPostings       int
	executionBalances      int
	completionPlanBytes    int
	accountingRequestBytes int
	preparedResponseBytes  int
	recoveryBytes          int
	cachedResponseBytes    int
}

var defaultAtomicTransactionBatchBudgetLimits = atomicTransactionBatchBudgetLimits{
	expandedPostings:       atomicTransactionBatchMaxExpandedPostings,
	executionBalances:      atomicTransactionBatchMaxExecutionBalances,
	completionPlanBytes:    atomicTransactionBatchMaxCompletionPlanBytes,
	accountingRequestBytes: atomicTransactionBatchMaxAccountingRequestBytes,
	preparedResponseBytes:  atomicTransactionBatchMaxPreparedResponseBytes,
	recoveryBytes:          atomicTransactionBatchMaxRecoveryBytes,
	cachedResponseBytes:    atomicTransactionBatchMaxCachedResponseBytes,
}

type atomicTransactionBatchBudgetMeasurements struct {
	expandedPostings       []int
	executionBalances      []int
	completionPlanBytes    []int
	accountingRequestBytes []int
	preparedResponseBytes  []int
	recoveryBytes          []int
	cachedResponseBytes    []int
}

func (uc *UseCase) enforceAtomicTransactionBatchExpandedPostings(run *atomicTransactionBatchRun) error {
	cumulative := make([]int, len(run.items))

	total := 0
	for index := range run.items {
		total += len(run.items[index].input.Send.Source.From) + len(run.items[index].input.Send.Distribute.To)
		cumulative[index] = total
	}

	err := validateAtomicTransactionBatchCumulativeBudget(
		atomicTransactionBatchBudgetExpandedPostings,
		cumulative,
		uc.resolvedAtomicTransactionBatchBudgetLimits().expandedPostings,
	)
	if err != nil {
		run.rejectionDimension = atomicTransactionBatchBudgetExpandedPostings
	}

	return err
}

func (uc *UseCase) prepareAndEnforceAtomicTransactionBatchBudgets(
	ctx context.Context,
	run *atomicTransactionBatchRun,
) error {
	if err := uc.prepareAtomicTransactionBatchCompletionPlans(ctx, run); err != nil {
		return err
	}

	measurements, err := measureAtomicTransactionBatchBudgets(run)
	if err != nil {
		return err
	}

	run.budgetMeasurements = &measurements
	limits := uc.resolvedAtomicTransactionBatchBudgetLimits()

	checks := []struct {
		dimension  string
		cumulative []int
		limit      int
	}{
		{atomicTransactionBatchBudgetExecutionBalances, measurements.executionBalances, limits.executionBalances},
		{atomicTransactionBatchBudgetCompletionPlanBytes, measurements.completionPlanBytes, limits.completionPlanBytes},
		{atomicTransactionBatchBudgetAccountingRequestBytes, measurements.accountingRequestBytes, limits.accountingRequestBytes},
		{atomicTransactionBatchBudgetPreparedResponseBytes, measurements.preparedResponseBytes, limits.preparedResponseBytes},
		{atomicTransactionBatchBudgetRecoveryBytes, measurements.recoveryBytes, limits.recoveryBytes},
		{atomicTransactionBatchBudgetCachedResponseBytes, measurements.cachedResponseBytes, limits.cachedResponseBytes},
	}
	for _, check := range checks {
		if err := validateAtomicTransactionBatchCumulativeBudget(check.dimension, check.cumulative, check.limit); err != nil {
			run.rejectionDimension = check.dimension
			return err
		}
	}

	return nil
}

func (uc *UseCase) resolvedAtomicTransactionBatchBudgetLimits() atomicTransactionBatchBudgetLimits {
	if uc.atomicTransactionBatchBudgetLimitOverride == nil {
		return defaultAtomicTransactionBatchBudgetLimits
	}

	return *uc.atomicTransactionBatchBudgetLimitOverride
}

func validateAtomicTransactionBatchCumulativeBudget(dimension string, cumulative []int, limit int) error {
	if dimension == "" || limit <= 0 {
		return fmt.Errorf("atomic transaction batch %s budget limit must be positive", dimension)
	}

	for index, observed := range cumulative {
		if observed <= limit {
			continue
		}

		primary := pkg.ValidateBusinessError(
			constant.ErrTransactionBatchBudgetExceeded,
			constant.EntityTransaction,
			dimension,
			index,
			observed,
			limit,
		)

		return withAtomicTransactionBatchItemError(
			primary,
			index,
			fmt.Sprintf("%s budget observed %d exceeds maximum %d", dimension, observed, limit),
		)
	}

	return nil
}

func (uc *UseCase) prepareAtomicTransactionBatchCompletionPlans(
	ctx context.Context,
	run *atomicTransactionBatchRun,
) error {
	if run.executionID == uuid.Nil {
		executionID, err := uc.UUIDv7Generator()
		if err != nil {
			return fmt.Errorf("generate atomic transaction batch execution id: %w", err)
		}

		if executionID == uuid.Nil {
			return errors.New("atomic transaction batch UUIDv7 generator returned a nil execution id")
		}

		run.executionID = executionID
	}

	_, _, headerID, _ := libObservability.NewTrackingFromContext(ctx)

	intent := EngineIntent{
		TenantID:       tmcore.GetTenantIDContext(ctx),
		OrganizationID: run.organizationID,
		LedgerID:       run.ledgerID,
		ExecutionID:    run.executionID,
		Transactions:   make([]EngineTransactionIntent, len(run.items)),
	}
	for index := range run.items {
		item := &run.items[index]
		item.guard = ExecutionGuard{
			TransactionID: item.transactionID,
			ExpectedToken: "",
			NextToken:     item.status,
		}
		item.completionPlan = TransactionCompletionPlan{
			FormatVersion:        TransactionCompletionFormatVersion,
			TenantID:             intent.TenantID,
			HeaderID:             headerID,
			TransactionID:        item.transactionID,
			GroupID:              run.groupID,
			FeesSkipped:          item.honoredFeeSkip,
			TracerSkipped:        item.honoredTracerSkip,
			OrganizationID:       item.organizationID,
			LedgerID:             item.ledgerID,
			ExecutionID:          run.executionID,
			TransactionInput:     item.input,
			TTL:                  item.operationUpdatedAt,
			Validate:             item.validate,
			TransactionStatus:    item.status,
			Action:               item.action,
			TransactionDate:      item.transactionDate,
			TransactionCreatedAt: item.transactionDate,
			TransactionUpdatedAt: item.transactionUpdatedAt,
			OperationUpdatedAt:   item.operationUpdatedAt,
			OperationSpecs:       item.prepared.projection,
		}
		intent.Transactions[index] = transactionCompletionIntent(item.prepared.transaction, item.completionPlan)
	}

	fingerprint, err := ComputeEngineIntentFingerprint(intent)
	if err != nil {
		return err
	}

	run.engineIntentFingerprint = fingerprint
	for index := range run.items {
		item := &run.items[index]
		item.completionPlan.IntentFingerprint = fingerprint

		payload, err := EncodeTransactionCompletionPlan(item.completionPlan)
		if err != nil {
			return err
		}

		item.completionPlanPayload = append(item.completionPlanPayload[:0], payload...)
	}

	return nil
}

func measureAtomicTransactionBatchBudgets(run *atomicTransactionBatchRun) (atomicTransactionBatchBudgetMeasurements, error) {
	measurements := atomicTransactionBatchBudgetMeasurements{
		expandedPostings:       make([]int, len(run.items)),
		executionBalances:      make([]int, len(run.items)),
		completionPlanBytes:    make([]int, len(run.items)),
		accountingRequestBytes: make([]int, len(run.items)),
		preparedResponseBytes:  make([]int, len(run.items)),
		recoveryBytes:          make([]int, len(run.items)),
		cachedResponseBytes:    make([]int, len(run.items)),
	}

	balancePrefixes, err := atomicTransactionBatchBalancePrefixes(run)
	if err != nil {
		return atomicTransactionBatchBudgetMeasurements{}, err
	}

	transactions := make([]accounting.Transaction, 0, len(run.items))
	guards := make([]ExecutionGuard, 0, len(run.items))
	records := make([]CompletionPlanRecord, 0, len(run.items))
	plans := make([]TransactionCompletionPlan, 0, len(run.items))
	publicTransactions := make([]*transaction.Transaction, 0, len(run.items))
	recoveryBytes := 0
	preparedWriteBehindBytes := 0
	completionPlanBytes := 0
	expandedPostings := 0
	protectedTransactions := make([]uuid.UUID, 0, len(run.items))
	protectedRecoveryFields := make([]string, 0, len(run.items))
	protectedIndexFields := make([]uuid.UUID, 0, len(run.items))
	capturedResponses := make(map[string]string, len(run.items))
	publicResponsePayloads := make([]json.RawMessage, 0, len(run.items))

	for index := range run.items {
		item := &run.items[index]
		expandedPostings += len(item.prepared.transaction.Postings)
		dependencies := []TransactionEvidenceReference{}

		dependencyPayload, err := json.Marshal(dependencies)
		if err != nil {
			return atomicTransactionBatchBudgetMeasurements{}, fmt.Errorf("measure atomic transaction batch dependencies: %w", err)
		}

		completionPlanBytes += len(item.completionPlanPayload) + len(dependencyPayload)
		transactions = append(transactions, item.prepared.transaction)
		guards = append(guards, item.guard)
		records = append(records, CompletionPlanRecord{
			TransactionID: item.transactionID,
			Payload:       append(json.RawMessage(nil), item.completionPlanPayload...),
			Dependencies:  dependencies,
		})
		plans = append(plans, item.completionPlan)
		publicTransactions = append(publicTransactions, atomicTransactionBatchFoundationResult(item))

		request := accounting.Execution{
			OrganizationID: run.organizationID,
			LedgerID:       run.ledgerID,
			ExecutionID:    run.executionID,
			Transactions:   append([]accounting.Transaction(nil), transactions...),
			Balances:       append([]accounting.BalanceSnapshot(nil), balancePrefixes[index]...),
		}
		engine := EngineExecution{
			Execution:         request,
			IntentFingerprint: run.engineIntentFingerprint,
			RetentionSeconds:  atomicTransactionBatchRetentionSeconds(run.idempotencyTTL),
			Guards:            append([]ExecutionGuard(nil), guards...),
			CompletionPlans:   append([]CompletionPlanRecord(nil), records...),
		}
		prepared := PreparedEngineExecution{
			Execution:       engine,
			CompletionPlans: append([]TransactionCompletionPlan(nil), plans...),
		}

		accountingBytes, err := json.Marshal(request)
		if err != nil {
			return atomicTransactionBatchBudgetMeasurements{}, fmt.Errorf("measure atomic transaction batch accounting request: %w", err)
		}

		preparedBytes, err := json.Marshal(prepared)
		if err != nil {
			return atomicTransactionBatchBudgetMeasurements{}, fmt.Errorf("measure atomic transaction batch prepared response: %w", err)
		}

		result := atomicTransactionBatchBudgetResult(*item)
		recovery := TransactionCompletionRecord{
			FormatVersion:     TransactionCompletionFormatVersion,
			TenantID:          item.completionPlan.TenantID,
			OrganizationID:    item.organizationID,
			LedgerID:          item.ledgerID,
			ExecutionID:       run.executionID,
			IntentFingerprint: run.engineIntentFingerprint,
			TransactionID:     item.transactionID,
			Payload:           string(item.completionPlanPayload),
			Result:            result,
		}

		recoveryPayload, err := json.Marshal(TransactionWriteBehindEnvelope{
			FormatVersion: TransactionWriteBehindFormatVersion, ApplicationState: TransactionApplicationConfirmed,
			ReplayState: TransactionReplayReconstructible, DurabilityState: TransactionDurabilityPending,
			Record: recovery, Dependencies: dependencies,
		})
		if err != nil {
			return atomicTransactionBatchBudgetMeasurements{}, fmt.Errorf("measure atomic transaction batch recovery: %w", err)
		}

		recoveryField := item.transactionID.String() + ":" + run.executionID.String()

		indexPayload, err := EncodeTransactionEvidenceIndex(TransactionEvidenceIndex{
			FormatVersion: TransactionEvidenceIndexFormatVersion, TenantID: item.completionPlan.TenantID,
			OrganizationID: item.organizationID, LedgerID: item.ledgerID, TransactionID: item.transactionID,
			ExecutionID: run.executionID, Action: item.action, ApplicationState: TransactionApplicationConfirmed,
			ReplayState: TransactionReplayReconstructible, DurabilityState: TransactionDurabilityPending,
			RecoveryField: recoveryField, ReceiptField: run.executionID.String(), Dependencies: dependencies,
		})
		if err != nil {
			return atomicTransactionBatchBudgetMeasurements{}, fmt.Errorf("measure atomic transaction batch index: %w", err)
		}

		recoveryBytes += len(recoveryPayload) +
			len(result.Movements)*atomicTransactionBatchMovementSafetyBytes +
			len(result.Final)*atomicTransactionBatchSnapshotSafetyBytes
		preparedWriteBehindBytes += len(recoveryPayload) + len(indexPayload) + len(recoveryField) + len(item.transactionID.String())
		protectedTransactions = append(protectedTransactions, item.transactionID)
		protectedRecoveryFields = append(protectedRecoveryFields, recoveryField)
		protectedIndexFields = append(protectedIndexFields, item.transactionID)

		responsePayload, err := json.Marshal(publicTransactions[len(publicTransactions)-1])
		if err != nil {
			return atomicTransactionBatchBudgetMeasurements{}, fmt.Errorf("measure atomic transaction batch immutable response: %w", err)
		}

		capturedResponses[item.transactionID.String()] = base64.StdEncoding.EncodeToString(responsePayload)
		publicResponsePayloads = append(publicResponsePayloads, responsePayload)

		batchResponsePayload, err := json.Marshal(struct {
			Transactions []json.RawMessage `json:"transactions"`
		}{Transactions: publicResponsePayloads})
		if err != nil {
			return atomicTransactionBatchBudgetMeasurements{}, fmt.Errorf("measure atomic transaction batch replay response: %w", err)
		}

		receiptPayload, err := json.Marshal(struct {
			FormatVersion     int       `json:"formatVersion"`
			TenantID          string    `json:"tenantId"`
			OrganizationID    uuid.UUID `json:"organizationId"`
			LedgerID          uuid.UUID `json:"ledgerId"`
			ExecutionID       uuid.UUID `json:"executionId"`
			IntentFingerprint string    `json:"intentFingerprint"`
			Response          string    `json:"response"`
			Protection        struct {
				FormatVersion         int              `json:"formatVersion"`
				RetentionSeconds      int64            `json:"retentionSeconds"`
				Transactions          []uuid.UUID      `json:"transactions"`
				RecoveryFields        []string         `json:"recoveryFields"`
				IndexFields           []uuid.UUID      `json:"indexFields"`
				Acknowledged          map[string]bool  `json:"acknowledged"`
				TerminalCompletedAtMS map[string]int64 `json:"terminalCompletedAtMs"`
			} `json:"protection"`
		}{
			FormatVersion: 1, TenantID: item.completionPlan.TenantID, OrganizationID: run.organizationID,
			LedgerID: run.ledgerID, ExecutionID: run.executionID, IntentFingerprint: run.engineIntentFingerprint,
			Response: string(batchResponsePayload), Protection: struct {
				FormatVersion         int              `json:"formatVersion"`
				RetentionSeconds      int64            `json:"retentionSeconds"`
				Transactions          []uuid.UUID      `json:"transactions"`
				RecoveryFields        []string         `json:"recoveryFields"`
				IndexFields           []uuid.UUID      `json:"indexFields"`
				Acknowledged          map[string]bool  `json:"acknowledged"`
				TerminalCompletedAtMS map[string]int64 `json:"terminalCompletedAtMs"`
			}{
				FormatVersion: 2, RetentionSeconds: atomicTransactionBatchRetentionSeconds(run.idempotencyTTL),
				Transactions: protectedTransactions, RecoveryFields: protectedRecoveryFields, IndexFields: protectedIndexFields,
				Acknowledged: map[string]bool{}, TerminalCompletedAtMS: map[string]int64{},
			},
		})
		if err != nil {
			return atomicTransactionBatchBudgetMeasurements{}, fmt.Errorf("measure atomic transaction batch receipt: %w", err)
		}

		cachedPayload, err := encodeAtomicTransactionBatchCachedBudget(
			publicTransactions, plans, capturedResponses, batchResponsePayload,
		)
		if err != nil {
			return atomicTransactionBatchBudgetMeasurements{}, fmt.Errorf("measure atomic transaction batch cached response: %w", err)
		}

		measurements.expandedPostings[index] = expandedPostings
		measurements.executionBalances[index] = len(balancePrefixes[index])
		measurements.completionPlanBytes[index] = completionPlanBytes
		measurements.accountingRequestBytes[index] = len(accountingBytes)
		measurements.preparedResponseBytes[index] = len(preparedBytes) + preparedWriteBehindBytes + len(receiptPayload)
		measurements.recoveryBytes[index] = recoveryBytes
		measurements.cachedResponseBytes[index] = len(cachedPayload)
	}

	return measurements, nil
}

func encodeAtomicTransactionBatchCachedBudget(
	transactions []*transaction.Transaction,
	plans []TransactionCompletionPlan,
	captures map[string]string,
	response json.RawMessage,
) ([]byte, error) {
	return json.Marshal(struct {
		Transactions []*transaction.Transaction  `json:"transactions"`
		Plans        []TransactionCompletionPlan `json:"plans"`
		Captures     map[string]string           `json:"initialResponses"`
		Response     json.RawMessage             `json:"response"`
	}{
		Transactions: transactions,
		Plans:        plans,
		Captures:     captures,
		Response:     response,
	})
}

func atomicTransactionBatchBalancePrefixes(run *atomicTransactionBatchRun) ([][]accounting.BalanceSnapshot, error) {
	if len(run.items) == 0 {
		return nil, errors.New("atomic transaction batch has no prepared items")
	}

	byRef := make(map[string]accounting.BalanceSnapshot)

	for index := range run.items {
		for _, snapshot := range run.items[index].prepared.pool.Snapshots {
			byRef[atomicTransactionBatchScopedSnapshotRef(snapshot)] = snapshot
		}
	}

	seen := make(map[string]struct{}, len(byRef))
	ordered := make([]accounting.BalanceSnapshot, 0, len(byRef))

	prefixes := make([][]accounting.BalanceSnapshot, len(run.items))
	for index := range run.items {
		item := &run.items[index]
		for _, balance := range run.items[index].prepared.pool.ExplicitBalances {
			ref := atomicTransactionBatchPreparedBalanceRef(balance)

			scopedRef := atomicTransactionBatchScopedRef(item.organizationID, item.ledgerID, ref)
			if snapshot, ok := byRef[scopedRef]; ok {
				if _, exists := seen[scopedRef]; !exists {
					seen[scopedRef] = struct{}{}

					ordered = append(ordered, snapshot)
				}
			}

			if balance.Key == constant.OverdraftBalanceKey {
				continue
			}

			companionRef := mtransaction.AliasKey(mtransaction.SplitAlias(balance.Alias), constant.OverdraftBalanceKey)

			companionScopedRef := atomicTransactionBatchScopedRef(item.organizationID, item.ledgerID, companionRef)
			if snapshot, ok := byRef[companionScopedRef]; ok {
				if _, exists := seen[companionScopedRef]; !exists {
					seen[companionScopedRef] = struct{}{}

					ordered = append(ordered, snapshot)
				}
			}
		}

		prefixes[index] = append([]accounting.BalanceSnapshot(nil), ordered...)
	}

	if len(seen) != len(byRef) {
		return nil, errors.New("atomic transaction batch shared pool contains an unowned execution balance")
	}

	return prefixes, nil
}

func atomicTransactionBatchScopedSnapshotRef(snapshot accounting.BalanceSnapshot) string {
	return atomicTransactionBatchScopedRef(snapshot.OrganizationID, snapshot.LedgerID, snapshot.BalanceRef)
}

func atomicTransactionBatchScopedRef(organizationID, ledgerID uuid.UUID, ref string) string {
	return organizationID.String() + ":" + ledgerID.String() + ":" + ref
}

func atomicTransactionBatchPreparedBalanceRef(balance *mmodel.Balance) string {
	key := balance.Key
	if key == "" {
		key = constant.DefaultBalanceKey
	}

	return mtransaction.AliasKey(mtransaction.SplitAlias(balance.Alias), key)
}

func atomicTransactionBatchBudgetResult(item atomicTransactionBatchItemRun) accounting.ExecutionResult {
	snapshots := make(map[string]accounting.BalanceSnapshot, len(item.prepared.pool.Snapshots))
	for _, snapshot := range item.prepared.pool.Snapshots {
		snapshots[snapshot.BalanceRef] = snapshot
	}

	companions := make(map[string]string)

	for _, spec := range item.prepared.projection {
		if spec.Role == accounting.RoleOverdraftCompanion {
			companions[spec.PostingRef] = spec.BalanceRef
		}
	}

	result := accounting.ExecutionResult{
		Movements: make([]accounting.Movement, 0, len(item.prepared.transaction.Postings)*2),
		Final:     make([]accounting.BalanceSnapshot, 0, len(item.prepared.pool.Snapshots)),
	}
	touched := make(map[string]struct{})

	appendMovement := func(posting accounting.Posting, role, balanceRef string) {
		snapshot, ok := snapshots[balanceRef]
		if !ok {
			return
		}

		state := accounting.BalanceState{
			Available:     snapshot.Available,
			OnHold:        snapshot.OnHold,
			OverdraftUsed: snapshot.OverdraftUsed,
			Version:       snapshot.Version,
		}

		result.Movements = append(result.Movements, accounting.Movement{
			Ref:            fmt.Sprintf("%s:%d:%s:%s:0", item.transactionID, len(posting.Ref), posting.Ref, role),
			TransactionID:  item.transactionID,
			PostingRef:     posting.Ref,
			Role:           role,
			BalanceRef:     balanceRef,
			Type:           posting.Type,
			Amount:         posting.Amount,
			OverdraftDelta: posting.Amount,
			Before:         state,
			After:          state,
		})
		if _, exists := touched[balanceRef]; !exists {
			touched[balanceRef] = struct{}{}

			result.Final = append(result.Final, snapshot)
		}
	}
	for _, posting := range item.prepared.transaction.Postings {
		appendMovement(posting, accounting.RolePrimary, posting.BalanceRef)

		if companionRef := companions[posting.Ref]; companionRef != "" {
			appendMovement(posting, accounting.RoleOverdraftCompanion, companionRef)
		}
	}

	return result
}

func atomicTransactionBatchRetentionSeconds(ttl time.Duration) int64 {
	return idempotencyRetentionSeconds(ttl)
}
