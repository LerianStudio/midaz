//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	_ "embed" // testdata/create_v2_body.json
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/outbox"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// Private outbox event types of the bench; lib-streaming names only its own publish rows.
const (
	kindMetadataUpsert = "ledger.metadata.upsert"
	kindTracerConfirm  = "tracer.reservation.confirm"
)

var (
	//go:embed testdata/create_v2_body.json
	createV2Body []byte

	// Fractional seconds give these fixtures the RFC3339Nano width product timestamps carry.
	benchTxTime     = benchSeedTime.Add(time.Hour + 123456789*time.Nanosecond)
	benchFeePackage = uuid.NewSHA1(benchNS, []byte("fee-package"))
)

// obligationFixture is one approved v2 create of the body with one known Tracer reservation. Only metadata keeps
// the body's client metadata; exact2leg and feeLeg post it without. The fee assumes one deductible,
// non-deferrable fee without accounting route; no fee configuration or fee engine runs here.
type obligationFixture struct {
	name     string
	metadata bool            // keep the body's client metadata
	fee      decimal.Decimal // carved out of the credit; zero is none
}

var obligationFixtures = [...]obligationFixture{
	{name: "exact2leg"},
	{name: "metadata", metadata: true},
	{name: "feeLeg", fee: decimal.NewFromInt(2)},
}

// requestObligations is what completion owes after one request commits: events written through the
// lib-streaming producer, metadata and Tracer rows through the lib-commons repository.
type requestObligations struct {
	requestID uuid.UUID
	events    []libStreaming.EmitRequest
	private   []*outbox.OutboxEvent
}

// createV2Input holds the fields of the v2 create body the fixtures read.
type createV2Input struct {
	Description string          `json:"description"`
	Asset       string          `json:"asset"`
	Amount      decimal.Decimal `json:"amount"`
	Debits      []createV2Leg   `json:"debits"`
	Credits     []createV2Leg   `json:"credits"`
	Metadata    map[string]any  `json:"metadata"`
}

type createV2Leg struct {
	Alias          string    `json:"alias"`
	OrganizationID uuid.UUID `json:"organizationId"`
	LedgerID       uuid.UUID `json:"ledgerId"`
}

func decodeCreateV2Body() (createV2Input, error) {
	var in createV2Input
	if err := json.Unmarshal(createV2Body, &in); err != nil {
		return createV2Input{}, fmt.Errorf("decode create_v2_body.json: %w", err)
	}

	return in, nil
}

// benchOperation is one transaction.posted operation as send_transaction_events.go marshals it: the
// ledger's internal operation.Operation plus accountType. tests/bench cannot import that type, so this
// mirrors its keys; routeId, routeCode and routeDescription are omitempty and unset in these fixtures.
type benchOperation struct {
	ID              string         `json:"id"`
	TransactionID   string         `json:"transactionId"`
	Description     string         `json:"description"`
	Type            string         `json:"type"`
	AssetCode       string         `json:"assetCode"`
	ChartOfAccounts string         `json:"chartOfAccounts"`
	Amount          benchAmount    `json:"amount"`
	Balance         benchBalance   `json:"balance"`
	BalanceAfter    benchBalance   `json:"balanceAfter"`
	Status          mmodel.Status  `json:"status"`
	AccountID       string         `json:"accountId"`
	AccountAlias    string         `json:"accountAlias"`
	BalanceKey      string         `json:"balanceKey"`
	BalanceID       string         `json:"balanceId"`
	OrganizationID  string         `json:"organizationId"`
	LedgerID        string         `json:"ledgerId"`
	Route           string         `json:"route"`
	BalanceAffected bool           `json:"balanceAffected"`
	Direction       string         `json:"direction,omitempty"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       *time.Time     `json:"deletedAt"`
	Metadata        map[string]any `json:"metadata"`
	AccountType     string         `json:"accountType"`
}

type benchAmount struct {
	Value decimal.Decimal `json:"value"`
}

type benchBalance struct {
	Available     decimal.Decimal `json:"available"`
	OnHold        decimal.Decimal `json:"onHold"`
	Version       int64           `json:"version"`
	OverdraftUsed decimal.Decimal `json:"overdraftUsed"`
}

// metadataUpsert is a bench-defined shape freezing the fields frozenMetadataRecords sets on one Mongo
// metadata document; Mongo generates _id when the document is projected.
type metadataUpsert struct {
	EntityName string         `json:"entityName"`
	EntityID   string         `json:"entityId"`
	Data       map[string]any `json:"data"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
}

