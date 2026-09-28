// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// stubContextTracer is an in-memory Tracer behind a real coordinator: it
// serves official facts built from the prepared entries, answers Reserve with a
// scripted decision, and records every call so lifecycle tests can assert what
// the ledger sent and in which order.
type stubContextTracer struct {
	mu sync.Mutex

	decision tracercontract.Decision
	// decisions, when set, scripts the decision per Reserve call in order;
	// calls past its end use decision.
	decisions   []tracercontract.Decision
	reserveErr  error
	completeErr error

	reserved    []tracercontract.ReserveRequest
	confirmedTx []uuid.UUID
	releasedTx  []uuid.UUID
}

// tracerTestEpoch is the fixed admission time of the tracer command tests.
var tracerTestEpoch = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

// fixedTracerClock is the coordinator clock of the tracer command tests.
func fixedTracerClock() time.Time { return tracerTestEpoch }

// stubContextTracerTimeoutMs is a TimeoutMs inside the ledger supported range.
const stubContextTracerTimeoutMs = 250

// coordinatorFor wires the stub into a coordinator with test bounds.
func (s *stubContextTracer) coordinatorFor(t *testing.T) *ContextTracerCoordinator {
	t.Helper()

	coordinator, err := NewContextTracerCoordinator(s, s, ContextTracerConfig{
		Bounds:           tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128},
		MaxReservations:  100,
		AdmissionTimeout: time.Second,
	}, fixedTracerClock)
	require.NoError(t, err)

	return coordinator
}

func (s *stubContextTracer) EvaluationContext(_ context.Context, _, _ uuid.UUID, entries []traceradapter.PreparedEntry) (tracercontract.Context, error) {
	facts := tracercontract.Context{Accounts: []tracercontract.Account{}, Entries: []tracercontract.Entry{}}
	seen := map[uuid.UUID]bool{}

	for _, entry := range entries {
		if entry.External {
			facts.Entries = append(facts.Entries, tracercontract.Entry{External: true, Direction: entry.Direction, Amount: tracercontract.Amount(entry.Amount.String()), Asset: entry.AssetCode})
			continue
		}

		if !seen[entry.AccountID] {
			seen[entry.AccountID] = true
			blocked := false
			facts.Accounts = append(facts.Accounts, tracercontract.Account{ID: entry.AccountID, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: entry.AssetCode})
		}

		facts.Entries = append(facts.Entries, tracercontract.Entry{AccountID: entry.AccountID, Direction: entry.Direction, Amount: tracercontract.Amount(entry.Amount.String()), Asset: entry.AssetCode})
	}

	return facts, nil
}

func (s *stubContextTracer) Reserve(_ context.Context, request tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reserved = append(s.reserved, request)

	if s.reserveErr != nil {
		return nil, s.reserveErr
	}

	decision, reason := tracercontract.DecisionAllow, tracercontract.ReasonLimitsSatisfied

	scripted := s.decision
	if call := len(s.reserved) - 1; call < len(s.decisions) {
		scripted = s.decisions[call]
	}

	switch scripted {
	case tracercontract.DecisionDeny:
		decision, reason = tracercontract.DecisionDeny, tracercontract.ReasonLimitExceeded
	case tracercontract.DecisionReview:
		decision, reason = tracercontract.DecisionReview, tracercontract.ReasonRuleReview
	}

	rules := tracercontract.RulesNotRequested
	if request.ValidationMode == tracercontract.ValidationRulesAndLimits {
		rules = tracercontract.RulesEvaluated
	}

	return &tracercontract.ReserveResult{
		ContractRevision: request.ContractRevision, TransactionID: request.TransactionID,
		EvaluationID: uuid.MustParse("487458bf-78f8-4f83-84f4-3eb35b66db83"), Decision: decision,
		Controls:       tracercontract.ReserveControls{Rules: rules, Limits: tracercontract.LimitsEvaluated},
		ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{reason},
	}, nil
}

func (s *stubContextTracer) ConfirmByTransaction(_ context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.confirmedTx = append(s.confirmedTx, transactionID)

	if s.completeErr != nil {
		return nil, s.completeErr
	}

	return completionResult(transactionID, "CONFIRMED"), nil
}

func (s *stubContextTracer) ReleaseByTransaction(_ context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.releasedTx = append(s.releasedTx, transactionID)

	if s.completeErr != nil {
		return nil, s.completeErr
	}

	return completionResult(transactionID, "RELEASED"), nil
}

func (s *stubContextTracer) reserves() []tracercontract.ReserveRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]tracercontract.ReserveRequest(nil), s.reserved...)
}

func (s *stubContextTracer) confirmedTransactions() []uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]uuid.UUID(nil), s.confirmedTx...)
}

func (s *stubContextTracer) releasedTransactions() []uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]uuid.UUID(nil), s.releasedTx...)
}

// snapshot returns the reserve requests and the transactions confirmed and
// released, in call order.
func (s *stubContextTracer) snapshot() ([]tracercontract.ReserveRequest, []uuid.UUID, []uuid.UUID) {
	return s.reserves(), s.confirmedTransactions(), s.releasedTransactions()
}
