// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// revertIndexedOriginReader serves the origin the way the engine-aware query use case
// does: the lifecycle lookup answers the indexed execution's view, and the primary
// with-operations read answers every persisted row.
type revertIndexedOriginReader struct {
	*revertReader

	indexed     *transaction.Transaction
	executionID uuid.UUID
	pending     bool

	primary       *transaction.Transaction
	primaryErr    error
	primaryReads  int
	primaryRouted []bool

	routes map[string]*mmodel.OperationRoute
}

func (r *revertIndexedOriginReader) ResolveTransactionProjection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*transaction.Transaction, uuid.UUID, bool, error) {
	return cloneRevertOrigin(r.indexed), r.executionID, r.pending, nil
}

func (r *revertIndexedOriginReader) GetTransactionWithOperationsByID(ctx context.Context, _, _, _ uuid.UUID) (*transaction.Transaction, error) {
	r.primaryReads++
	r.primaryRouted = append(r.primaryRouted, readrouting.IsPrimaryRead(ctx))

	return cloneRevertOrigin(r.primary), r.primaryErr
}

func (r *revertIndexedOriginReader) GetOperationRouteByID(_ context.Context, _ uuid.UUID, routeID uuid.UUID) (*mmodel.OperationRoute, error) {
	return r.routes[routeID.String()], nil
}

func cloneRevertOrigin(tran *transaction.Transaction) *transaction.Transaction {
	if tran == nil {
		return nil
	}

	cloned := *tran
	cloned.Operations = append([]*operation.Operation(nil), tran.Operations...)

	return &cloned
}

// committedHold holds the rows a hold of 500 from @payer to @payee writes once it is
// committed on a ledger that validates routes: the hold splits the source into a DEBIT
// and an ON_HOLD credit, and the commit releases the hold and credits the destination.
type committedHold struct {
	holdOnHold, holdDebit, commitOnHold, commitCredit *operation.Operation
}

func newCommittedHold(amount decimal.Decimal) committedHold {
	row := func(operationType, direction, alias string) *operation.Operation {
		value := amount

		return &operation.Operation{
			ID: uuid.NewString(), Type: operationType, Direction: direction, AccountAlias: alias,
			BalanceKey: constant.DefaultBalanceKey, AssetCode: "BRL", Amount: operation.Amount{Value: &value},
		}
	}

	return committedHold{
		holdOnHold:   row(constant.ONHOLD, constant.DirectionCredit, "@payer"),
		holdDebit:    row(constant.DEBIT, constant.DirectionDebit, "@payer"),
		commitOnHold: row(constant.ONHOLD, constant.DirectionDebit, "@payer"),
		commitCredit: row(constant.CREDIT, constant.DirectionCredit, "@payee"),
	}
}

func (h committedHold) commitRows() []*operation.Operation {
	return []*operation.Operation{h.commitOnHold, h.commitCredit}
}

func (h committedHold) allRows() []*operation.Operation {
	return []*operation.Operation{h.holdOnHold, h.holdDebit, h.commitOnHold, h.commitCredit}
}

func approvedOrigin(in RevertTransactionInput, amount decimal.Decimal, rows []*operation.Operation) *transaction.Transaction {
	return &transaction.Transaction{
		ID: in.TransactionID.String(), OrganizationID: in.OrganizationID.String(), LedgerID: in.LedgerID.String(),
		AssetCode: "BRL", Amount: &amount, Status: transaction.Status{Code: constant.APPROVED},
		Operations: rows,
	}
}

func reversalLegs(entries []mtransaction.FromTo) map[string]string {
	legs := make(map[string]string, len(entries))
	for _, entry := range entries {
		legs[entry.AccountAlias] = entry.Amount.Value.String()
	}

	return legs
}

