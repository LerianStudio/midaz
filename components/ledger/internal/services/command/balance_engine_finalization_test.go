// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	postgresTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type finalizationStoreStub struct {
	err     error
	records []BalanceEnginePersistenceRecord
	calls   *[]string
}

type finalizationOutcomeStoreStub struct {
	outcome   BalanceEngineRecoveryOutcome
	err       error
	legacyErr error
	records   []BalanceEnginePersistenceRecord
	calls     *[]string
}

func (store *finalizationOutcomeStoreStub) Persist(context.Context, BalanceEnginePersistenceRecord) error {
	*store.calls = append(*store.calls, "legacy-sql")

	return store.legacyErr
}

func (store *finalizationOutcomeStoreStub) PersistWithOutcome(ctx context.Context, record BalanceEnginePersistenceRecord) (BalanceEngineRecoveryOutcome, error) {
	*store.calls = append(*store.calls, "sql-with-outcome")
	store.records = append(store.records, record)
	if err := ctx.Err(); err != nil {
		return BalanceEngineRecoveryOutcome{}, err
	}

	return store.outcome, store.err
}

func (store *finalizationStoreStub) Persist(ctx context.Context, record BalanceEnginePersistenceRecord) error {
	*store.calls = append(*store.calls, "sql")
	store.records = append(store.records, record)
	if err := ctx.Err(); err != nil {
		return err
	}

	return store.err
}

type finalizationMetadataStub struct {
	calls     *[]string
	data      map[string]*mongodb.Metadata
	createErr error
	create    func(string) error
	findErr   error
	find      func(*mongodb.Metadata) *mongodb.Metadata
}

type finalizationEventPublisherStub struct {
	calls        *[]string
	transactions []*postgresTransaction.Transaction
	phases       []string
}

func (publisher *finalizationEventPublisherStub) PublishBalanceEngineEvents(_ context.Context, tran *postgresTransaction.Transaction, phase string) {
	*publisher.calls = append(*publisher.calls, "publish")
	publisher.transactions = append(publisher.transactions, tran)
	publisher.phases = append(publisher.phases, phase)
}

func (repo *finalizationMetadataStub) Create(_ context.Context, collection string, metadata *mongodb.Metadata) error {
	*repo.calls = append(*repo.calls, "create:"+collection)
	if repo.createErr != nil {
		return repo.createErr
	}

	if repo.create != nil {
		if err := repo.create(collection); err != nil {
			return err
		}
	}

	key := collection + ":" + metadata.EntityID
	if repo.data[key] != nil {
		return nil
	}

	model := &mongodb.MetadataMongoDBModel{}
	if err := model.FromEntity(metadata); err != nil {
		return err
	}

	encoded, err := bson.Marshal(model)
	if err != nil {
		return err
	}

	var decoded mongodb.MetadataMongoDBModel
	if err := bson.Unmarshal(encoded, &decoded); err != nil {
		return err
	}

	repo.data[key] = decoded.ToEntity()

	return nil
}

func (repo *finalizationMetadataStub) FindByEntity(_ context.Context, collection, id string) (*mongodb.Metadata, error) {
	*repo.calls = append(*repo.calls, "find:"+collection)
	if repo.findErr != nil {
		return nil, repo.findErr
	}

	actual := repo.data[collection+":"+id]
	if repo.find != nil {
		actual = repo.find(actual)
	}

	return actual, nil
}

func finalizationFixture(t testing.TB) (context.Context, *BalanceEngineRecoveryEnvelope) {
	t.Helper()
	payload, result := recoveryContractFixture(t)
	parent := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	payload.ParentTransactionID, payload.FeesSkipped, payload.TracerSkipped = &parent, true, true
	payload.TransactionCreatedAt = payload.TransactionDate.Add(-48 * time.Hour)
	payload.TransactionUpdatedAt = payload.TransactionDate.Add(time.Millisecond)
	payload.OperationUpdatedAt = payload.TransactionDate.Add(2 * time.Millisecond)
	payload.TransactionInput.Metadata = map[string]any{"sequence": json.Number("9007199254740993"), "fraction": json.Number("0.1"), "purpose": "frozen"}
	payload.TransactionInput.RouteID = payload.Projection[0].RouteID
	payload.TransactionInput.ChartOfAccountsGroupName = "frozen chart"
	payload.Validate.Sources, payload.Validate.Destinations = []string{"@source"}, []string{"@destination"}
	var err error
	payload.IntentFingerprint, err = ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	envelope := recoveryContractEnvelope(t, payload, result)

	return tmcore.ContextWithTenantID(context.Background(), payload.TenantID), &envelope
}

