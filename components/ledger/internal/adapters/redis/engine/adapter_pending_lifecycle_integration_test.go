//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	postgresTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	txredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type pendingLifecycleReader struct {
	command.TransactionReader
	balances  []*mmodel.Balance
	persisted *postgresTransaction.Transaction
	settings  mmodel.LedgerSettings
}

func (r *pendingLifecycleReader) GetParsedLedgerSettings(context.Context, uuid.UUID, uuid.UUID) (mmodel.LedgerSettings, error) {
	return r.settings, nil
}

func (r *pendingLifecycleReader) GetBalances(_ context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	out := make([]*mmodel.Balance, 0, len(aliases))
	for _, alias := range aliases {
		for _, balance := range r.balances {
			if mtransaction.AliasKey(balance.Alias, balance.Key) == alias {
				out = append(out, balance)
			}
		}
	}
	return out, nil
}

func (r *pendingLifecycleReader) GetBalanceEngineBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []*mmodel.Balance, error) {
	balances, err := r.GetBalances(ctx, organizationID, ledgerID, aliases)
	return balances, balances, err
}

func (r *pendingLifecycleReader) ValidateAccountingRules(context.Context, uuid.UUID, uuid.UUID, []mmodel.BalanceOperation, *mtransaction.Responses, string) (*mmodel.TransactionRouteCache, error) {
	return nil, nil
}

func (r *pendingLifecycleReader) GetWriteBehindTransaction(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*postgresTransaction.Transaction, error) {
	return clonePendingLifecycleTransaction(r.persisted), nil
}

func (r *pendingLifecycleReader) GetTransactionWithOperationsByID(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*postgresTransaction.Transaction, error) {
	return clonePendingLifecycleTransaction(r.persisted), nil
}

func clonePendingLifecycleTransaction(input *postgresTransaction.Transaction) *postgresTransaction.Transaction {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.Body.Send.Source.From = append([]mtransaction.FromTo(nil), input.Body.Send.Source.From...)
	cloned.Body.Send.Distribute.To = append([]mtransaction.FromTo(nil), input.Body.Send.Distribute.To...)
	cloned.Operations = append(cloned.Operations[:0:0], input.Operations...)
	return &cloned
}

type pendingLifecycleClientProvider struct {
	client redis.UniversalClient
}

func (p pendingLifecycleClientProvider) GetClient(context.Context) (redis.UniversalClient, error) {
	return p.client, nil
}

type pendingLifecycleAdapter struct {
	delegate   *Adapter
	executions []command.EngineExecution
	bootstraps []command.ExecutionGuard
}

type racingPendingLifecycleAdapter struct {
	delegate *Adapter

	mu         sync.Mutex
	armed      bool
	executions []command.EngineExecution
	bootstraps []command.ExecutionGuard
	arrived    chan struct{}
	release    chan struct{}
}

func (a *racingPendingLifecycleAdapter) Execute(ctx context.Context, execution command.EngineExecution) (*core.ExecutionResult, error) {
	a.mu.Lock()
	a.executions = append(a.executions, execution)
	armed := a.armed
	a.mu.Unlock()
	if armed {
		a.arrived <- struct{}{}
		<-a.release
	}
	return a.delegate.Execute(ctx, execution)
}

func (a *racingPendingLifecycleAdapter) EnsureTransactionGuard(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, nextToken string) error {
	a.mu.Lock()
	a.bootstraps = append(a.bootstraps, command.ExecutionGuard{TransactionID: transactionID, NextToken: nextToken})
	a.mu.Unlock()
	return a.delegate.EnsureTransactionGuard(ctx, organizationID, ledgerID, transactionID, nextToken)
}

func (a *racingPendingLifecycleAdapter) arm() {
	a.mu.Lock()
	a.armed = true
	a.mu.Unlock()
}

func (a *racingPendingLifecycleAdapter) captured() ([]command.EngineExecution, []command.ExecutionGuard) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]command.EngineExecution(nil), a.executions...), append([]command.ExecutionGuard(nil), a.bootstraps...)
}

func (a *pendingLifecycleAdapter) Execute(ctx context.Context, execution command.EngineExecution) (*core.ExecutionResult, error) {
	a.executions = append(a.executions, execution)
	return a.delegate.Execute(ctx, execution)
}