func TestPrepareRevertTransaction_DurableCommittedHoldReversesBothPhases(t *testing.T) {
	amount := decimal.NewFromInt(500)
	hold := newCommittedHold(amount)
	in := revertInput()

	reader := &revertIndexedOriginReader{
		revertReader: &revertReader{},
		indexed:      approvedOrigin(in, amount, hold.commitRows()),
		executionID:  uuid.New(),
		primary:      approvedOrigin(in, amount, hold.allRows()),
	}
	uc := &UseCase{TransactionReader: reader}

	reversal, _, err := uc.prepareRevertTransaction(context.Background(), trace.SpanFromContext(context.Background()), in)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"@payee": "500"}, reversalLegs(reversal.Send.Source.From))
	assert.Equal(t, map[string]string{"@payer": "500"}, reversalLegs(reversal.Send.Distribute.To),
		"the hold's DEBIT is the destination of the reversal; the commit's execution alone does not carry it")
	assert.Equal(t, []bool{true}, reader.primaryRouted, "the hold's rows are read from the primary, never a lagging replica")

	_, err = mtransaction.ValidateSendSourceAndDistribute(context.Background(), reversal, constant.CREATED)
	require.NoError(t, err, "the reversal must balance")
}

func TestPrepareRevertTransaction_DurableCommitWithoutRouteValidationKeepsItsLegs(t *testing.T) {
	amount := decimal.NewFromInt(500)
	hold := newCommittedHold(amount)
	commitDebit := &operation.Operation{
		ID: uuid.NewString(), Type: constant.DEBIT, Direction: constant.DirectionDebit, AccountAlias: "@payer",
		BalanceKey: constant.DefaultBalanceKey, AssetCode: "BRL", Amount: operation.Amount{Value: &amount},
	}
	holdOnHold := &operation.Operation{
		ID: uuid.NewString(), Type: constant.ONHOLD, Direction: constant.DirectionDebit, AccountAlias: "@payer",
		BalanceKey: constant.DefaultBalanceKey, AssetCode: "BRL", Amount: operation.Amount{Value: &amount},
	}
	in := revertInput()

	reader := &revertIndexedOriginReader{
		revertReader: &revertReader{},
		indexed:      approvedOrigin(in, amount, []*operation.Operation{commitDebit, hold.commitCredit}),
		executionID:  uuid.New(),
		primary:      approvedOrigin(in, amount, []*operation.Operation{holdOnHold, commitDebit, hold.commitCredit}),
	}
	uc := &UseCase{TransactionReader: reader}

	reversal, _, err := uc.prepareRevertTransaction(context.Background(), trace.SpanFromContext(context.Background()), in)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"@payee": "500"}, reversalLegs(reversal.Send.Source.From))
	assert.Equal(t, map[string]string{"@payer": "500"}, reversalLegs(reversal.Send.Distribute.To),
		"a row read from both sources is one leg, not two")
	assert.Len(t, reversal.Send.Distribute.To, 1)
}

func TestPrepareRevertTransaction_PersistedOriginNeedsNoExtraRead(t *testing.T) {
	amount := decimal.NewFromInt(500)
	hold := newCommittedHold(amount)
	in := revertInput()
	persisted := approvedOrigin(in, amount, hold.allRows())

	reader := &revertIndexedOriginReader{revertReader: &revertReader{}, indexed: persisted, primary: persisted}
	uc := &UseCase{TransactionReader: reader}

	reversal, _, err := uc.prepareRevertTransaction(context.Background(), trace.SpanFromContext(context.Background()), in)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"@payer": "500"}, reversalLegs(reversal.Send.Distribute.To))
	assert.Zero(t, reader.primaryReads, "an origin already served by the primary carries every row")
}