func finalizationDependencies() (*BalanceEngineFinalizer, *finalizationStoreStub, *finalizationMetadataStub, *[]string) {
	calls := []string{}
	store := &finalizationStoreStub{calls: &calls}
	metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}

	return NewBalanceEngineFinalizer(store, metadata), store, metadata, &calls
}

func TestBalanceEngineFinalizerConfirmsSQLAndMetadata(t *testing.T) {
	ctx, envelope := finalizationFixture(t)
	finalizer, store, metadata, calls := finalizationDependencies()
	before, err := json.Marshal(envelope)
	require.NoError(t, err)
	require.NoError(t, finalizer.Finalize(ctx, envelope))
	require.NoError(t, finalizer.Finalize(ctx, envelope))
	assert.Equal(t, []string{"sql", "create:" + constant.EntityTransaction, "find:" + constant.EntityTransaction, "create:" + constant.EntityOperation, "find:" + constant.EntityOperation, "sql", "create:" + constant.EntityTransaction, "find:" + constant.EntityTransaction, "create:" + constant.EntityOperation, "find:" + constant.EntityOperation}, *calls)
	require.Len(t, store.records, 2)
	first, replay := store.records[0].Transaction, store.records[1].Transaction
	assert.Equal(t, first.Operations[0].ID, replay.Operations[0].ID)
	assert.Equal(t, "99999999-9999-4999-8999-999999999999", *first.ParentTransactionID)
	assert.True(t, first.FeesSkipped)
	assert.True(t, first.TracerSkipped)
	assert.Equal(t, []string{"@source"}, first.Source)
	assert.Equal(t, []string{"@destination"}, first.Destination)
	assert.Equal(t, "frozen chart", first.ChartOfAccountsGroupName)
	assert.Equal(t, "55555555-5555-4555-8555-555555555555", *first.RouteID)
	frozen, err := DecodeBalanceEngineRecoveryPayload([]byte(envelope.Payload))
	require.NoError(t, err)
	assert.Equal(t, frozen.TransactionCreatedAt, first.CreatedAt)
	assert.Equal(t, frozen.TransactionUpdatedAt, first.UpdatedAt)
	assert.Equal(t, frozen.TransactionDate, first.Operations[0].CreatedAt)
	assert.Equal(t, frozen.OperationUpdatedAt, first.Operations[0].UpdatedAt)
	assert.True(t, first.CreatedAt.Before(first.Operations[0].CreatedAt))
	assert.True(t, first.UpdatedAt.After(first.Operations[0].CreatedAt))
	assert.Equal(t, "direct", store.records[0].Action)
	assert.Empty(t, store.records[0].ExpectedStatus)
	storedMetadata := metadata.data[constant.EntityTransaction+":"+first.ID]
	require.NotNil(t, storedMetadata)
	assert.Equal(t, int64(9007199254740993), storedMetadata.Data["sequence"])
	assert.Equal(t, 0.1, storedMetadata.Data["fraction"])
	after, err := json.Marshal(envelope)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestBalanceEngineFinalizerPreservesLegacyPublicAliasesFromNormalizedValidation(t *testing.T) {
	payload, result := recoveryContractFixture(t)
	payload.Validate.Sources = []string{
		"@source#default",
		"@source#default",
		"@source#overdraft",
		"@fees#settlement",
	}
	payload.Validate.Destinations = []string{
		"@destination#default",
		"@destination#reserve",
	}
	var err error
	payload.IntentFingerprint, err = ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	envelope := recoveryContractEnvelope(t, payload, result)
	ctx := tmcore.ContextWithTenantID(context.Background(), payload.TenantID)
	finalizer, store, _, _ := finalizationDependencies()

	require.NoError(t, finalizer.Finalize(ctx, &envelope))

	require.Len(t, store.records, 1)
	assert.Equal(t, []string{"@source", "@source", "@fees"}, store.records[0].Transaction.Source)
	assert.Equal(t, []string{"@destination", "@destination"}, store.records[0].Transaction.Destination)
	assert.NotContains(t, store.records[0].Transaction.Source, "@source#overdraft")
}

func TestBalanceEngineFinalizerReturnsDurableOutcomeAfterMetadataVerification(t *testing.T) {
	ctx, envelope := finalizationFixture(t)
	calls := []string{}
	store := &finalizationOutcomeStoreStub{
		outcome: BalanceEngineRecoveryOutcome{TransactionStatus: constant.APPROVED, LifecyclePhase: TransactionLifecyclePhaseCreated},
		calls:   &calls,
	}
	metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}
	finalizer := NewBalanceEngineFinalizer(store, metadata)

	result, err := finalizer.FinalizeWithOutcome(ctx, envelope)

	require.NoError(t, err)
	assert.Equal(t, constant.APPROVED, result.Outcome.TransactionStatus)
	assert.Equal(t, TransactionLifecyclePhaseCreated, result.Outcome.LifecyclePhase)
	assert.Equal(t, []string{
		"sql-with-outcome",
		"create:" + constant.EntityTransaction,
		"find:" + constant.EntityTransaction,
		"create:" + constant.EntityOperation,
		"find:" + constant.EntityOperation,
	}, calls)
	require.Len(t, store.records, 1)
}

