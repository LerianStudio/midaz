// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// buildCrossLedgerGroupExecution combines independently prepared lifecycle and
// create fragments under one execution identity and one intent fingerprint.
//
//nolint:gocyclo,gocognit // exhaustive fragment validation protects the single mixed-execution boundary
func buildCrossLedgerGroupExecution(
	organizationID, ledgerID, groupID, executionID uuid.UUID,
	fragments []PreparedEngineExecution,
) (PreparedEngineExecution, error) {
	if organizationID == uuid.Nil || ledgerID == uuid.Nil || groupID == uuid.Nil || executionID == uuid.Nil {
		return PreparedEngineExecution{}, errors.New("cross-ledger group execution identity is incomplete")
	}

	if len(fragments) == 0 {
		return PreparedEngineExecution{}, errors.New("cross-ledger group execution has no fragments")
	}

	prepared := PreparedEngineExecution{
		Execution: EngineExecution{
			Execution: accounting.Execution{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				ExecutionID:    executionID,
				Transactions:   make([]accounting.Transaction, 0),
				Balances:       make([]accounting.BalanceSnapshot, 0),
			},
			Guards:          make([]ExecutionGuard, 0),
			CompletionPlans: make([]CompletionPlanRecord, 0),
		},
		CompletionPlans: make([]TransactionCompletionPlan, 0),
	}

	dependencies := make([][]TransactionEvidenceReference, 0)
	seenBalances := make(map[string]accounting.BalanceSnapshot)

	for fragmentIndex := range fragments {
		fragment := fragments[fragmentIndex]

		count := len(fragment.Execution.Execution.Transactions)
		if count == 0 || len(fragment.Execution.Guards) != count ||
			len(fragment.Execution.CompletionPlans) != count || len(fragment.CompletionPlans) != count {
			return PreparedEngineExecution{}, fmt.Errorf("cross-ledger group fragment %d is incomplete", fragmentIndex)
		}

		prepared.Execution.Execution.Transactions = append(
			prepared.Execution.Execution.Transactions,
			fragment.Execution.Execution.Transactions...,
		)

		prepared.Execution.Guards = append(prepared.Execution.Guards, fragment.Execution.Guards...)
		if fragment.Execution.RetentionSeconds > prepared.Execution.RetentionSeconds {
			prepared.Execution.RetentionSeconds = fragment.Execution.RetentionSeconds
		}

		for index := range fragment.CompletionPlans {
			plan := fragment.CompletionPlans[index]
			plan.ExecutionID = executionID
			plan.GroupID = cloneUUIDPointer(&groupID)
			plan.IntentFingerprint = ""
			prepared.CompletionPlans = append(prepared.CompletionPlans, plan)
			dependencies = append(dependencies, append(
				[]TransactionEvidenceReference(nil),
				fragment.Execution.CompletionPlans[index].Dependencies...,
			))
		}

		for _, snapshot := range fragment.Execution.Execution.Balances {
			key := completionScopedBalanceRef(snapshot.OrganizationID, snapshot.LedgerID, snapshot.BalanceRef)
			if existing, ok := seenBalances[key]; ok {
				if !equalEngineSnapshot(existing, snapshot) {
					return PreparedEngineExecution{}, fmt.Errorf("cross-ledger group balance %q diverges between fragments", key)
				}

				continue
			}

			seenBalances[key] = snapshot
			prepared.Execution.Execution.Balances = append(prepared.Execution.Execution.Balances, snapshot)
		}
	}

	if len(prepared.CompletionPlans) > maxPreparedEngineTransactions {
		return PreparedEngineExecution{}, errors.New("cross-ledger group execution exceeds transaction cardinality")
	}

	refs := make([]atomicTransactionBatchLedgerRef, 0, len(prepared.CompletionPlans))
	for _, plan := range prepared.CompletionPlans {
		refs = append(refs, atomicTransactionBatchLedgerRef{organizationID: plan.OrganizationID, ledgerID: plan.LedgerID})
	}

	coordinationOrganizationID, coordinationLedgerID := atomicTransactionBatchCoordinationScope(refs)
	multiScope := false

	plans := make([]*TransactionCompletionPlan, len(prepared.CompletionPlans))
	for index := range prepared.CompletionPlans {
		plans[index] = &prepared.CompletionPlans[index]
	}

	stampTransactionCompletionMembers(plans)

	for _, ref := range refs[1:] {
		if ref != refs[0] {
			multiScope = true
			break
		}
	}

	if multiScope {
		for index := range prepared.CompletionPlans {
			prepared.CompletionPlans[index].CoordinationOrganizationID = &coordinationOrganizationID
			prepared.CompletionPlans[index].CoordinationLedgerID = &coordinationLedgerID
			prepared.CompletionPlans[index].ReceiptOrganizationID = &organizationID
			prepared.CompletionPlans[index].ReceiptLedgerID = &ledgerID
		}
	}

	intent := EngineIntent{
		TenantID:       prepared.CompletionPlans[0].TenantID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		ExecutionID:    executionID,
		Transactions:   make([]EngineTransactionIntent, len(prepared.CompletionPlans)),
	}
	for index := range prepared.CompletionPlans {
		plan := prepared.CompletionPlans[index]
		if plan.TenantID != intent.TenantID {
			return PreparedEngineExecution{}, errors.New("cross-ledger group execution mixes tenants")
		}

		intent.Transactions[index] = transactionCompletionIntent(
			prepared.Execution.Execution.Transactions[index],
			plan,
		)
	}

	fingerprint, err := ComputeEngineIntentFingerprint(intent)
	if err != nil {
		return PreparedEngineExecution{}, err
	}

	prepared.Execution.IntentFingerprint = fingerprint

	prepared.Execution.CompletionPlans = make([]CompletionPlanRecord, len(prepared.CompletionPlans))
	for index := range prepared.CompletionPlans {
		prepared.CompletionPlans[index].IntentFingerprint = fingerprint

		payload, err := EncodeTransactionCompletionPlan(prepared.CompletionPlans[index])
		if err != nil {
			return PreparedEngineExecution{}, err
		}

		prepared.Execution.CompletionPlans[index] = CompletionPlanRecord{
			TransactionID: prepared.CompletionPlans[index].TransactionID,
			Payload:       append(json.RawMessage(nil), payload...),
			Dependencies:  dependencies[index],
		}
	}

	if err := validatePreparedEngineExecution(prepared); err != nil {
		return PreparedEngineExecution{}, err
	}

	return prepared, nil
}