func (a *pendingLifecycleAdapter) EnsureTransactionGuard(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, nextToken string) error {
	a.bootstraps = append(a.bootstraps, command.ExecutionGuard{TransactionID: transactionID, NextToken: nextToken})
	return a.delegate.EnsureTransactionGuard(ctx, organizationID, ledgerID, transactionID, nextToken)
}

type pendingLifecycleFinalizer struct {
	outcomes  []string
	envelopes []*command.TransactionCompletionRecord
}

type pendingRaceFinalizer struct {
	envelopes []*command.TransactionCompletionRecord
}

type pendingLockExpiryFinalizer struct {
	pendingRaceFinalizer
	failAfter int
	err       error
}

func (f *pendingLockExpiryFinalizer) Complete(ctx context.Context, envelope *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	result, err := f.pendingRaceFinalizer.Complete(ctx, envelope)
	if err != nil {
		return command.TransactionCompletionResult{}, err
	}
	if len(f.envelopes) > f.failAfter {
		return command.TransactionCompletionResult{}, f.err
	}
	return result, nil
}

func (f *pendingRaceFinalizer) Complete(_ context.Context, envelope *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	payload, err := command.DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	if err != nil {
		return command.TransactionCompletionResult{}, err
	}
	f.envelopes = append(f.envelopes, envelope)
	record, err := command.BuildTransactionWriteSet(*payload, envelope.Result)
	if err != nil {
		return command.TransactionCompletionResult{}, err
	}

	return command.TransactionCompletionResult{
		Record:  record,
		Outcome: command.TransactionPersistenceOutcome{TransactionStatus: payload.TransactionStatus},
	}, nil
}

func (f *pendingLifecycleFinalizer) Complete(_ context.Context, envelope *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	f.envelopes = append(f.envelopes, envelope)
	payload, err := command.DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	if err != nil {
		return command.TransactionCompletionResult{}, err
	}
	record, err := command.BuildTransactionWriteSet(*payload, envelope.Result)
	if err != nil {
		return command.TransactionCompletionResult{}, err
	}

	return command.TransactionCompletionResult{
		Record:  record,
		Outcome: command.TransactionPersistenceOutcome{TransactionStatus: f.outcomes[len(f.envelopes)-1]},
	}, nil
}

type pendingLifecycleTracer struct {
	reserveRequests []tracer.ReserveRequest
	confirmedIDs    []uuid.UUID
	releasedIDs     []uuid.UUID
	confirmedTxns   []uuid.UUID
	releasedTxns    []uuid.UUID
	reservationID   uuid.UUID
}

func (s *pendingLifecycleTracer) Reserve(_ context.Context, request tracer.ReserveRequest) (*tracer.ReserveResult, error) {
	s.reserveRequests = append(s.reserveRequests, request)
	return &tracer.ReserveResult{TransactionID: request.TransactionID, ReservationIDs: []uuid.UUID{s.reservationID}}, nil
}

func (s *pendingLifecycleTracer) Confirm(_ context.Context, reservationID uuid.UUID) error {
	s.confirmedIDs = append(s.confirmedIDs, reservationID)
	return nil
}

func (s *pendingLifecycleTracer) Release(_ context.Context, reservationID uuid.UUID) error {
	s.releasedIDs = append(s.releasedIDs, reservationID)
	return nil
}

func (s *pendingLifecycleTracer) ConfirmByTransaction(_ context.Context, transactionID uuid.UUID) error {
	s.confirmedTxns = append(s.confirmedTxns, transactionID)
	return nil
}

func (s *pendingLifecycleTracer) ReleaseByTransaction(_ context.Context, transactionID uuid.UUID) error {
	s.releasedTxns = append(s.releasedTxns, transactionID)
	return nil
}