func TestBalanceEngineFinalizerReturnsCallerOwnedProjectedRecord(t *testing.T) {
	ctx, envelope := finalizationFixture(t)
	calls := []string{}
	store := &finalizationOutcomeStoreStub{
		outcome: BalanceEngineRecoveryOutcome{TransactionStatus: constant.APPROVED, LifecyclePhase: TransactionLifecyclePhaseCreated},
		calls:   &calls,
	}
	metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}

	result, err := NewBalanceEngineFinalizer(store, metadata).FinalizeWithOutcome(ctx, envelope)

	require.NoError(t, err)
	require.NotNil(t, result.Record.Transaction)
	require.Len(t, result.Record.Transaction.Operations, 1)
	require.Len(t, store.records, 1)
	assert.Equal(t, result.Record.Transaction, store.records[0].Transaction)
	assert.NotSame(t, result.Record.Transaction, store.records[0].Transaction)
	assert.NotSame(t, result.Record.Transaction.Operations[0], store.records[0].Transaction.Operations[0])

	result.Record.Transaction.Status.Code = constant.CREATED
	result.Record.Transaction.Metadata["purpose"] = "caller"
	result.Record.Transaction.Operations[0].Metadata["purpose"] = "caller"

	assert.Equal(t, constant.APPROVED, store.records[0].Transaction.Status.Code)
	assert.Equal(t, "frozen", store.records[0].Transaction.Metadata["purpose"])
	assert.NotEqual(t, "caller", store.records[0].Transaction.Operations[0].Metadata["purpose"])
}

func TestBalanceEngineFinalizerWithEventsPublishesFrozenTransactionAfterDurability(t *testing.T) {
	ctx, envelope := finalizationFixture(t)
	calls := []string{}
	store := &finalizationOutcomeStoreStub{
		outcome: BalanceEngineRecoveryOutcome{TransactionStatus: constant.APPROVED, LifecyclePhase: TransactionLifecyclePhaseCreated},
		calls:   &calls,
	}
	metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}
	publisher := &finalizationEventPublisherStub{calls: &calls}
	finalizer, err := NewBalanceEngineFinalizerWithEvents(store, metadata, publisher)
	require.NoError(t, err)

	require.NoError(t, finalizer.Finalize(ctx, envelope))

	assert.Equal(t, []string{
		"sql-with-outcome",
		"create:" + constant.EntityTransaction,
		"find:" + constant.EntityTransaction,
		"create:" + constant.EntityOperation,
		"find:" + constant.EntityOperation,
		"publish",
	}, calls)
	require.Len(t, publisher.transactions, 1)
	require.Len(t, publisher.phases, 1)
	require.Len(t, store.records, 1)
	assert.Same(t, store.records[0].Transaction, publisher.transactions[0], "publisher must receive the frozen persisted transaction and rows")
	assert.Equal(t, TransactionLifecyclePhaseCreated, publisher.phases[0])
	payload, err := DecodeBalanceEngineRecoveryPayload([]byte(envelope.Payload))
	require.NoError(t, err)
	assert.Equal(t, payload.TransactionID.String(), publisher.transactions[0].ID)
	assert.Equal(t, payload.TransactionCreatedAt, publisher.transactions[0].CreatedAt)
	assert.Equal(t, payload.TransactionUpdatedAt, publisher.transactions[0].UpdatedAt)
	require.Len(t, publisher.transactions[0].Operations, 1)
	assert.Equal(t, payload.OperationUpdatedAt, publisher.transactions[0].Operations[0].UpdatedAt)
}