func TestPrepareRevertTransaction_DurableCommittedHoldChecksTheHoldRoutes(t *testing.T) {
	amount := decimal.NewFromInt(500)
	hold := newCommittedHold(amount)
	sourceOnly, bidirectional := uuid.NewString(), uuid.NewString()
	hold.holdDebit.RouteID = &sourceOnly
	hold.commitCredit.RouteID = &bidirectional
	in := revertInput()

	reader := &revertIndexedOriginReader{
		revertReader: &revertReader{},
		indexed:      approvedOrigin(in, amount, hold.commitRows()),
		executionID:  uuid.New(),
		primary:      approvedOrigin(in, amount, hold.allRows()),
		routes: map[string]*mmodel.OperationRoute{
			sourceOnly:    {OperationType: "source"},
			bidirectional: {OperationType: "bidirectional"},
		},
	}
	uc := &UseCase{TransactionReader: reader}

	_, _, err := uc.prepareRevertTransaction(context.Background(), trace.SpanFromContext(context.Background()), in)

	var business pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &business)
	assert.Equal(t, constant.ErrRouteNotBidirectional.Error(), business.Code)
}

func TestPrepareRevertTransaction_DurableOriginPrimaryReadFailurePropagates(t *testing.T) {
	amount := decimal.NewFromInt(500)
	hold := newCommittedHold(amount)
	in := revertInput()
	primaryDown := errors.New("primary unavailable")

	reader := &revertIndexedOriginReader{
		revertReader: &revertReader{},
		indexed:      approvedOrigin(in, amount, hold.commitRows()),
		executionID:  uuid.New(),
		primaryErr:   primaryDown,
	}
	uc := &UseCase{TransactionReader: reader}

	reversal, _, err := uc.prepareRevertTransaction(context.Background(), trace.SpanFromContext(context.Background()), in)

	require.ErrorIs(t, err, primaryDown)
	assert.True(t, reversal.IsEmpty(), "no reversal is built from a partial set")
}

const revertEvidenceTenant = "tenant-revert-origin"

// holdEvidence builds the engine evidence of the hold execution of origin: the source
// DEBIT of amount the hold wrote on @source, re-scoped from the recovery contract
// fixture so the lookup view is composed by the production evidence path.
func holdEvidence(t *testing.T, in RevertTransactionInput) TransactionWriteBehindEnvelope {
	t.Helper()

	plan, result := recoveryContractFixture(t)
	plan.TenantID = revertEvidenceTenant
	plan.OrganizationID, plan.LedgerID, plan.TransactionID = in.OrganizationID, in.LedgerID, in.TransactionID
	plan.ExecutionID = uuid.New()
	plan.TransactionStatus, plan.Action = constant.PENDING, constant.ActionHold

	for index := range plan.OperationSpecs {
		plan.OperationSpecs[index].TransactionID = in.TransactionID
		plan.OperationSpecs[index].Balance.OrganizationID = in.OrganizationID.String()
		plan.OperationSpecs[index].Balance.LedgerID = in.LedgerID.String()
	}

	for index := range result.Movements {
		result.Movements[index].TransactionID = in.TransactionID
	}

	var err error
	plan.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(plan))
	require.NoError(t, err)

	return TransactionWriteBehindEnvelope{
		FormatVersion: TransactionWriteBehindFormatVersion, ApplicationState: TransactionApplicationConfirmed,
		ReplayState: TransactionReplayReconstructible, DurabilityState: TransactionDurabilityComplete,
		Record: recoveryContractEnvelope(t, plan, result), Dependencies: []TransactionEvidenceReference{},
	}
}

// commitEvidence builds the evidence of the commit execution that follows hold, naming
// it as predecessor when named is true.
func commitEvidence(t *testing.T, hold TransactionWriteBehindEnvelope, named bool) TransactionWriteBehindEnvelope {
	t.Helper()

	commit := transactionWriteBehindExecutionFixture(t, hold, uuid.New())
	commit.DurabilityState = TransactionDurabilityPending

	if named {
		commit.Dependencies = []TransactionEvidenceReference{transactionEvidenceReference(TransactionDependencyPredecessor, hold)}
	}

	return commit
}

