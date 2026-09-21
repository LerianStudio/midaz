// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

type transactionEvidenceResolverStub struct {
	records map[string]*TransactionWriteBehindEnvelope
	calls   []string
}

func (resolver *transactionEvidenceResolverStub) ResolveTransactionEvidence(_ context.Context, reference TransactionEvidenceReference) (*TransactionWriteBehindEnvelope, error) {
	identity := transactionCompletionEvidenceIdentity(reference.TransactionID, reference.ExecutionID)
	resolver.calls = append(resolver.calls, identity)
	return resolver.records[identity], nil
}

type orderedAppliedTransactionCompleter struct {
	records []string
	failOn  uuid.UUID
}

func (completer *orderedAppliedTransactionCompleter) Complete(_ context.Context, record *TransactionCompletionRecord) (TransactionCompletionResult, error) {
	completer.records = append(completer.records, transactionCompletionEvidenceIdentity(record.TransactionID, record.ExecutionID))
	if record.ExecutionID == completer.failOn {
		return TransactionCompletionResult{}, ErrTransactionCompletionConflict
	}

	return TransactionCompletionResult{Outcome: TransactionPersistenceOutcome{TransactionStatus: "durable"}}, nil
}

func TestEngineWriteBehindCompletionPersistsPredecessorBeforeCurrentAndConvergesDuplicates(t *testing.T) {
	current := writeBehindEnvelopeFixture(t)
	predecessor := transactionWriteBehindExecutionFixture(t, current, uuid.New())
	current.Dependencies = []TransactionEvidenceReference{transactionEvidenceReference(TransactionDependencyPredecessor, predecessor)}
	resolver := &transactionEvidenceResolverStub{records: map[string]*TransactionWriteBehindEnvelope{
		transactionCompletionEvidenceIdentity(predecessor.Record.TransactionID, predecessor.Record.ExecutionID): &predecessor,
	}}
	completer := &orderedAppliedTransactionCompleter{}

	first, err := CompleteTransactionWriteBehind(context.Background(), &current, resolver, completer)
	require.NoError(t, err)
	require.Len(t, first.Dependencies, 1)
	require.Equal(t, []string{
		transactionCompletionEvidenceIdentity(predecessor.Record.TransactionID, predecessor.Record.ExecutionID),
		transactionCompletionEvidenceIdentity(current.Record.TransactionID, current.Record.ExecutionID),
	}, completer.records)

	_, err = CompleteTransactionWriteBehind(context.Background(), &current, resolver, completer)
	require.NoError(t, err)
	require.Len(t, completer.records, 4, "redelivery re-verifies the same two durable records without synthesizing state")
}

func TestEngineWriteBehindCompletionResolvesOriginAndRetainsConflict(t *testing.T) {
	current := writeBehindEnvelopeFixture(t)
	origin := transactionWriteBehindExecutionFixture(t, current, uuid.New())
	origin.Record.TransactionID = uuid.New()
	originPlan, err := DecodeTransactionCompletionPlan([]byte(origin.Record.Payload))
	require.NoError(t, err)
	originPlan.TransactionID = origin.Record.TransactionID
	for index := range originPlan.OperationSpecs {
		originPlan.OperationSpecs[index].TransactionID = origin.Record.TransactionID
	}
	originPlan.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(*originPlan))
	require.NoError(t, err)
	originResult := origin.Record.Result
	originResult.Movements = append([]accounting.Movement(nil), originResult.Movements...)
	for index := range originResult.Movements {
		originResult.Movements[index].TransactionID = origin.Record.TransactionID
	}
	origin.Record = recoveryContractEnvelope(t, *originPlan, originResult)

	currentPlan, err := DecodeTransactionCompletionPlan([]byte(current.Record.Payload))
	require.NoError(t, err)
	currentPlan.ParentTransactionID = &origin.Record.TransactionID
	currentPlan.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(*currentPlan))
	require.NoError(t, err)
	current.Record = recoveryContractEnvelope(t, *currentPlan, current.Record.Result)
	current.Dependencies = []TransactionEvidenceReference{transactionEvidenceReference(TransactionDependencyOrigin, origin)}
	resolver := &transactionEvidenceResolverStub{records: map[string]*TransactionWriteBehindEnvelope{
		transactionCompletionEvidenceIdentity(origin.Record.TransactionID, origin.Record.ExecutionID): &origin,
	}}
	completer := &orderedAppliedTransactionCompleter{failOn: current.Record.ExecutionID}

	_, err = CompleteTransactionWriteBehind(context.Background(), &current, resolver, completer)
	require.ErrorIs(t, err, ErrTransactionCompletionConflict)
	require.Equal(t, []string{
		transactionCompletionEvidenceIdentity(origin.Record.TransactionID, origin.Record.ExecutionID),
		transactionCompletionEvidenceIdentity(current.Record.TransactionID, current.Record.ExecutionID),
	}, completer.records)
}

func TestEngineWriteBehindCompletionRejectsCycleAndMismatchedEvidence(t *testing.T) {
	first := writeBehindEnvelopeFixture(t)
	second := transactionWriteBehindExecutionFixture(t, first, uuid.New())
	first.Dependencies = []TransactionEvidenceReference{transactionEvidenceReference(TransactionDependencyPredecessor, second)}
	second.Dependencies = []TransactionEvidenceReference{transactionEvidenceReference(TransactionDependencyPredecessor, first)}
	resolver := &transactionEvidenceResolverStub{records: map[string]*TransactionWriteBehindEnvelope{
		transactionCompletionEvidenceIdentity(first.Record.TransactionID, first.Record.ExecutionID):   &first,
		transactionCompletionEvidenceIdentity(second.Record.TransactionID, second.Record.ExecutionID): &second,
	}}

	_, err := CompleteTransactionWriteBehind(context.Background(), &first, resolver, &orderedAppliedTransactionCompleter{})
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)

	wrong := second
	wrong.Record.ExecutionID = uuid.New()
	resolver.records[transactionCompletionEvidenceIdentity(second.Record.TransactionID, second.Record.ExecutionID)] = &wrong
	_, err = CompleteTransactionWriteBehind(context.Background(), &first, resolver, &orderedAppliedTransactionCompleter{})
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	require.False(t, errors.Is(err, context.Canceled))
}

func transactionWriteBehindExecutionFixture(t testing.TB, base TransactionWriteBehindEnvelope, executionID uuid.UUID) TransactionWriteBehindEnvelope {
	t.Helper()
	plan, err := DecodeTransactionCompletionPlan([]byte(base.Record.Payload))
	require.NoError(t, err)
	plan.ExecutionID = executionID
	plan.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(*plan))
	require.NoError(t, err)
	base.Record = recoveryContractEnvelope(t, *plan, base.Record.Result)
	base.Dependencies = []TransactionEvidenceReference{}

	return base
}

func transactionEvidenceReference(kind string, envelope TransactionWriteBehindEnvelope) TransactionEvidenceReference {
	return TransactionEvidenceReference{
		Kind: kind, TenantID: envelope.Record.TenantID, OrganizationID: envelope.Record.OrganizationID,
		LedgerID: envelope.Record.LedgerID, TransactionID: envelope.Record.TransactionID, ExecutionID: envelope.Record.ExecutionID,
	}
}