func TestBalanceEngineFinalizerWithEventsRequiresPublisherAndOutcomeStore(t *testing.T) {
	metadata := &finalizationMetadataStub{data: make(map[string]*mongodb.Metadata)}
	outcomeStore := &finalizationOutcomeStoreStub{}

	finalizer, err := NewBalanceEngineFinalizerWithEvents(outcomeStore, metadata, nil)
	require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
	require.Nil(t, finalizer)
	var typedNilPublisher *finalizationEventPublisherStub
	finalizer, err = NewBalanceEngineFinalizerWithEvents(outcomeStore, metadata, typedNilPublisher)
	require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
	require.Nil(t, finalizer)

	calls := []string{}
	finalizer, err = NewBalanceEngineFinalizerWithEvents(
		&finalizationStoreStub{calls: &calls},
		metadata,
		&finalizationEventPublisherStub{calls: &calls},
	)
	require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
	require.Nil(t, finalizer)
	assert.Empty(t, calls, "missing outcome capability must fail before SQL")
}

func TestBalanceEngineFinalizerWithEventsPublishesOnlyAfterConfirmedCompletion(t *testing.T) {
	failure := errors.New("durability is not confirmed")
	tests := []struct {
		name       string
		outcome    BalanceEngineRecoveryOutcome
		storeErr   error
		metadataFn func(*finalizationMetadataStub)
		wantErr    error
	}{
		{
			name: "SQL failure",
			outcome: BalanceEngineRecoveryOutcome{
				TransactionStatus: constant.APPROVED,
				LifecyclePhase:    TransactionLifecyclePhaseCreated,
			},
			storeErr: failure,
			wantErr:  failure,
		},
		{
			name: "metadata failure",
			outcome: BalanceEngineRecoveryOutcome{
				TransactionStatus: constant.APPROVED,
				LifecyclePhase:    TransactionLifecyclePhaseCreated,
			},
			metadataFn: func(metadata *finalizationMetadataStub) { metadata.createErr = failure },
			wantErr:    failure,
		},
		{
			name:    "empty phase",
			outcome: BalanceEngineRecoveryOutcome{TransactionStatus: constant.APPROVED},
			wantErr: ErrBalanceEnginePersistenceConflict,
		},
		{
			name: "unknown phase",
			outcome: BalanceEngineRecoveryOutcome{
				TransactionStatus: constant.APPROVED,
				LifecyclePhase:    "future",
			},
			wantErr: ErrBalanceEnginePersistenceConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, envelope := finalizationFixture(t)
			calls := []string{}
			store := &finalizationOutcomeStoreStub{outcome: test.outcome, err: test.storeErr, calls: &calls}
			metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}
			if test.metadataFn != nil {
				test.metadataFn(metadata)
			}
			publisher := &finalizationEventPublisherStub{calls: &calls}
			finalizer, err := NewBalanceEngineFinalizerWithEvents(store, metadata, publisher)
			require.NoError(t, err)

			err = finalizer.Finalize(ctx, envelope)

			require.ErrorIs(t, err, test.wantErr)
			assert.Empty(t, publisher.transactions)
			assert.NotContains(t, calls, "publish")
		})
	}
}

func TestBalanceEngineFinalizerWithEventsUsesReportedLifecyclePhase(t *testing.T) {
	for _, phase := range []string{
		TransactionLifecyclePhaseCreated,
		TransactionLifecyclePhaseUpdated,
		TransactionLifecyclePhaseNoop,
	} {
		t.Run(phase, func(t *testing.T) {
			ctx, envelope := finalizationFixture(t)
			calls := []string{}
			store := &finalizationOutcomeStoreStub{
				outcome: BalanceEngineRecoveryOutcome{TransactionStatus: constant.APPROVED, LifecyclePhase: phase},
				calls:   &calls,
			}
			metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}
			publisher := &finalizationEventPublisherStub{calls: &calls}
			finalizer, err := NewBalanceEngineFinalizerWithEvents(store, metadata, publisher)
			require.NoError(t, err)

			result, err := finalizer.FinalizeWithOutcome(ctx, envelope)

			require.NoError(t, err)
			assert.Equal(t, phase, result.Outcome.LifecyclePhase)
			assert.Equal(t, []string{phase}, publisher.phases)
			require.Len(t, publisher.transactions, 1)
		})
	}
}