func TestIntegration_CreatePendingV2ThenTransitionWithRealAdapter(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	for _, test := range []struct {
		name              string
		terminalStatus    string
		transition        func(*command.UseCase, context.Context, command.PendingTransitionInput) (*postgresTransaction.Transaction, error)
		transitionTypes   []core.PostingType
		expectedBalances  []pendingLifecycleBalanceExpectation
		absentBalanceRefs []string
	}{
		{
			name:           "commit confirms reservation and moves held funds",
			terminalStatus: constant.APPROVED,
			transition:     (*command.UseCase).CommitTransactionV2,
			transitionTypes: []core.PostingType{
				core.PostingUnreserve,
				core.PostingCredit,
			},
			expectedBalances: []pendingLifecycleBalanceExpectation{
				{ref: "@source#default", available: "70", onHold: "0", version: 9},
				{ref: "@target#default", available: "50", onHold: "0", version: 4},
			},
		},
		{
			name:            "cancel releases reservation and held funds",
			terminalStatus:  constant.CANCELED,
			transition:      (*command.UseCase).CancelTransactionV2,
			transitionTypes: []core.PostingType{core.PostingRelease},
			expectedBalances: []pendingLifecycleBalanceExpectation{
				{ref: "@source#default", available: "100", onHold: "0", version: 9},
			},
			absentBalanceRefs: []string{"@target#default"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-pending-lifecycle")
			ctx = libObservability.ContextWithHeaderID(ctx, "request-pending-lifecycle")
			client, _, _ := newAdapterValkey(t)
			organizationID := uuid.MustParse("91111111-1111-4111-8111-111111111111")
			ledgerID := uuid.MustParse("92222222-2222-4222-8222-222222222222")
			reader := &pendingLifecycleReader{
				settings: mmodel.LedgerSettings{Tracer: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce}},
				balances: []*mmodel.Balance{
					adapterCreateBalance(organizationID, ledgerID, "93333333-3333-4333-8333-333333333333", "94444444-4444-4444-8444-444444444444", "@source", 100, 7),
					adapterCreateBalance(organizationID, ledgerID, "95555555-5555-4555-8555-555555555555", "96666666-6666-4666-8666-666666666666", "@target", 20, 3),
				},
			}

			ctrl := gomock.NewController(t)
			redisRepository := txredis.NewMockRedisRepository(ctrl)
			stored := make(chan struct{})
			redisRepository.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
			redisRepository.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
				func(context.Context, string, string, time.Duration) error { close(stored); return nil },
			)
			redisRepository.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

			realAdapter, err := NewAdapter(&integrationClientProvider{client: client}, guardBootstrapLimits())
			require.NoError(t, err)
			executor := &pendingLifecycleAdapter{delegate: realAdapter}
			finalizer := &pendingLifecycleFinalizer{outcomes: []string{constant.PENDING, test.terminalStatus}}
			reservationID := uuid.MustParse("97777777-7777-4777-8777-777777777777")
			tracerControl := &pendingLifecycleTracer{reservationID: reservationID}
			uc := &command.UseCase{
				TransactionRedisRepo:        redisRepository,
				TransactionReader:           reader,
				BalanceEngine:               executor,
				AppliedTransactionCompleter: finalizer,
				TracerReserver:              tracerControl,
			}

			amount := decimal.NewFromInt(30)
			body := mtransaction.Transaction{
				Description: "pending lifecycle",
				Pending:     true,
				Send: mtransaction.Send{
					Asset: "USD",
					Value: amount,
					Source: mtransaction.Source{From: []mtransaction.FromTo{{
						AccountAlias: "@source",
						Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
					}}},
					Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
						AccountAlias: "@target",
						Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
					}}},
				},
			}

			pending, replayed, err := uc.CreateTransactionV2(ctx, command.CreateTransactionV2Input{
				OrganizationID:    organizationID,
				LedgerID:          ledgerID,
				Transaction:       body,
				TransactionStatus: constant.PENDING,
				IdempotencyTTL:    time.Minute,
			})
			require.NoError(t, err)
			require.False(t, replayed)
			require.Equal(t, constant.PENDING, pending.Status.Code)
			require.Len(t, pending.Operations, 1)
			require.Len(t, executor.executions, 1)
			require.Equal(t, []core.PostingType{core.PostingHold}, pendingLifecyclePostingTypes(executor.executions[0]))
			require.Len(t, tracerControl.reserveRequests, 1)
			require.True(t, tracerControl.reserveRequests[0].LongLived)
			require.Equal(t, pending.ID, tracerControl.reserveRequests[0].TransactionID.String())
			require.Empty(t, tracerControl.confirmedIDs)
			require.Empty(t, tracerControl.releasedIDs)

			select {
			case <-stored:
			case <-time.After(time.Second):
				t.Fatal("pending create did not populate idempotency")
			}

			reader.persisted = pending
			reader.balances[0].Available = decimal.NewFromInt(70)
			reader.balances[0].OnHold = decimal.NewFromInt(30)
			reader.balances[0].Version = 8
			transitioned, err := test.transition(uc, ctx, command.PendingTransitionInput{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				TransactionID:  uuid.MustParse(pending.ID),
			})
			require.NoError(t, err)
			require.Equal(t, pending.ID, transitioned.ID)
			require.Equal(t, test.terminalStatus, transitioned.Status.Code)
			require.Len(t, executor.executions, 2)
			require.Equal(t, test.transitionTypes, pendingLifecyclePostingTypes(executor.executions[1]))
			require.Len(t, executor.bootstraps, 1)
			require.Equal(t, command.ExecutionGuard{TransactionID: uuid.MustParse(pending.ID), NextToken: constant.PENDING}, executor.bootstraps[0])

			createExecution, transitionExecution := executor.executions[0], executor.executions[1]
			require.NotEqual(t, createExecution.Execution.ExecutionID, transitionExecution.Execution.ExecutionID)
			require.Equal(t, pending.ID, createExecution.Execution.Transactions[0].ID.String())
			require.Equal(t, pending.ID, transitionExecution.Execution.Transactions[0].ID.String())
			require.Equal(t, command.ExecutionGuard{TransactionID: uuid.MustParse(pending.ID), NextToken: constant.PENDING}, createExecution.Guards[0])
			require.Equal(t, command.ExecutionGuard{TransactionID: uuid.MustParse(pending.ID), ExpectedToken: constant.PENDING, NextToken: test.terminalStatus}, transitionExecution.Guards[0])
			require.Len(t, finalizer.envelopes, 2)
			transactions := []*postgresTransaction.Transaction{pending, transitioned}
			for index, execution := range executor.executions {
				envelope := finalizer.envelopes[index]
				require.Equal(t, execution.Execution.ExecutionID, envelope.ExecutionID)
				require.Equal(t, execution.IntentFingerprint, envelope.IntentFingerprint)
				assertPendingLifecycleProjection(t, envelope, transactions[index])
			}

			keys, err := resolveAdapterKeys(ctx, transitionExecution.Execution)
			require.NoError(t, err)
			t.Cleanup(func() { deleteMultiTransactionAcceptanceState(t, client, keys) })
			require.Equal(t, test.terminalStatus, client.HGet(ctx, keys.Guards, pending.ID).Val())
			require.Equal(t, int64(1), client.HLen(ctx, keys.Guards).Val())
			require.Equal(t, int64(2), client.HLen(ctx, keys.Recovery).Val())
			require.Equal(t, int64(2), client.HLen(ctx, keys.Receipts).Val())
			assertPendingLifecycleBalances(t, ctx, client, keys, test.expectedBalances)
			for _, ref := range test.absentBalanceRefs {
				_, exists := keys.Balances[ref]
				require.False(t, exists)
			}
			assertPendingLifecycleRecovery(t, ctx, client, keys, createExecution, pending)
			assertPendingLifecycleRecovery(t, ctx, client, keys, transitionExecution, transitioned)

			if test.terminalStatus == constant.APPROVED {
				require.Equal(t, []uuid.UUID{uuid.MustParse(pending.ID)}, tracerControl.confirmedTxns)
				require.Empty(t, tracerControl.releasedTxns)
			} else {
				require.Empty(t, tracerControl.confirmedTxns)
				require.Equal(t, []uuid.UUID{uuid.MustParse(pending.ID)}, tracerControl.releasedTxns)
			}
			require.Empty(t, tracerControl.confirmedIDs)
			require.Empty(t, tracerControl.releasedIDs)
		})
	}
}

