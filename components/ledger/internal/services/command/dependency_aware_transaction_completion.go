// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

const (
	maxTransactionCompletionDependencyDepth   = 3
	maxTransactionCompletionDependencyRecords = 8
)

// TransactionEvidenceResolver performs bounded point resolution of immutable
// accounting evidence. Implementations must use the authenticated reference;
// callers never supply arbitrary persistence rows.
type TransactionEvidenceResolver interface {
	ResolveTransactionEvidence(context.Context, TransactionEvidenceReference) (*TransactionWriteBehindEnvelope, error)
}

// DependencyAwareTransactionCompletionResult reports every durable unit in
// causal order and the outcome for the requested execution.
type DependencyAwareTransactionCompletionResult struct {
	Dependencies []TransactionCompletionResult
	Current      TransactionCompletionResult
}

func completeTransactionWriteBehindFallback(
	ctx context.Context,
	envelope *TransactionWriteBehindEnvelope,
	resolver TransactionEvidenceResolver,
	completer AppliedTransactionCompleter,
) (TransactionCompletionResult, error) {
	if envelope == nil {
		return TransactionCompletionResult{}, invalidTransactionCompletionRecord("transaction write-behind envelope is missing")
	}

	if len(envelope.Dependencies) == 0 {
		return completer.Complete(ctx, &envelope.Record)
	}

	completed, err := CompleteTransactionWriteBehind(ctx, envelope, resolver, completer)
	if err != nil {
		return TransactionCompletionResult{}, err
	}

	return completed.Current, nil
}

// CompleteTransactionWriteBehind resolves and verifies predecessor/origin
// evidence before completing the requested execution. Resolution is bounded,
// cycle checked, scope checked by the envelope codec, and never invokes the
// accounting engine.
func CompleteTransactionWriteBehind(
	ctx context.Context,
	envelope *TransactionWriteBehindEnvelope,
	resolver TransactionEvidenceResolver,
	completer AppliedTransactionCompleter,
) (DependencyAwareTransactionCompletionResult, error) {
	var outcome DependencyAwareTransactionCompletionResult
	if err := ctx.Err(); err != nil {
		return outcome, err
	}

	if envelope == nil || completer == nil {
		return outcome, invalidTransactionCompletionRecord("dependency-aware completion is not configured")
	}

	if err := validateTransactionWriteBehindEnvelope(*envelope); err != nil {
		return outcome, err
	}

	if len(envelope.Dependencies) > 0 && resolver == nil {
		return outcome, invalidTransactionCompletionRecord("transaction evidence resolver is not configured")
	}

	ordered := make([]*TransactionCompletionRecord, 0, len(envelope.Dependencies)+1)
	visited := make(map[string]struct{}, len(envelope.Dependencies)+1)

	active := make(map[string]struct{}, len(envelope.Dependencies)+1)
	if err := resolveTransactionCompletionOrder(ctx, *envelope, resolver, 0, visited, active, &ordered); err != nil {
		return outcome, err
	}

	for index, record := range ordered {
		if err := ctx.Err(); err != nil {
			return outcome, err
		}

		completed, err := completer.Complete(ctx, record)
		if err != nil {
			return outcome, fmt.Errorf("complete causal transaction %s:%s: %w", record.TransactionID, record.ExecutionID, err)
		}

		if index == len(ordered)-1 {
			outcome.Current = completed
		} else {
			outcome.Dependencies = append(outcome.Dependencies, completed)
		}
	}

	return outcome, nil
}