func TestBalanceEngineFinalizerSelectsTheRequestedPersistenceCapability(t *testing.T) {
	ctx, envelope := finalizationFixture(t)

	t.Run("legacy finalize uses Persist", func(t *testing.T) {
		calls := []string{}
		store := &finalizationOutcomeStoreStub{
			outcome: BalanceEngineRecoveryOutcome{TransactionStatus: constant.APPROVED},
			calls:   &calls,
		}
		metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}

		require.NoError(t, NewBalanceEngineFinalizer(store, metadata).Finalize(ctx, envelope))
		assert.Equal(t, "legacy-sql", calls[0])
		assert.NotContains(t, calls, "sql-with-outcome")
	})

	t.Run("outcome finalize uses PersistWithOutcome", func(t *testing.T) {
		calls := []string{}
		store := &finalizationOutcomeStoreStub{
			outcome:   BalanceEngineRecoveryOutcome{TransactionStatus: constant.APPROVED},
			legacyErr: errors.New("legacy persistence must not be used by outcome finalization"),
			calls:     &calls,
		}
		metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}

		result, err := NewBalanceEngineFinalizer(store, metadata).FinalizeWithOutcome(ctx, envelope)
		require.NoError(t, err)
		assert.Equal(t, constant.APPROVED, result.Outcome.TransactionStatus)
		assert.Equal(t, "sql-with-outcome", calls[0])
		assert.NotContains(t, calls, "legacy-sql")
	})
}

func TestBalanceEngineFinalizerReturnsZeroOutcomeWhenCompletionFails(t *testing.T) {
	failure := errors.New("durability is not confirmed")

	for _, scenario := range []struct {
		name       string
		storeErr   error
		metadataFn func(*finalizationMetadataStub)
	}{
		{name: "SQL", storeErr: failure},
		{name: "metadata create", metadataFn: func(metadata *finalizationMetadataStub) { metadata.createErr = failure }},
		{name: "metadata find", metadataFn: func(metadata *finalizationMetadataStub) { metadata.findErr = failure }},
		{name: "metadata compare", metadataFn: func(metadata *finalizationMetadataStub) {
			metadata.find = func(actual *mongodb.Metadata) *mongodb.Metadata {
				actual.Data["purpose"] = "changed"

				return actual
			}
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, envelope := finalizationFixture(t)
			calls := []string{}
			store := &finalizationOutcomeStoreStub{
				outcome: BalanceEngineRecoveryOutcome{TransactionStatus: constant.APPROVED, LifecyclePhase: TransactionLifecyclePhaseCreated},
				err:     scenario.storeErr,
				calls:   &calls,
			}
			metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}
			if scenario.metadataFn != nil {
				scenario.metadataFn(metadata)
			}

			result, err := NewBalanceEngineFinalizer(store, metadata).FinalizeWithOutcome(ctx, envelope)

			if scenario.name == "metadata compare" {
				require.ErrorIs(t, err, ErrBalanceEngineMetadataConflict)
			} else {
				require.ErrorIs(t, err, failure)
			}
			assert.Equal(t, BalanceEngineFinalizationResult{}, result)
			if scenario.storeErr != nil {
				assert.Equal(t, []string{"sql-with-outcome"}, calls)
			}
		})
	}
}

