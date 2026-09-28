// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// stubReserver is an in-memory contextual Tracer: it serves facts built from the
// prepared entries, allows every admission unless reserveErr is set, and
// records what the reservation lifecycle asked of it.
// fixedIntegrationToken is the bearer token the REST seam tests present.
type fixedIntegrationToken struct{}

func (fixedIntegrationToken) Token(context.Context) (string, error) { return "integration-token", nil }

type stubReserver struct {
	mu sync.Mutex

	reserveCalls  int
	confirmedTxns []uuid.UUID
	releasedTxns  []uuid.UUID

	reserveErr error
}

// attach installs the stub as the use case's tracer coordinator.
func (s *stubReserver) attach(t *testing.T, uc *command.UseCase) {
	t.Helper()

	uc.ContextTracer = newTestContextCoordinator(t, s, s)
}

func (s *stubReserver) EvaluationContext(_ context.Context, _, _ uuid.UUID, entries []tracer.PreparedEntry) (tracercontract.Context, error) {
	facts := tracercontract.Context{Accounts: []tracercontract.Account{}, Entries: []tracercontract.Entry{}}
	seen := map[uuid.UUID]bool{}

	for _, entry := range entries {
		if !entry.External && !seen[entry.AccountID] {
			seen[entry.AccountID] = true
			blocked := false
			facts.Accounts = append(facts.Accounts, tracercontract.Account{ID: entry.AccountID, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: entry.AssetCode})
		}

		facts.Entries = append(facts.Entries, tracercontract.Entry{AccountID: entry.AccountID, External: entry.External, Direction: entry.Direction, Amount: tracercontract.Amount(entry.Amount.String()), Asset: entry.AssetCode})
	}

	return facts, nil
}

func (s *stubReserver) Reserve(_ context.Context, request tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reserveCalls++

	if s.reserveErr != nil {
		return nil, s.reserveErr
	}

	rules := tracercontract.RulesNotRequested
	if request.ValidationMode == tracercontract.ValidationRulesAndLimits {
		rules = tracercontract.RulesEvaluated
	}

	return &tracercontract.ReserveResult{
		ContractRevision: request.ContractRevision, TransactionID: request.TransactionID,
		EvaluationID: uuid.MustParse("487458bf-78f8-4f83-84f4-3eb35b66db83"), Decision: tracercontract.DecisionAllow,
		Controls:       tracercontract.ReserveControls{Rules: rules, Limits: tracercontract.LimitsEvaluated},
		ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied},
	}, nil
}

func (s *stubReserver) ConfirmByTransaction(_ context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.confirmedTxns = append(s.confirmedTxns, transactionID)

	return &tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transactionID, Status: "CONFIRMED"}, nil
}

func (s *stubReserver) ReleaseByTransaction(_ context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.releasedTxns = append(s.releasedTxns, transactionID)

	return &tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transactionID, Status: "RELEASED"}, nil
}

// forbiddenReserver fails the test on ANY call. It is the direct proof the route gate
// is asked for: asserting a zero call count only shows the stub was not invoked, while
// this shows the seam could not have reached a transport at all.
type forbiddenReserver struct {
	t *testing.T
}

// attach installs the forbidding tracer as the use case's tracer coordinator.
func (f *forbiddenReserver) attach(t *testing.T, uc *command.UseCase) {
	t.Helper()

	uc.ContextTracer = newTestContextCoordinator(t, f, f)
}

func (f *forbiddenReserver) fail(method string) {
	f.t.Helper()
	f.t.Fatalf("a /v1 route reached the tracer via %s — the route gate must return before any transport call", method)
}

func (f *forbiddenReserver) EvaluationContext(context.Context, uuid.UUID, uuid.UUID, []tracer.PreparedEntry) (tracercontract.Context, error) {
	f.fail("EvaluationContext")

	return tracercontract.Context{}, nil
}

func (f *forbiddenReserver) Reserve(context.Context, tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
	f.fail("Reserve")

	return nil, nil
}

func (f *forbiddenReserver) ConfirmByTransaction(context.Context, uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	f.fail("ConfirmByTransaction")

	return nil, nil
}

func (f *forbiddenReserver) ReleaseByTransaction(context.Context, uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	f.fail("ReleaseByTransaction")

	return nil, nil
}

func newTestContextCoordinator(t *testing.T, client command.ContextTracerClient, facts command.TracerFactsLoader) *command.ContextTracerCoordinator {
	t.Helper()

	coordinator, err := command.NewContextTracerCoordinator(client, facts, command.ContextTracerConfig{
		Bounds:           tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128},
		MaxReservations:  100,
		AdmissionTimeout: 5 * time.Second,
	}, time.Now)
	require.NoError(t, err)

	return coordinator
}