// tracerConfirm correlates the known reservation with its transaction, as the retrier's transition does. It is
// not the RPC body: Confirm sends only the reservation id.
type tracerConfirm struct {
	TransactionID string `json:"transactionId"`
	ReservationID string `json:"reservationId"`
}

// benchLeg is one movement on a seeded default balance.
type benchLeg struct {
	class    string
	index    int
	alias    string
	debit    bool
	amount   decimal.Decimal
	metadata map[string]any
}

// fixtureObligations returns what request n of the fixture owes, its legs recorded on their seeded balances.
func fixtureObligations(ctx context.Context, in createV2Input, fx obligationFixture, n int) (requestObligations, error) {
	requestID := uuid.NewSHA1(benchNS, fmt.Appendf(nil, "request/%s/%d", fx.name, n))
	legs, metadata := fixtureLegs(in, fx)
	approved := constant.APPROVED
	tx := events.TransactionSource{
		ID: obligationID(requestID, "transaction", 0).String(), OrganizationID: benchOrgID.String(), LedgerID: benchLedgerID.String(),
		Status: mmodel.Status{Code: approved, Description: &approved}, Amount: &in.Amount, AssetCode: in.Asset,
		Description: in.Description, Metadata: metadata, CreatedAt: benchTxTime, UpdatedAt: benchTxTime,
	}
	ops := make([]benchOperation, len(legs))

	for i, leg := range legs {
		ops[i] = leg.operation(in, tx.ID, obligationID(requestID, "operation", i).String())

		if leg.debit {
			tx.Source = append(tx.Source, leg.alias)
		} else {
			tx.Destination = append(tx.Destination, leg.alias)
		}
	}

	return buildObligations(ctx, requestID, tx, ops)
}

// buildObligations returns what a decided v2 create owes: transaction.posted with its operations, one balance.changed
// per operation at its after state, fee_charge.applied under send_transaction_events.go's metadata gate, one metadata
// document per non-nil metadata (transaction first) dated at tx.CreatedAt, and the Tracer confirm. No fee debt opens.
func buildObligations(ctx context.Context, requestID uuid.UUID, tx events.TransactionSource, ops []benchOperation) (requestObligations, error) {
	var docs []metadataUpsert

	addDoc := func(entity, id string, data map[string]any) { // frozenMetadataRecords: a v2 create's TransactionDate
		if data != nil {
			docs = append(docs, metadataUpsert{EntityName: entity, EntityID: id, Data: data, CreatedAt: tx.CreatedAt, UpdatedAt: tx.CreatedAt})
		}
	}
	addDoc(constant.EntityTransaction, tx.ID, tx.Metadata)

	ob := requestObligations{requestID: requestID, events: make([]libStreaming.EmitRequest, 1, len(ops)+2)}
	tx.Operations = make([]json.RawMessage, 0, len(ops))

	for i, op := range ops {
		raw, err := json.Marshal(op)
		if err != nil {
			return requestObligations{}, fmt.Errorf("marshal operation %d: %w", i, err)
		}

		changed, err := balanceChanged(op)
		if err != nil {
			return requestObligations{}, err
		}

		tx.Operations, ob.events = append(tx.Operations, raw), append(ob.events, changed)

		addDoc(constant.EntityOperation, op.ID, op.Metadata)
	}

	posted, err := events.NewTransactionPosted(tx).ToEmitRequestPosted("", tx.CreatedAt)
	if err != nil {
		return requestObligations{}, err
	}

	ob.events[0] = posted

	feeApplied, _ := tx.Metadata["feeApplied"].(string)
	if packageID, _ := tx.Metadata["packageAppliedID"].(string); feeApplied == "true" && packageID != "" {
		applied, err := events.NewFeesApplied(tx.ID, tx.OrganizationID, tx.LedgerID, packageID, tx.CreatedAt).ToEmitRequest("", tx.CreatedAt)
		if err != nil {
			return requestObligations{}, err
		}

		ob.events = append(ob.events, applied)
	}

	for i := range ob.events {
		ob.events[i].EventID = obligationID(requestID, ob.events[i].DefinitionKey, i).String()
	}

	return ob.withPrivate(ctx, docs, tracerConfirm{TransactionID: tx.ID, ReservationID: obligationID(requestID, "reservation", 0).String()})
}