func holdLookupOperations(t *testing.T, hold TransactionWriteBehindEnvelope) []*operation.Operation {
	t.Helper()

	views, err := BuildTransactionEvidenceViews(hold.Record)
	require.NoError(t, err)

	return views.Lookup.Operations
}

// inFlightCommittedHold is the origin as the engine index serves it while the commit is
// still pending persistence: the commit released the hold on @source and credited
// @payee, and the source DEBIT lives only in the hold's execution.
func inFlightCommittedHold(in RevertTransactionInput) *transaction.Transaction {
	amount := decimal.NewFromInt(30)
	row := func(operationType, direction, alias string) *operation.Operation {
		value := amount

		return &operation.Operation{
			ID: uuid.NewString(), Type: operationType, Direction: direction, AccountAlias: alias,
			BalanceKey: constant.DefaultBalanceKey, AssetCode: "USD", Amount: operation.Amount{Value: &value},
		}
	}

	origin := approvedOrigin(in, amount, []*operation.Operation{
		row(constant.ONHOLD, constant.DirectionDebit, "@source"),
		row(constant.CREDIT, constant.DirectionCredit, "@payee"),
	})
	origin.AssetCode = "USD"

	return origin
}

func evidenceIdentity(envelope TransactionWriteBehindEnvelope) string {
	return transactionCompletionEvidenceIdentity(envelope.Record.TransactionID, envelope.Record.ExecutionID)
}

func TestPrepareRevertTransaction_InFlightCommittedHoldReversesFromItsPredecessor(t *testing.T) {
	in := revertInput()
	hold := holdEvidence(t, in)
	commit := commitEvidence(t, hold, true)

	reader := &revertIndexedOriginReader{
		revertReader: &revertReader{},
		indexed:      inFlightCommittedHold(in),
		executionID:  commit.Record.ExecutionID,
		pending:      true,
	}
	resolver := &transactionEvidenceResolverStub{records: map[string]*TransactionWriteBehindEnvelope{
		evidenceIdentity(hold): &hold, evidenceIdentity(commit): &commit,
	}}
	uc := &UseCase{TransactionReader: reader, TransactionEvidenceResolver: resolver}
	ctx := tmcore.ContextWithTenantID(context.Background(), revertEvidenceTenant)

	reversal, _, err := uc.prepareRevertTransaction(ctx, trace.SpanFromContext(ctx), in)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"@payee": "30"}, reversalLegs(reversal.Send.Source.From))
	assert.Equal(t, map[string]string{"@source": "30"}, reversalLegs(reversal.Send.Distribute.To),
		"the hold's DEBIT, read from the predecessor evidence, is the destination of the reversal")
	assert.Zero(t, reader.primaryReads, "a named predecessor answers without the primary")

	_, err = mtransaction.ValidateSendSourceAndDistribute(ctx, reversal, constant.CREATED)
	require.NoError(t, err, "the reversal must balance")
}

func TestPrepareRevertTransaction_InFlightCommitWithoutPredecessorReadsTheHoldFromThePrimary(t *testing.T) {
	in := revertInput()
	hold := holdEvidence(t, in)
	commit := commitEvidence(t, hold, false)
	indexed := inFlightCommittedHold(in)

	reader := &revertIndexedOriginReader{
		revertReader: &revertReader{},
		indexed:      indexed,
		executionID:  commit.Record.ExecutionID,
		pending:      true,
		primary:      approvedOrigin(in, *indexed.Amount, holdLookupOperations(t, hold)),
	}
	resolver := &transactionEvidenceResolverStub{records: map[string]*TransactionWriteBehindEnvelope{
		evidenceIdentity(commit): &commit,
	}}
	uc := &UseCase{TransactionReader: reader, TransactionEvidenceResolver: resolver}
	ctx := tmcore.ContextWithTenantID(context.Background(), revertEvidenceTenant)

	reversal, _, err := uc.prepareRevertTransaction(ctx, trace.SpanFromContext(ctx), in)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"@source": "30"}, reversalLegs(reversal.Send.Distribute.To))
	assert.Equal(t, 1, reader.primaryReads)
}