func TestBalanceEngineFinalizerRequiresValidDurableOutcome(t *testing.T) {
	t.Run("store capability", func(t *testing.T) {
		ctx, envelope := finalizationFixture(t)
		finalizer, _, _, calls := finalizationDependencies()

		result, err := finalizer.FinalizeWithOutcome(ctx, envelope)

		require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
		assert.Empty(t, result.Outcome.TransactionStatus)
		assert.Empty(t, *calls)
	})

	for _, status := range []string{"", constant.CREATED, constant.NOTED, "UNKNOWN"} {
		t.Run("status "+status, func(t *testing.T) {
			ctx, envelope := finalizationFixture(t)
			calls := []string{}
			store := &finalizationOutcomeStoreStub{
				outcome: BalanceEngineRecoveryOutcome{TransactionStatus: status},
				calls:   &calls,
			}
			metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}

			result, err := NewBalanceEngineFinalizer(store, metadata).FinalizeWithOutcome(ctx, envelope)

			require.ErrorIs(t, err, ErrBalanceEnginePersistenceConflict)
			assert.Empty(t, result.Outcome.TransactionStatus)
			assert.Equal(t, []string{"sql-with-outcome"}, calls)
		})
	}
}

func TestBalanceEngineFinalizerAcceptsPendingAndRecoveredHoldTerminalOutcomes(t *testing.T) {
	for _, status := range []string{constant.PENDING, constant.APPROVED, constant.CANCELED} {
		t.Run(status, func(t *testing.T) {
			payload, result := recoveryContractFixture(t)
			payload.Action = constant.ActionHold
			payload.TransactionStatus = constant.PENDING
			var err error
			payload.IntentFingerprint, err = ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
			require.NoError(t, err)
			envelope := recoveryContractEnvelope(t, payload, result)
			ctx := tmcore.ContextWithTenantID(context.Background(), payload.TenantID)
			calls := []string{}
			store := &finalizationOutcomeStoreStub{
				outcome: BalanceEngineRecoveryOutcome{TransactionStatus: status},
				calls:   &calls,
			}
			metadata := &finalizationMetadataStub{calls: &calls, data: make(map[string]*mongodb.Metadata)}

			finalization, err := NewBalanceEngineFinalizer(store, metadata).FinalizeWithOutcome(ctx, &envelope)

			require.NoError(t, err)
			assert.Equal(t, status, finalization.Outcome.TransactionStatus)
		})
	}
}

func TestBalanceEngineFinalizerPropagatesFailures(t *testing.T) {
	for _, stage := range []string{"sql", "create", "find"} {
		t.Run(stage, func(t *testing.T) {
			ctx, envelope := finalizationFixture(t)
			finalizer, store, metadata, calls := finalizationDependencies()
			failure := errors.New("durability is not confirmed")
			switch stage {
			case "sql":
				store.err = failure
			case "create":
				metadata.createErr = failure
			case "find":
				metadata.findErr = failure
			}

			require.ErrorIs(t, finalizer.Finalize(ctx, envelope), failure)
			if stage == "sql" {
				assert.Equal(t, []string{"sql"}, *calls)
			}
		})
	}
}

func TestBalanceEngineFinalizerRepairsMetadataAfterSQLReplay(t *testing.T) {
	ctx, envelope := finalizationFixture(t)
	finalizer, store, metadata, calls := finalizationDependencies()
	failure := errors.New("operation metadata unavailable")
	metadata.create = func(collection string) error {
		if collection == constant.EntityOperation {
			return failure
		}

		return nil
	}
	require.ErrorIs(t, finalizer.Finalize(ctx, envelope), failure)
	require.Len(t, metadata.data, 1)
	metadata.create = nil
	require.NoError(t, finalizer.Finalize(ctx, envelope))
	require.Len(t, metadata.data, 2)
	require.Len(t, store.records, 2)
	assert.Equal(t, store.records[0].Transaction.Operations[0].ID, store.records[1].Transaction.Operations[0].ID)
	assert.Equal(t, []string{
		"sql", "create:" + constant.EntityTransaction, "find:" + constant.EntityTransaction, "create:" + constant.EntityOperation,
		"sql", "create:" + constant.EntityTransaction, "find:" + constant.EntityTransaction, "create:" + constant.EntityOperation, "find:" + constant.EntityOperation,
	}, *calls)
}