func TestIntegration_CreatePendingV2FencesConcurrentCommitAndCancel(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-pending-race")
	ctx = libObservability.ContextWithHeaderID(ctx, "request-pending-race")
	client, _, _ := newAdapterValkey(t)
	organizationID := uuid.MustParse("a1111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("a2222222-2222-4222-8222-222222222222")
	reader := &pendingLifecycleReader{
		settings: mmodel.LedgerSettings{Tracer: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce}},
		balances: []*mmodel.Balance{
			adapterCreateBalance(organizationID, ledgerID, "a3333333-3333-4333-8333-333333333333", "a4444444-4444-4444-8444-444444444444", "@source", 100, 7),
			adapterCreateBalance(organizationID, ledgerID, "a5555555-5555-4555-8555-555555555555", "a6666666-6666-4666-8666-666666666666", "@target", 20, 3),
		},
	}

	ctrl := gomock.NewController(t)
	redisRepository := txredis.NewMockRedisRepository(ctrl)
	stored := make(chan struct{})
	redisRepository.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
	redisRepository.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
		func(context.Context, string, string, time.Duration) error { close(stored); return nil },
	)
	redisRepository.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Duration(300)).Return(true, nil).Times(2)
	redisRepository.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	realAdapter, err := NewAdapter(pendingLifecycleClientProvider{client: client}, guardBootstrapLimits())
	require.NoError(t, err)
	executor := &racingPendingLifecycleAdapter{
		delegate: realAdapter,
		arrived:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	finalizer := &pendingRaceFinalizer{}
	tracerControl := &pendingLifecycleTracer{reservationID: uuid.MustParse("a7777777-7777-4777-8777-777777777777")}
	uc := &command.UseCase{
		TransactionRedisRepo:        redisRepository,
		TransactionReader:           reader,
		BalanceEngine:               executor,
		AppliedTransactionCompleter: finalizer,
		TracerReserver:              tracerControl,
	}

	amount := decimal.NewFromInt(30)
	pending, replayed, err := uc.CreateTransactionV2(ctx, command.CreateTransactionV2Input{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Transaction: mtransaction.Transaction{
			Description: "pending race",
			Pending:     true,
			Send: mtransaction.Send{
				Asset: "USD",
				Value: amount,
				Source: mtransaction.Source{From: []mtransaction.FromTo{{
					AccountAlias: "@source",
					Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
				}}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
					AccountAlias: "@target",
					Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
				}}},
			},
		},
		TransactionStatus: constant.PENDING,
		IdempotencyTTL:    time.Minute,
	})
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, constant.PENDING, pending.Status.Code)
	select {
	case <-stored:
	case <-time.After(time.Second):
		t.Fatal("pending create did not populate idempotency")
	}

	reader.persisted = pending
	reader.balances[0].Available = decimal.NewFromInt(70)
	reader.balances[0].OnHold = decimal.NewFromInt(30)
	reader.balances[0].Version = 8
	executor.arm()
	transitionInput := command.PendingTransitionInput{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		TransactionID:  uuid.MustParse(pending.ID),
	}
	type raceOutcome struct {
		requestedStatus string
		transaction     *postgresTransaction.Transaction
		err             error
	}
	outcomes := make(chan raceOutcome, 2)
	go func() {
		transaction, transitionErr := uc.CommitTransactionV2(ctx, transitionInput)
		outcomes <- raceOutcome{requestedStatus: constant.APPROVED, transaction: transaction, err: transitionErr}
	}()
	go func() {
		transaction, transitionErr := uc.CancelTransactionV2(ctx, transitionInput)
		outcomes <- raceOutcome{requestedStatus: constant.CANCELED, transaction: transaction, err: transitionErr}
	}()

	for range 2 {
		select {
		case <-executor.arrived:
		case <-time.After(time.Second):
			close(executor.release)
			t.Fatal("both pending transitions did not reach the adapter")
		}
	}
	close(executor.release)
	first, second := <-outcomes, <-outcomes
	var winner, loser raceOutcome
	if first.err == nil {
		winner, loser = first, second
	} else {
		winner, loser = second, first
	}
	require.NoError(t, winner.err)
	require.NotNil(t, winner.transaction)
	require.Equal(t, winner.requestedStatus, winner.transaction.Status.Code)
	require.Error(t, loser.err)
	require.Nil(t, loser.transaction)
	require.NotEqual(t, winner.requestedStatus, loser.requestedStatus)

	executions, bootstraps := executor.captured()
	require.Len(t, executions, 3)
	require.Len(t, bootstraps, 2)
	for _, bootstrap := range bootstraps {
		require.Equal(t, command.ExecutionGuard{TransactionID: uuid.MustParse(pending.ID), NextToken: constant.PENDING}, bootstrap)
	}
	var winningExecution, losingExecution command.EngineExecution
	for _, execution := range executions[1:] {
		require.Equal(t, pending.ID, execution.Execution.Transactions[0].ID.String())
		if execution.Guards[0].NextToken == winner.requestedStatus {
			winningExecution = execution
		} else {
			losingExecution = execution
		}
	}
	require.NotEqual(t, uuid.Nil, winningExecution.Execution.ExecutionID)
	require.NotEqual(t, uuid.Nil, losingExecution.Execution.ExecutionID)
	require.NotEqual(t, winningExecution.Execution.ExecutionID, losingExecution.Execution.ExecutionID)
	require.Len(t, finalizer.envelopes, 2)
	winnerEnvelope := finalizer.envelopes[1]
	require.Equal(t, winningExecution.Execution.ExecutionID, winnerEnvelope.ExecutionID)
	require.Equal(t, winningExecution.IntentFingerprint, winnerEnvelope.IntentFingerprint)
	assertPendingLifecycleProjection(t, winnerEnvelope, winner.transaction)

	keys, err := resolveAdapterKeys(ctx, winningExecution.Execution)
	require.NoError(t, err)
	t.Cleanup(func() { deleteMultiTransactionAcceptanceState(t, client, keys) })
	require.Equal(t, winner.requestedStatus, client.HGet(ctx, keys.Guards, pending.ID).Val())
	require.Equal(t, int64(1), client.HLen(ctx, keys.Guards).Val())
	require.Equal(t, int64(2), client.HLen(ctx, keys.Recovery).Val())
	require.Equal(t, int64(2), client.HLen(ctx, keys.Receipts).Val())
	require.False(t, client.HExists(ctx, keys.Recovery, pending.ID+":"+losingExecution.Execution.ExecutionID.String()).Val())
	require.False(t, client.HExists(ctx, keys.Receipts, losingExecution.Execution.ExecutionID.String()).Val())
	assertPendingLifecycleRecovery(t, ctx, client, keys, winningExecution, winner.transaction)

	if winner.requestedStatus == constant.APPROVED {
		require.Equal(t, []uuid.UUID{uuid.MustParse(pending.ID)}, tracerControl.confirmedTxns)
		require.Empty(t, tracerControl.releasedTxns)
		assertPendingLifecycleBalances(t, ctx, client, keys, []pendingLifecycleBalanceExpectation{
			{ref: "@source#default", available: "70", onHold: "0", version: 9},
			{ref: "@target#default", available: "50", onHold: "0", version: 4},
		})
	} else {
		require.Empty(t, tracerControl.confirmedTxns)
		require.Equal(t, []uuid.UUID{uuid.MustParse(pending.ID)}, tracerControl.releasedTxns)
		assertPendingLifecycleBalances(t, ctx, client, keys, []pendingLifecycleBalanceExpectation{
			{ref: "@source#default", available: "100", onHold: "0", version: 9},
		})
	}
	require.Len(t, tracerControl.reserveRequests, 1)
	require.Empty(t, tracerControl.confirmedIDs)
	require.Empty(t, tracerControl.releasedIDs)
}