func TestPrepareRevertTransaction_InFlightCommitMergesRowsAlreadyPersisted(t *testing.T) {
	in := revertInput()
	hold := holdEvidence(t, in)
	commit := commitEvidence(t, hold, false)
	indexed := inFlightCommittedHold(in)
	// The completer can have written some commit rows before its acknowledgement.
	persisted := append(holdLookupOperations(t, hold), indexed.Operations[1])

	reader := &revertIndexedOriginReader{
		revertReader: &revertReader{},
		indexed:      indexed,
		executionID:  commit.Record.ExecutionID,
		pending:      true,
		primary:      approvedOrigin(in, *indexed.Amount, persisted),
	}
	resolver := &transactionEvidenceResolverStub{records: map[string]*TransactionWriteBehindEnvelope{
		evidenceIdentity(commit): &commit,
	}}
	uc := &UseCase{TransactionReader: reader, TransactionEvidenceResolver: resolver}
	ctx := tmcore.ContextWithTenantID(context.Background(), revertEvidenceTenant)

	reversal, _, err := uc.prepareRevertTransaction(ctx, trace.SpanFromContext(ctx), in)
	require.NoError(t, err)

	assert.Len(t, reversal.Send.Source.From, 1, "a commit row read from both sources is one leg")
	_, err = mtransaction.ValidateSendSourceAndDistribute(ctx, reversal, constant.CREATED)
	require.NoError(t, err)
}

func TestPrepareRevertTransaction_InFlightCommitFailsClosedOnUnreadablePredecessor(t *testing.T) {
	in := revertInput()
	hold := holdEvidence(t, in)
	commit := commitEvidence(t, hold, true)

	// A well-formed envelope of another execution of the same transaction, stored where
	// the predecessor's should be: only the revert's own scope check can refuse it.
	foreign := transactionWriteBehindExecutionFixture(t, hold, uuid.New())

	for _, tc := range []struct {
		name    string
		tenant  string
		records map[string]*TransactionWriteBehindEnvelope
	}{
		{name: "predecessor evidence is missing", records: map[string]*TransactionWriteBehindEnvelope{
			evidenceIdentity(commit): &commit,
		}},
		{name: "predecessor evidence names another execution", records: map[string]*TransactionWriteBehindEnvelope{
			evidenceIdentity(commit): &commit, evidenceIdentity(hold): &foreign,
		}},
		{name: "evidence belongs to another tenant", tenant: "another-tenant", records: map[string]*TransactionWriteBehindEnvelope{
			evidenceIdentity(commit): &commit, evidenceIdentity(hold): &hold,
		}},
		{name: "commit evidence is missing", records: map[string]*TransactionWriteBehindEnvelope{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tenant := revertEvidenceTenant
			if tc.tenant != "" {
				tenant = tc.tenant
			}

			reader := &revertIndexedOriginReader{
				revertReader: &revertReader{},
				indexed:      inFlightCommittedHold(in),
				executionID:  commit.Record.ExecutionID,
				pending:      true,
				primary:      approvedOrigin(in, decimal.NewFromInt(30), holdLookupOperations(t, hold)),
			}
			uc := &UseCase{TransactionReader: reader, TransactionEvidenceResolver: &transactionEvidenceResolverStub{records: tc.records}}
			ctx := tmcore.ContextWithTenantID(context.Background(), tenant)

			reversal, _, err := uc.prepareRevertTransaction(ctx, trace.SpanFromContext(ctx), in)

			require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
			assert.True(t, reversal.IsEmpty(), "no reversal is built from a partial set")
			assert.Zero(t, reader.primaryReads, "an unreadable named predecessor must not fall back to the primary")
		})
	}
}