// fixtureLegs returns the posted legs and transaction metadata. A deductible fee is carved out of the credit,
// with the marks fee distribute.go generatedFeeLeg and calculate-fee.go updateFeeMetadataIfNeeded write.
func fixtureLegs(in createV2Input, fx obligationFixture) ([]benchLeg, map[string]any) {
	payer := in.Credits[0].Alias
	debit := benchLeg{class: classOrdinary, index: 0, alias: in.Debits[0].Alias, debit: true, amount: in.Amount}
	credit := benchLeg{class: classOrdinary, index: 1, alias: payer, amount: in.Amount.Sub(fx.fee)}

	var metadata map[string]any
	if fx.metadata {
		metadata = maps.Clone(in.Metadata)
	}

	if !fx.fee.IsPositive() {
		return []benchLeg{debit, credit}, metadata
	}

	if metadata == nil {
		metadata = map[string]any{}
	}

	metadata["packageAppliedID"] = benchFeePackage.String()
	metadata["feeApplied"] = "true"

	return []benchLeg{debit, credit, {
		class: classFee, alias: benchAlias(classFee, 0), amount: fx.fee,
		metadata: map[string]any{constant.MetadataKeyFeeLeg: constant.MetadataValueFeeLeg, "source": payer},
	}}, metadata
}

// operation is the leg as completion records it on a balance at its seeded state. Neither request nor fee legs
// carry a description, so newOperationRecordSpec takes the transaction's. Status stays empty: none is set.
func (leg benchLeg) operation(in createV2Input, txID, opID string) benchOperation {
	opType, direction, delta := constant.CREDIT, constant.DirectionCredit, leg.amount
	if leg.debit {
		opType, direction, delta = constant.DEBIT, constant.DirectionDebit, leg.amount.Neg()
	}

	before := benchBalance{Available: decimal.NewFromInt(benchFunds)}

	return benchOperation{
		ID: opID, TransactionID: txID, Description: in.Description, Type: opType, AssetCode: in.Asset, Amount: benchAmount{Value: leg.amount},
		Balance: before, BalanceAfter: benchBalance{Available: before.Available.Add(delta), Version: 1},
		AccountID: accountID(leg.class, leg.index).String(), AccountAlias: leg.alias, BalanceKey: constant.DefaultBalanceKey,
		BalanceID:      balanceID(leg.class, leg.index, constant.DefaultBalanceKey).String(),
		OrganizationID: benchOrgID.String(), LedgerID: benchLedgerID.String(), BalanceAffected: true, Direction: direction,
		CreatedAt: benchTxTime, UpdatedAt: benchTxTime, Metadata: leg.metadata, AccountType: "deposit", // seeded account_type
	}
}