func TestIntegration_PendingTransitionGuardFencesRetriesAfterGoLockExpiry(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-pending-lock-expiry")
	ctx = libObservability.ContextWithHeaderID(ctx, "request-pending-lock-expiry")
	client, _, _ := newAdapterValkey(t)
	organizationID := uuid.MustParse("b1111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("b2222222-2222-4222-8222-222222222222")
	reader := &pendingLifecycleReader{
		balances: []*mmodel.Balance{
			adapterCreateBalance(organizationID, ledgerID, "b3333333-3333-4333-8333-333333333333", "b4444444-4444-4444-8444-444444444444", "@source", 100, 7),
			adapterCreateBalance(organizationID, ledgerID, "b5555555-5555-4555-8555-555555555555", "b6666666-6666-4666-8666-666666666666", "@target", 20, 3),
		},
	}

	ctrl := gomock.NewController(t)
	redisRepository := txredis.NewMockRedisRepository(ctrl)
	stored := make(chan struct{})
	redisRepository.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
	redisRepository.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
		func(context.Context, string, string, time.Duration) error { close(stored); return nil },
	)
	lockAcquisitions := 0
	redisRepository.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Duration(300)).DoAndReturn(
		func(context.Context, string, string, time.Duration) (bool, error) {
			lockAcquisitions++
			return true, nil
		},
	).Times(3)
	redisRepository.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	realAdapter, err := NewAdapter(pendingLifecycleClientProvider{client: client}, guardBootstrapLimits())
	require.NoError(t, err)
	executor := &pendingLifecycleAdapter{delegate: realAdapter}
	finalizationErr := errors.New("pending transition persistence unavailable")
	finalizer := &pendingLockExpiryFinalizer{failAfter: 1, err: finalizationErr}
	uc := &command.UseCase{
		TransactionRedisRepo:        redisRepository,
		TransactionReader:           reader,
		BalanceEngine:               executor,
		AppliedTransactionCompleter: finalizer,
	}

	amount := decimal.NewFromInt(30)
	pending, replayed, err := uc.CreateTransactionV2(ctx, command.CreateTransactionV2Input{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Transaction: mtransaction.Transaction{
			Description: "pending lock expiry",
			Pending:     true,
			Send: mtransaction.Send{
				Asset: "USD",
				Value: amount,
				Source: mtransaction.Source{From: []mtransaction.FromTo{{
					AccountAlias: "@source",
					Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
				}}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
					AccountAlias: "@target",
					Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
				}}},
			},
		},
		TransactionStatus: constant.PENDING,
		IdempotencyTTL:    time.Minute,
	})
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, constant.PENDING, pending.Status.Code)
	select {
	case <-stored:
	case <-time.After(time.Second):
		t.Fatal("pending create did not populate idempotency")
	}

	reader.persisted = pending
	reader.balances[0].Available = decimal.NewFromInt(70)
	reader.balances[0].OnHold = decimal.NewFromInt(30)
	reader.balances[0].Version = 8
	transitionInput := command.PendingTransitionInput{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		TransactionID:  uuid.MustParse(pending.ID),
	}

	transitioned, err := uc.CommitTransactionV2(ctx, transitionInput)
	require.ErrorIs(t, err, finalizationErr)
	require.Nil(t, transitioned)
	require.Equal(t, 1, lockAcquisitions)
	require.Len(t, executor.executions, 2)
	require.Len(t, finalizer.envelopes, 2)
	winningExecution := executor.executions[1]
	keys, err := resolveAdapterKeys(ctx, winningExecution.Execution)
	require.NoError(t, err)
	t.Cleanup(func() { deleteMultiTransactionAcceptanceState(t, client, keys) })
	require.Equal(t, constant.APPROVED, client.HGet(ctx, keys.Guards, pending.ID).Val())
	assertPendingLifecycleBalances(t, ctx, client, keys, []pendingLifecycleBalanceExpectation{
		{ref: "@source#default", available: "70", onHold: "0", version: 9},
		{ref: "@target#default", available: "50", onHold: "0", version: 4},
	})

	// A successful SetNX is the command-visible state after the expiring Go lock
	// has disappeared. Both later commands reacquire it, but neither may cross the
	// durable PENDING -> APPROVED engine guard a second time.
	transitioned, err = uc.CommitTransactionV2(ctx, transitionInput)
	requirePendingLifecycleLockConflict(t, err)
	require.Nil(t, transitioned)
	require.Equal(t, 2, lockAcquisitions)

	transitioned, err = uc.CancelTransactionV2(ctx, transitionInput)
	requirePendingLifecycleLockConflict(t, err)
	require.Nil(t, transitioned)
	require.Equal(t, 3, lockAcquisitions)
	require.Len(t, executor.executions, 4)
	require.Len(t, finalizer.envelopes, 2)
	require.Equal(t, command.ExecutionGuard{TransactionID: transitionInput.TransactionID, ExpectedToken: constant.PENDING, NextToken: constant.APPROVED}, executor.executions[2].Guards[0])
	require.Equal(t, command.ExecutionGuard{TransactionID: transitionInput.TransactionID, ExpectedToken: constant.PENDING, NextToken: constant.CANCELED}, executor.executions[3].Guards[0])
	require.Equal(t, constant.APPROVED, client.HGet(ctx, keys.Guards, pending.ID).Val())
	require.Equal(t, int64(2), client.HLen(ctx, keys.Recovery).Val())
	require.Equal(t, int64(2), client.HLen(ctx, keys.Receipts).Val())
	for _, rejected := range executor.executions[2:] {
		require.False(t, client.HExists(ctx, keys.Recovery, pending.ID+":"+rejected.Execution.ExecutionID.String()).Val())
		require.False(t, client.HExists(ctx, keys.Receipts, rejected.Execution.ExecutionID.String()).Val())
	}
	winningPayload, err := command.DecodeTransactionCompletionPlan([]byte(finalizer.envelopes[1].Payload))
	require.NoError(t, err)
	winningRecord, err := command.BuildTransactionWriteSet(*winningPayload, finalizer.envelopes[1].Result)
	require.NoError(t, err)
	assertPendingLifecycleRecovery(t, ctx, client, keys, winningExecution, winningRecord.Transaction)
	assertPendingLifecycleBalances(t, ctx, client, keys, []pendingLifecycleBalanceExpectation{
		{ref: "@source#default", available: "70", onHold: "0", version: 9},
		{ref: "@target#default", available: "50", onHold: "0", version: 4},
	})
}