func TestPrepareRevertTransaction_InFlightCommitWithoutEvidenceResolverReadsThePrimary(t *testing.T) {
	in := revertInput()
	hold := holdEvidence(t, in)
	indexed := inFlightCommittedHold(in)

	reader := &revertIndexedOriginReader{
		revertReader: &revertReader{},
		indexed:      indexed,
		executionID:  uuid.New(),
		pending:      true,
		primary:      approvedOrigin(in, *indexed.Amount, holdLookupOperations(t, hold)),
	}
	uc := &UseCase{TransactionReader: reader}

	reversal, _, err := uc.prepareRevertTransaction(context.Background(), trace.SpanFromContext(context.Background()), in)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"@source": "30"}, reversalLegs(reversal.Send.Distribute.To))
	assert.Equal(t, 1, reader.primaryReads)
}

// failingEvidenceResolver answers from records and fails the reads named in errs, the way
// the Redis resolver fails on a missing or unreachable evidence hash.
type failingEvidenceResolver struct {
	records map[string]*TransactionWriteBehindEnvelope
	errs    map[string]error
}

func (resolver *failingEvidenceResolver) ResolveTransactionEvidence(_ context.Context, reference TransactionEvidenceReference) (*TransactionWriteBehindEnvelope, error) {
	identity := transactionCompletionEvidenceIdentity(reference.TransactionID, reference.ExecutionID)
	if err := resolver.errs[identity]; err != nil {
		return nil, err
	}

	return resolver.records[identity], nil
}

func TestPrepareRevertTransaction_InFlightCommitPropagatesAPredecessorReadFailure(t *testing.T) {
	in := revertInput()
	hold := holdEvidence(t, in)
	commit := commitEvidence(t, hold, true)
	unreachable := errors.New("evidence store unreachable")

	reader := &revertIndexedOriginReader{
		revertReader: &revertReader{},
		indexed:      inFlightCommittedHold(in),
		executionID:  commit.Record.ExecutionID,
		pending:      true,
		primary:      approvedOrigin(in, decimal.NewFromInt(30), holdLookupOperations(t, hold)),
	}
	uc := &UseCase{TransactionReader: reader, TransactionEvidenceResolver: &failingEvidenceResolver{
		records: map[string]*TransactionWriteBehindEnvelope{evidenceIdentity(commit): &commit},
		errs:    map[string]error{evidenceIdentity(hold): unreachable},
	}}
	ctx := tmcore.ContextWithTenantID(context.Background(), revertEvidenceTenant)

	reversal, _, err := uc.prepareRevertTransaction(ctx, trace.SpanFromContext(ctx), in)

	require.ErrorIs(t, err, unreachable)
	assert.True(t, reversal.IsEmpty(), "no reversal is built from a partial set")
	assert.Zero(t, reader.primaryReads, "a named predecessor that cannot be read must not fall back to the primary")
}

// twoSourceHoldEvidence is holdEvidence with the hold split over @source and @source-b,
// 15 each, every leg carrying its own metadata.
func twoSourceHoldEvidence(t *testing.T, in RevertTransactionInput) TransactionWriteBehindEnvelope {
	t.Helper()

	envelope := holdEvidence(t, in)
	plan, err := DecodeTransactionCompletionPlan([]byte(envelope.Record.Payload))
	require.NoError(t, err)

	result := envelope.Record.Result
	half := decimal.NewFromInt(15)

	second := plan.OperationSpecs[0]
	second.PostingRef, second.BalanceRef = "source:1", "@source-b#default"
	second.Balance.ID, second.Balance.AccountID, second.Balance.Alias = uuid.NewString(), uuid.NewString(), "@source-b"
	second.Metadata = map[string]any{"purpose": "second leg"}
	plan.OperationSpecs[0].RequestedAmount, second.RequestedAmount = half, half
	plan.OperationSpecs = append(plan.OperationSpecs, second)

	secondMovement := result.Movements[0]
	secondMovement.Ref, secondMovement.PostingRef, secondMovement.BalanceRef = "movement:1", "source:1", "@source-b#default"
	result.Movements[0].Amount, secondMovement.Amount = half, half
	result.Movements[0].After.Available, secondMovement.After.Available = decimal.NewFromInt(85), decimal.NewFromInt(85)
	result.Movements = append(result.Movements, secondMovement)
	result.Final = recoveryContractFinal(*plan, result.Movements)

	plan.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(*plan))
	require.NoError(t, err)
	envelope.Record = recoveryContractEnvelope(t, *plan, result)

	return envelope
}