// balanceChanged maps the operation as send_balance_changed_events.go does.
func balanceChanged(op benchOperation) (libStreaming.EmitRequest, error) {
	reason := events.BalanceChangeReasonCredit
	if op.Type == constant.DEBIT {
		reason = events.BalanceChangeReasonDebit
	}

	return events.NewBalanceChanged(events.BalanceChangedSource{
		OrganizationID: op.OrganizationID, LedgerID: op.LedgerID, AccountID: op.AccountID, BalanceID: op.BalanceID,
		Alias: op.AccountAlias, AssetCode: op.AssetCode, BalanceKey: op.BalanceKey, Available: op.BalanceAfter.Available,
		OnHold: op.BalanceAfter.OnHold, Version: op.BalanceAfter.Version, Reason: reason, OperationType: op.Type,
		Direction: op.Direction, Amount: op.Amount.Value, TransactionID: op.TransactionID, OperationID: op.ID, OccurredAt: op.CreatedAt,
	}).ToEmitRequest("", op.CreatedAt)
}

// withPrivate adds the metadata documents and the Tracer confirm as private rows grouped by request.
func (ob requestObligations) withPrivate(ctx context.Context, docs []metadataUpsert, confirm tracerConfirm) (requestObligations, error) {
	ob.private = make([]*outbox.OutboxEvent, 0, len(docs)+1)

	add := func(kind string, ordinal int, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal %s payload: %w", kind, err)
		}

		row, err := outbox.NewOutboxEventWithID(ctx, obligationID(ob.requestID, kind, ordinal), kind, ob.requestID, data)
		if err != nil {
			return err
		}

		ob.private = append(ob.private, row)

		return nil
	}

	for i, doc := range docs {
		if err := add(kindMetadataUpsert, i, doc); err != nil {
			return requestObligations{}, err
		}
	}

	if err := add(kindTracerConfirm, 0, confirm); err != nil {
		return requestObligations{}, err
	}

	return ob, nil
}

// obligationID is stable per request, kind and ordinal, so rebuilding a request repeats its IDs.
func obligationID(requestID uuid.UUID, kind string, ordinal int) uuid.UUID {
	return uuid.NewSHA1(benchNS, fmt.Appendf(nil, "%s/%s/%d", requestID, kind, ordinal))
}

// The ledger's streaming identity: bootstrap/streaming.go streamingServiceName and its primary target.
const (
	benchStreamingSource = "ledger"
	benchStreamingTarget = "primary"
)

// newOutboxProducer builds the producer the ledger builds (its source, one required catch-all route to its
// application topic) over the lib-commons outbox repository, with broker as its only transport.
func newOutboxProducer(ctx context.Context, repo outbox.OutboxRepository, broker libStreaming.TransportAdapter) (libStreaming.TransactionalBatchEmitter, error) {
	catalog, err := libStreaming.NewCatalog(pkgStreaming.CatalogEntriesFromDefinitions([]events.Definition{
		events.TransactionPostedDefinition, events.BalanceChangedDefinition, events.FeesAppliedDefinition,
	})...)
	if err != nil {
		return nil, fmt.Errorf("streaming catalog: %w", err)
	}

	topic, err := libStreaming.AppTopic(benchStreamingSource)
	if err != nil {
		return nil, fmt.Errorf("streaming topic: %w", err)
	}

	emitter, err := libStreaming.NewBuilder().
		Source(benchStreamingSource).
		Catalog(catalog).
		Routes(libStreaming.RouteDefinition{
			Key: benchStreamingSource + ".facts." + benchStreamingTarget, Target: benchStreamingTarget,
			Destination: libStreaming.KafkaTopic(topic), Requirement: libStreaming.RouteRequired,
		}).
		Target(libStreaming.TargetConfig{Name: benchStreamingTarget, Kind: libStreaming.TransportKafkaLike}).
		RegisterTransport(libStreaming.TransportKafkaLike,
			func(context.Context, libStreaming.TransportAdapterOptions) (libStreaming.TransportAdapter, error) {
				return broker, nil
			}).
		OutboxRepository(repo).
		Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("streaming producer: %w", err)
	}

	batch, ok := emitter.(libStreaming.TransactionalBatchEmitter)
	if !ok {
		return nil, errors.Join(fmt.Errorf("streaming producer %T has no EmitBatch", emitter), emitter.Close())
	}

	return batch, nil
}