func requirePendingLifecycleLockConflict(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var conflict pkg.EntityConflictError
	require.True(t, errors.As(err, &conflict))
	require.Equal(t, constant.ErrPendingTransactionLocked.Error(), conflict.Code)
}

type pendingLifecycleBalanceExpectation struct {
	ref       string
	available string
	onHold    string
	version   int64
}

func pendingLifecyclePostingTypes(execution command.EngineExecution) []core.PostingType {
	types := make([]core.PostingType, 0, len(execution.Execution.Transactions[0].Postings))
	for _, posting := range execution.Execution.Transactions[0].Postings {
		types = append(types, posting.Type)
	}
	return types
}

func assertPendingLifecycleProjection(t *testing.T, envelope *command.TransactionCompletionRecord, transaction *postgresTransaction.Transaction) {
	t.Helper()
	payload, err := command.DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	require.NoError(t, err)
	projected, err := command.BuildOperationRecordsFromMovements(*payload, envelope.Result)
	require.NoError(t, err)
	requireJSONEqual(t, transaction.Operations, projected)
}

func assertPendingLifecycleRecovery(t *testing.T, ctx context.Context, client *redis.Client, keys resolvedExecutionKeys, execution command.EngineExecution, transaction *postgresTransaction.Transaction) {
	t.Helper()
	recoveryRaw, err := client.HGet(ctx, keys.Recovery, transaction.ID+":"+execution.Execution.ExecutionID.String()).Bytes()
	require.NoError(t, err)
	recovery, err := command.DecodeTransactionCompletionRecord(recoveryRaw)
	require.NoError(t, err)
	require.Equal(t, execution.Execution.ExecutionID, recovery.ExecutionID)
	require.Equal(t, execution.IntentFingerprint, recovery.IntentFingerprint)
	assertPendingLifecycleProjection(t, recovery, transaction)
}