// asPersisted returns rows the way the primary answers them: FindWithOperations loads no
// operation metadata and imposes no order.
func asPersisted(rows []*operation.Operation) []*operation.Operation {
	persisted := make([]*operation.Operation, 0, len(rows))
	for index := len(rows) - 1; index >= 0; index-- {
		row := *rows[index]
		row.Metadata = nil
		persisted = append(persisted, &row)
	}

	return persisted
}

// TestPrepareRevertTransaction_CommittedHoldReversalIsTheSameFromEverySource locks the
// revert idempotency key of a committed hold: it is a hash of the reversal payload, and a
// retry must find the slot the first attempt claimed whether the hold's rows came from its
// evidence or from the primary.
func TestPrepareRevertTransaction_CommittedHoldReversalIsTheSameFromEverySource(t *testing.T) {
	in := revertInput()
	hold := twoSourceHoldEvidence(t, in)
	commitNamed, commitUnnamed := commitEvidence(t, hold, true), commitEvidence(t, hold, false)

	indexed := inFlightCommittedHold(in)
	indexed.Operations[1].Metadata = map[string]any{"purpose": "settlement"}
	holdRows := holdLookupOperations(t, hold)

	reversalFrom := func(t *testing.T, reader *revertIndexedOriginReader, records map[string]*TransactionWriteBehindEnvelope) mtransaction.Transaction {
		t.Helper()

		reader.revertReader, reader.indexed = &revertReader{}, indexed
		uc := &UseCase{TransactionReader: reader, TransactionEvidenceResolver: &transactionEvidenceResolverStub{records: records}}
		ctx := tmcore.ContextWithTenantID(context.Background(), revertEvidenceTenant)

		reversal, _, err := uc.prepareRevertTransaction(ctx, trace.SpanFromContext(ctx), in)
		require.NoError(t, err)
		require.Len(t, reversal.Send.Distribute.To, 2, "each hold source is a destination of the reversal")

		return reversal
	}

	fromEvidence := reversalFrom(t, &revertIndexedOriginReader{executionID: commitNamed.Record.ExecutionID, pending: true},
		map[string]*TransactionWriteBehindEnvelope{evidenceIdentity(commitNamed): &commitNamed, evidenceIdentity(hold): &hold})
	fromPrimaryInFlight := reversalFrom(t, &revertIndexedOriginReader{
		executionID: commitUnnamed.Record.ExecutionID, pending: true,
		primary: approvedOrigin(in, *indexed.Amount, asPersisted(holdRows)),
	}, map[string]*TransactionWriteBehindEnvelope{evidenceIdentity(commitUnnamed): &commitUnnamed})
	fromPrimaryDurable := reversalFrom(t, &revertIndexedOriginReader{
		executionID: commitNamed.Record.ExecutionID,
		primary:     approvedOrigin(in, *indexed.Amount, asPersisted(append(append([]*operation.Operation{}, indexed.Operations...), holdRows...))),
	}, nil)

	key := func(reversal mtransaction.Transaction) string {
		source, err := resolveIdempotencyHashSource(reversal)
		require.NoError(t, err)

		return source
	}

	assert.Equal(t, key(fromEvidence), key(fromPrimaryInFlight), "in flight, predecessor evidence and primary must build the same reversal")
	assert.Equal(t, key(fromEvidence), key(fromPrimaryDurable), "in flight and durable must build the same reversal")
}