func TestBalanceEngineFinalizerRejectsUnconfirmedMetadata(t *testing.T) {
	for _, scenario := range []struct {
		name string
		find func(*mongodb.Metadata) *mongodb.Metadata
	}{
		{"missing document", func(*mongodb.Metadata) *mongodb.Metadata { return nil }},
		{"different identity", func(m *mongodb.Metadata) *mongodb.Metadata { m.EntityID = "different"; return m }},
		{"different entity", func(m *mongodb.Metadata) *mongodb.Metadata { m.EntityName = "different"; return m }},
		{"different content", func(m *mongodb.Metadata) *mongodb.Metadata { m.Data["purpose"] = "updated"; return m }},
		{"rounded integer", func(m *mongodb.Metadata) *mongodb.Metadata { m.Data["sequence"] = float64(9007199254740992); return m }},
		{"numeric string", func(m *mongodb.Metadata) *mongodb.Metadata { m.Data["sequence"] = "9007199254740993"; return m }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, envelope := finalizationFixture(t)
			finalizer, _, metadata, _ := finalizationDependencies()
			metadata.find = scenario.find
			require.ErrorIs(t, finalizer.Finalize(ctx, envelope), ErrBalanceEngineMetadataConflict)
		})
	}
}

func TestBalanceEngineFinalizerDoesNotOverwriteExistingMetadata(t *testing.T) {
	ctx, envelope := finalizationFixture(t)
	finalizer, _, metadata, _ := finalizationDependencies()
	key := constant.EntityTransaction + ":" + envelope.TransactionID.String()
	metadata.data[key] = &mongodb.Metadata{EntityID: envelope.TransactionID.String(), EntityName: constant.EntityTransaction, Data: mongodb.JSON{"purpose": "later authorized edit"}}
	require.ErrorIs(t, finalizer.Finalize(ctx, envelope), ErrBalanceEngineMetadataConflict)
	assert.Equal(t, mongodb.JSON{"purpose": "later authorized edit"}, metadata.data[key].Data)
}

func TestBalanceEngineFinalizerRejectsScopeBeforeSQL(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*BalanceEngineRecoveryEnvelope)
	}{
		{"tenant", func(e *BalanceEngineRecoveryEnvelope) { e.TenantID = "tenant-b" }},
		{"ledger", func(e *BalanceEngineRecoveryEnvelope) { e.LedgerID = e.OrganizationID }},
		{"fingerprint", func(e *BalanceEngineRecoveryEnvelope) { e.IntentFingerprint = "invalid" }},
		{"result transaction", func(e *BalanceEngineRecoveryEnvelope) { e.Result.Movements[0].TransactionID = uuid.Nil }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, envelope := finalizationFixture(t)
			finalizer, _, _, calls := finalizationDependencies()
			scenario.mutate(envelope)
			require.Error(t, finalizer.Finalize(ctx, envelope))
			assert.Empty(t, *calls)
		})
	}
}

func TestBalanceEngineFinalizerPreflightsLateMetadataBeforeSQL(t *testing.T) {
	ctx, envelope := finalizationFixture(t)
	payload, err := DecodeBalanceEngineRecoveryPayload([]byte(envelope.Payload))
	require.NoError(t, err)
	payload.Projection[0].Metadata["unrepresentable"] = json.Number("0.12345678901234567890123456789")
	payload.IntentFingerprint, err = ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(*payload))
	require.NoError(t, err)
	updated := recoveryContractEnvelope(t, *payload, envelope.Result)
	finalizer, _, _, calls := finalizationDependencies()
	require.ErrorIs(t, finalizer.Finalize(ctx, &updated), ErrBalanceEngineMetadataConflict)
	assert.Empty(t, *calls)
}

func TestBalanceEngineFinalizerLifecycleReconstruction(t *testing.T) {
	for _, scenario := range []struct{ action, inputStatus, status, expected string }{
		{"direct", constant.CREATED, constant.APPROVED, ""},
		{"revert", constant.CREATED, constant.APPROVED, ""},
		{"hold", constant.PENDING, constant.PENDING, ""},
		{"commit", constant.APPROVED, constant.APPROVED, constant.PENDING},
		{"cancel", constant.CANCELED, constant.CANCELED, constant.PENDING},
	} {
		t.Run(scenario.action, func(t *testing.T) {
			payload, result := recoveryContractFixture(t)
			payload.Action, payload.TransactionStatus = scenario.action, scenario.inputStatus
			payload.Validate = nil
			payload.TransactionInput.Send.Source.From = []mtransaction.FromTo{{AccountAlias: "0#@source#default"}}
			payload.TransactionInput.Send.Distribute.To = []mtransaction.FromTo{{AccountAlias: "0#@destination#default"}}
			record, err := ComposeBalanceEnginePersistenceRecord(payload, result)
			require.NoError(t, err)
			assert.Equal(t, scenario.status, record.Transaction.Status.Code)
			assert.Equal(t, scenario.expected, record.ExpectedStatus)
			assert.Equal(t, []string{"@source"}, record.Transaction.Source)
			assert.Equal(t, []string{"@destination"}, record.Transaction.Destination)
			if scenario.status == constant.PENDING {
				assert.Equal(t, payload.TransactionInput, record.Transaction.Body)
			} else {
				assert.True(t, record.Transaction.Body.IsEmpty())
			}
		})
	}
}