// CompleteTransactionWriteBehindBulk resolves all dependencies across a
// same-scope broker group, deduplicates shared evidence, and projects the
// resulting records once. Returned results correspond to the input envelopes,
// not to the expanded dependency list.
func CompleteTransactionWriteBehindBulk(
	ctx context.Context,
	envelopes []*TransactionWriteBehindEnvelope,
	resolver TransactionEvidenceResolver,
	completer AppliedTransactionBulkCompleter,
) ([]TransactionCompletionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(envelopes) == 0 {
		return []TransactionCompletionResult{}, nil
	}

	if completer == nil {
		return nil, invalidTransactionCompletionRecord("dependency-aware bulk completion is not configured")
	}

	ordered := make([]*TransactionCompletionRecord, 0, len(envelopes))
	groupSeen := make(map[string]struct{}, len(envelopes))

	currentIdentities := make([]string, len(envelopes))
	for index, envelope := range envelopes {
		if envelope == nil {
			return nil, invalidTransactionCompletionRecord("bulk transaction envelope is missing")
		}

		if err := validateTransactionWriteBehindEnvelope(*envelope); err != nil {
			return nil, err
		}

		if len(envelope.Dependencies) > 0 && resolver == nil {
			return nil, invalidTransactionCompletionRecord("transaction evidence resolver is not configured")
		}

		currentIdentities[index] = transactionCompletionEvidenceIdentity(envelope.Record.TransactionID, envelope.Record.ExecutionID)

		dependencyOrder := make([]*TransactionCompletionRecord, 0, len(envelope.Dependencies)+1)
		if err := resolveTransactionCompletionOrder(ctx, *envelope, resolver, 0, make(map[string]struct{}), make(map[string]struct{}), &dependencyOrder); err != nil {
			return nil, err
		}

		for _, record := range dependencyOrder {
			identity := transactionCompletionEvidenceIdentity(record.TransactionID, record.ExecutionID)
			if _, exists := groupSeen[identity]; exists {
				continue
			}

			groupSeen[identity] = struct{}{}

			ordered = append(ordered, record)
		}
	}

	completed, err := completer.CompleteBulk(ctx, ordered)
	if err != nil {
		return nil, err
	}

	if len(completed) != len(ordered) {
		return nil, invalidTransactionCompletionRecord("bulk completer returned an uncorrelated result")
	}

	byIdentity := make(map[string]TransactionCompletionResult, len(ordered))
	for index, record := range ordered {
		byIdentity[transactionCompletionEvidenceIdentity(record.TransactionID, record.ExecutionID)] = completed[index]
	}

	results := make([]TransactionCompletionResult, len(envelopes))

	for index, identity := range currentIdentities {
		result, ok := byIdentity[identity]
		if !ok {
			return nil, invalidTransactionCompletionRecord("bulk completion omitted a requested execution")
		}

		results[index] = result
	}

	return results, nil
}

func resolveTransactionCompletionOrder(
	ctx context.Context,
	envelope TransactionWriteBehindEnvelope,
	resolver TransactionEvidenceResolver,
	depth int,
	visited, active map[string]struct{},
	ordered *[]*TransactionCompletionRecord,
) error {
	if depth > maxTransactionCompletionDependencyDepth || len(visited) >= maxTransactionCompletionDependencyRecords {
		return invalidTransactionCompletionRecord("transaction completion dependency bound exceeded")
	}

	identity := transactionCompletionEvidenceIdentity(envelope.Record.TransactionID, envelope.Record.ExecutionID)
	if _, exists := active[identity]; exists {
		return invalidTransactionCompletionRecord("cyclic transaction completion dependency")
	}

	if _, exists := visited[identity]; exists {
		return nil
	}

	active[identity] = struct{}{}
	defer delete(active, identity)

	for _, reference := range envelope.Dependencies {
		if err := ctx.Err(); err != nil {
			return err
		}

		resolved, err := resolver.ResolveTransactionEvidence(ctx, reference)
		if err != nil {
			return fmt.Errorf("resolve transaction evidence %s:%s: %w", reference.TransactionID, reference.ExecutionID, err)
		}

		if resolved == nil || resolved.Record.TenantID != reference.TenantID ||
			resolved.Record.OrganizationID != reference.OrganizationID || resolved.Record.LedgerID != reference.LedgerID ||
			resolved.Record.TransactionID != reference.TransactionID || resolved.Record.ExecutionID != reference.ExecutionID {
			return invalidTransactionCompletionRecord("resolved transaction evidence differs from dependency reference")
		}

		if err := validateTransactionWriteBehindEnvelope(*resolved); err != nil {
			return err
		}

		if err := resolveTransactionCompletionOrder(ctx, *resolved, resolver, depth+1, visited, active, ordered); err != nil {
			return err
		}
	}

	if len(visited) >= maxTransactionCompletionDependencyRecords {
		return invalidTransactionCompletionRecord("transaction completion dependency bound exceeded")
	}

	record := envelope.Record
	*ordered = append(*ordered, &record)
	visited[identity] = struct{}{}

	return nil
}

func transactionCompletionEvidenceIdentity(transactionID, executionID uuid.UUID) string {
	return transactionID.String() + ":" + executionID.String()
}