func assertPendingLifecycleBalances(t *testing.T, ctx context.Context, client *redis.Client, keys resolvedExecutionKeys, expectations []pendingLifecycleBalanceExpectation) {
	t.Helper()
	for _, expected := range expectations {
		raw, err := client.Get(ctx, keys.Balances[expected.ref].Balance).Bytes()
		require.NoError(t, err)
		balance, err := balancecache.Decode(raw)
		require.NoError(t, err)
		require.Equal(t, expected.available, balance.Available.String())
		require.Equal(t, expected.onHold, balance.OnHold.String())
		require.Equal(t, expected.version, balance.Version)
	}
}

var (
	_ command.TransactionReader              = (*pendingLifecycleReader)(nil)
	_ command.BalanceEngine                  = (*pendingLifecycleAdapter)(nil)
	_ command.BalanceEngineGuardBootstrapper = (*pendingLifecycleAdapter)(nil)
	_ command.BalanceEngine                  = (*racingPendingLifecycleAdapter)(nil)
	_ command.BalanceEngineGuardBootstrapper = (*racingPendingLifecycleAdapter)(nil)
	_ command.AppliedTransactionCompleter    = (*pendingLifecycleFinalizer)(nil)
	_ command.AppliedTransactionCompleter    = (*pendingRaceFinalizer)(nil)
	_ command.AppliedTransactionCompleter    = (*pendingLockExpiryFinalizer)(nil)
	_ command.TracerReserver                 = (*pendingLifecycleTracer)(nil)
)