func TestBalanceEngineFinalizerEmptyTenantAndMetadata(t *testing.T) {
	payload, result := recoveryContractFixture(t)
	payload.TenantID = ""
	payload.TransactionInput.Metadata, payload.Projection[0].Metadata = nil, nil
	var err error
	payload.IntentFingerprint, err = ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	envelope := recoveryContractEnvelope(t, payload, result)
	finalizer, _, _, calls := finalizationDependencies()
	require.NoError(t, finalizer.Finalize(context.Background(), &envelope))
	assert.Equal(t, []string{"sql"}, *calls)
}

func TestBalanceEngineFinalizerCancellationAndMissingDependencies(t *testing.T) {
	ctx, envelope := finalizationFixture(t)
	finalizer, _, _, calls := finalizationDependencies()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, finalizer.Finalize(canceled, envelope), context.Canceled)
	assert.Empty(t, *calls)
	require.ErrorIs(t, finalizer.Finalize(ctx, nil), ErrInvalidBalanceEngineRecovery)
	require.ErrorIs(t, NewBalanceEngineFinalizer(nil, nil).Finalize(ctx, envelope), ErrInvalidBalanceEngineRecovery)
}

func TestFrozenMetadataNumericRoundTrip(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		input    any
		expected any
	}{
		{"large integer", json.Number("9007199254740993"), int64(9007199254740993)},
		{"min integer", json.Number("-9223372036854775808"), int64(math.MinInt64)},
		{"integer exponent", json.Number("1.000e3"), int64(1000)},
		{"fraction", json.Number("0.100"), float64(0.1)},
		{"tiny fraction", json.Number("1e-100"), float64(1e-100)},
		{"signed zero", json.Number("-0"), int64(0)},
		{"bool", true, true},
		{"string", "9007199254740993", "9007199254740993"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			normalized, err := normalizeFrozenMetadata(map[string]any{"value": scenario.input})
			require.NoError(t, err)
			assert.Equal(t, scenario.expected, normalized["value"])
			encoded, err := bson.Marshal(normalized)
			require.NoError(t, err)
			var decoded mongodb.JSON
			require.NoError(t, bson.Unmarshal(encoded, &decoded))
			require.NoError(t, compareFrozenMetadata(normalized, decoded))
		})
	}
}

func TestFrozenMetadataRejectsLossAndUnsupportedValues(t *testing.T) {
	for _, value := range []any{json.Number("0.123456789012345678901"), uint64(math.MaxUint64), json.Number("1e1000000000"), json.Number("1e-1000000000"), json.Number("01"), math.NaN(), math.Inf(1), []string{"nested"}, map[string]any{"nested": true}} {
		_, err := normalizeFrozenMetadata(map[string]any{"value": value})
		require.ErrorIs(t, err, ErrBalanceEngineMetadataConflict)
	}
}

func TestFrozenMetadataComparesExactNumericSemantics(t *testing.T) {
	precise, err := bson.ParseDecimal128("9007199254740993")
	require.NoError(t, err)
	for _, actual := range []any{int64(9007199254740993), json.Number("9007199254740993.0"), precise} {
		require.NoError(t, compareFrozenMetadata(mongodb.JSON{"value": json.Number("9007199254740993")}, mongodb.JSON{"value": actual}))
	}

	require.NoError(t, compareFrozenMetadata(mongodb.JSON{"value": int64(1)}, mongodb.JSON{"value": int32(1)}))
	require.ErrorIs(t, compareFrozenMetadata(mongodb.JSON{"value": int64(9007199254740993)}, mongodb.JSON{"value": float64(9007199254740992)}), ErrBalanceEngineMetadataConflict)
	require.ErrorIs(t, compareFrozenMetadata(mongodb.JSON{"value": int64(1)}, mongodb.JSON{"value": "1"}), ErrBalanceEngineMetadataConflict)
}
