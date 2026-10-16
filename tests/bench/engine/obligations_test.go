//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	outboxpg "github.com/LerianStudio/lib-commons/v7/commons/outbox/postgres"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

var (
	keyPosted  = events.TransactionPostedDefinition.Key()
	keyChanged = events.BalanceChangedDefinition.Key()
	keyApplied = events.FeesAppliedDefinition.Key()
)

// Each fixture owes exactly the obligations completion leaves for it: the event route, the metadata documents
// with the fee engine's marks and the one Tracer confirm, with every event reporting the moved amounts.
func TestObligationCatalog(t *testing.T) {
	in, err := decodeCreateV2Body()
	require.NoError(t, err)
	require.Len(t, in.Debits, 1)
	require.Len(t, in.Credits, 1)

	// Writers post this body against the seed, so it must name the seeded accounts. Only the metadata fixture
	// keeps its client metadata; exact2leg and feeLeg post the body without it.
	src, dst, feeAccount := benchAlias(classOrdinary, 0), benchAlias(classOrdinary, 1), benchAlias(classFee, 0)
	assert.Equal(t, []any{src, benchOrgID, benchLedgerID}, []any{in.Debits[0].Alias, in.Debits[0].OrganizationID, in.Debits[0].LedgerID})
	assert.Equal(t, []any{dst, benchOrgID, benchLedgerID}, []any{in.Credits[0].Alias, in.Credits[0].OrganizationID, in.Credits[0].LedgerID})
	t.Logf("input create_v2_body.json=%dB", len(createV2Body))

	transfer := map[string]string{src: "-100", dst: "100"}
	feeTx := map[string]any{"packageAppliedID": benchFeePackage.String(), "feeApplied": "true"}
	feeLeg := map[string]any{"feeLeg": "true", "source": dst}
	want := map[string]struct {
		counts   map[string]int    // exact rows by kind
		moved    map[string]string // signed amount by alias
		metadata []map[string]any  // metadata documents, transaction first
	}{
		"exact2leg": {map[string]int{keyPosted: 1, keyChanged: 2, kindTracerConfirm: 1}, transfer, nil},
		"metadata": {
			map[string]int{keyPosted: 1, keyChanged: 2, kindMetadataUpsert: 1, kindTracerConfirm: 1},
			transfer,
			[]map[string]any{in.Metadata},
		},
		"feeLeg": {
			map[string]int{keyPosted: 1, keyChanged: 3, keyApplied: 1, kindMetadataUpsert: 2, kindTracerConfirm: 1},
			map[string]string{src: "-100", dst: "98", feeAccount: "2"},
			[]map[string]any{feeTx, feeLeg},
		},
	}
	require.Len(t, want, len(obligationFixtures))

	for _, fx := range obligationFixtures {
		t.Run(fx.name, func(t *testing.T) {
			tc, ok := want[fx.name]
			require.True(t, ok)

			ob, err := fixtureObligations(t.Context(), in, fx, 0)
			require.NoError(t, err)

			got := decodeObligations(t, fx.name, ob)
			assert.Equal(t, tc.counts, got.counts)
			assert.Equal(t, "100", got.posted.Amount.String())
			require.Len(t, got.posted.Operations, tc.counts[keyChanged])

			moved := map[string]string{}

			for _, raw := range got.posted.Operations {
				var op benchOperation
				require.NoError(t, json.Unmarshal(raw, &op))
				assert.Equal(t, in.Description, op.Description, "operations take the transaction description")

				amount := op.Amount.Value
				if op.Type == constant.DEBIT {
					amount = amount.Neg()
				}

				moved[op.AccountAlias] = amount.String()
				assert.Equal(t, decimal.NewFromInt(benchFunds).Add(amount).String(), got.changed[op.BalanceID].Available.String(),
					"balance.changed available after the movement")
			}

			assert.Equal(t, tc.moved, moved, "operations move the debit to the credits")
			assert.Equal(t, tc.metadata, got.metadata)

			var txMetadata map[string]any
			if len(tc.metadata) > 0 {
				txMetadata = tc.metadata[0]
			}

			assert.Equal(t, txMetadata, got.posted.Metadata, "posted carries the transaction metadata")
		})
	}
}

// A decided fee request owes what its records say: two keys of one account debited 30 + 20, credited 49 + fee 1,
// each operation carried as recorded, the fee event under the package its metadata names, and metadata documents
// dated at the transaction's creation, as frozenMetadataRecords dates a v2 create.
func TestBuildObligations_DecidedFeeRequest_CarriesRecords(t *testing.T) {
	t.Parallel()

	requestID := uuid.MustParse("019a0f3e-7b2c-7d10-8a6e-5f0c2d9b1a01")
	txID, feePackage := "019a0f3e-7b2c-7d10-8a6e-5f0c2d9b1a02", "019a0f3e-7b2c-7d10-8a6e-5f0c2d9b1a03"
	createdAt := time.Date(2026, 10, 16, 14, 3, 7, 123456789, time.UTC)
	txUpdatedAt, opUpdatedAt := createdAt.Add(2*time.Second), createdAt.Add(3*time.Second) // separate captures

	decided := func(n int, class, key string, debit bool, amount, before, after string, version int64) benchOperation {
		opType, direction := constant.CREDIT, constant.DirectionCredit
		if debit {
			opType, direction = constant.DEBIT, constant.DirectionDebit
		}

		return benchOperation{
			ID: fmt.Sprintf("019a0f3e-7b2c-7d10-8a6e-5f0c2d9b1b0%d", n), TransactionID: txID, Type: opType, AssetCode: "BRL",
			Amount:       benchAmount{Value: decimal.RequireFromString(amount)},
			Balance:      benchBalance{Available: decimal.RequireFromString(before), OnHold: decimal.NewFromInt(7), Version: version},
			BalanceAfter: benchBalance{Available: decimal.RequireFromString(after), OnHold: decimal.NewFromInt(7), Version: version + 1},
			AccountID:    accountID(class, 0).String(), AccountAlias: benchAlias(class, 0), BalanceKey: key,
			BalanceID: balanceID(class, 0, key).String(), OrganizationID: benchOrgID.String(), LedgerID: benchLedgerID.String(),
			BalanceAffected: true, Direction: direction, CreatedAt: createdAt, UpdatedAt: opUpdatedAt,
		}
	}
	payer, payee, feeAccount := benchAlias(classMultikey, 0), benchAlias(classHot, 0), benchAlias(classFee, 0)
	ops := []benchOperation{
		decided(1, classMultikey, constant.DefaultBalanceKey, true, "30", "41.5", "11.5", 7),
		decided(2, classMultikey, keySecondary, true, "20", "20.25", "0.25", 3),
		decided(3, classHot, constant.DefaultBalanceKey, false, "49", "900", "949", 41),
		decided(4, classFee, constant.DefaultBalanceKey, false, "1", "12", "13", 9),
	}
	ops[3].Metadata = map[string]any{constant.MetadataKeyFeeLeg: constant.MetadataValueFeeLeg, "source": payee}

	amount, approved := decimal.NewFromInt(50), constant.APPROVED
	txMetadata := map[string]any{"packageAppliedID": feePackage, "feeApplied": "true"}

	ob, err := buildObligations(t.Context(), requestID, events.TransactionSource{
		ID: txID, OrganizationID: benchOrgID.String(), LedgerID: benchLedgerID.String(), Status: mmodel.Status{Code: approved, Description: &approved},
		Amount: &amount, AssetCode: "BRL", Source: []string{payer, payer}, Destination: []string{payee, feeAccount}, Metadata: txMetadata,
		CreatedAt: createdAt, UpdatedAt: txUpdatedAt,
	}, ops)
	require.NoError(t, err)

	got := decodeObligations(t, "decided", ob)
	assert.Equal(t, map[string]int{keyPosted: 1, keyChanged: 4, keyApplied: 1, kindMetadataUpsert: 2, kindTracerConfirm: 1}, got.counts)
	assert.Equal(t, []string{createdAt.Format(time.RFC3339), txUpdatedAt.Format(time.RFC3339)}, []string{got.posted.CreatedAt, got.posted.UpdatedAt})
	require.Len(t, got.posted.Operations, len(ops))

	for i, want := range ops {
		recorded, err := json.Marshal(want)
		require.NoError(t, err)
		assert.JSONEq(t, string(recorded), string(got.posted.Operations[i]), "posted operation %d is its record", i)

		changed, ok := got.changed[want.BalanceID]
		require.True(t, ok, "balance.changed for %s", want.BalanceID)
		assert.Equal(t,
			[]any{
				want.ID, txID, want.AccountID, want.AccountAlias, want.BalanceKey, want.Amount.Value.String(),
				want.BalanceAfter.Available.String(), want.BalanceAfter.OnHold.String(), want.BalanceAfter.Version, createdAt.Format(time.RFC3339),
			},
			[]any{
				changed.OperationID, changed.TransactionID, changed.AccountID, changed.Alias, changed.BalanceKey, changed.Amount.String(),
				changed.Available.String(), changed.OnHold.String(), changed.Version, changed.OccurredAt,
			},
			"balance.changed %d reports its balance after the operation", i)
	}

	assert.Equal(t, []metadataUpsert{
		{EntityName: constant.EntityTransaction, EntityID: txID, Data: txMetadata, CreatedAt: createdAt, UpdatedAt: createdAt},
		{EntityName: constant.EntityOperation, EntityID: ops[3].ID, Data: ops[3].Metadata, CreatedAt: createdAt, UpdatedAt: createdAt},
	}, got.docs, "metadata documents carry the transaction's creation, not later updates")
}

// decodedObligations is one request's obligations read back from their payloads.
type decodedObligations struct {
	counts   map[string]int
	posted   events.TransactionPayload
	changed  map[string]events.BalanceChangedPayload // by balance id: one account's aliases repeat across keys
	metadata []map[string]any                        // metadata documents' data in row order
	docs     []metadataUpsert
}

// decodeObligations decodes every payload and logs its size, checking fee_charge.applied, the request grouping
// of private rows and the transaction the Tracer confirm correlates.
func decodeObligations(t *testing.T, fixture string, ob requestObligations) decodedObligations {
	t.Helper()

	got := decodedObligations{counts: map[string]int{}, changed: map[string]events.BalanceChangedPayload{}}

	for _, ev := range ob.events {
		t.Logf("%s %s payload=%dB", fixture, ev.DefinitionKey, len(ev.Payload))
		got.counts[ev.DefinitionKey]++

		switch ev.DefinitionKey {
		case keyPosted:
			require.NoError(t, json.Unmarshal(ev.Payload, &got.posted))
		case keyChanged:
			var changed events.BalanceChangedPayload
			require.NoError(t, json.Unmarshal(ev.Payload, &changed))

			got.changed[changed.BalanceID] = changed
		case keyApplied:
			var applied events.FeesAppliedPayload
			require.NoError(t, json.Unmarshal(ev.Payload, &applied))
			assert.Equal(t, []any{got.posted.ID, got.posted.Metadata["packageAppliedID"], got.posted.CreatedAt},
				[]any{applied.TransactionID, applied.FeePackageID, applied.AppliedAt}, "fee_charge.applied for the package and time the transaction carries")
		}
	}

	for _, row := range ob.private {
		t.Logf("%s %s payload=%dB", fixture, row.EventType, len(row.Payload))
		got.counts[row.EventType]++
		assert.Equal(t, ob.requestID, row.AggregateID, "private rows group by request")

		switch row.EventType {
		case kindMetadataUpsert:
			var doc metadataUpsert
			require.NoError(t, json.Unmarshal(row.Payload, &doc))

			got.metadata, got.docs = append(got.metadata, doc.Data), append(got.docs, doc)
		case kindTracerConfirm:
			var confirm tracerConfirm
			require.NoError(t, json.Unmarshal(row.Payload, &confirm))
			assert.Equal(t, got.posted.ID, confirm.TransactionID)
		}
	}

	return got
}

var errBrokerRefused = errors.New("bench broker refuses publish: obligations stay in the outbox")

// refusingBroker is the producer's only transport, the external I/O boundary: EmitBatch writes the outbox
// and never publishes, so every Publish is counted and refused.
type refusingBroker struct{ publishes atomic.Int64 }

func (*refusingBroker) Kind() libStreaming.TransportKind { return libStreaming.TransportKafkaLike }

func (b *refusingBroker) Publish(context.Context, libStreaming.TransportMessage) error {
	b.publishes.Add(1)

	return errBrokerRefused
}

func (*refusingBroker) Healthy(context.Context) error { return nil }

func (*refusingBroker) Flush(context.Context) error { return nil }

func (*refusingBroker) Close(context.Context) error { return nil }

func (*refusingBroker) Classify(error) libStreaming.ErrorClass { return libStreaming.ClassValidation }

// newTestOutbox builds the ledger's producer over the lib-commons repository on a pool the client owns, the refusing
// broker its only transport, and lends that pool for transactions. Cleanup closes the producer, then client and pool.
func newTestOutbox(ctx context.Context, t *testing.T, env *benchEnv) (*sql.DB, libStreaming.TransactionalBatchEmitter, *outboxpg.Repository, *refusingBroker) {
	t.Helper()

	db := stdlib.OpenDB(*env.primary)

	client, err := libPostgres.NewFromPools(db, nil, libPostgres.Config{})
	if err != nil {
		assert.NoError(t, db.Close())
		require.NoError(t, err)
	}

	t.Cleanup(func() { assert.NoError(t, client.Close()) })

	resolver, err := outboxpg.NewSchemaResolver(client, outboxpg.WithAllowEmptyTenant())
	require.NoError(t, err)

	repo, err := outboxpg.NewRepository(client, resolver, resolver)
	require.NoError(t, err)

	broker := &refusingBroker{}

	emitter, err := newOutboxProducer(ctx, repo, broker)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, emitter.Close()) }) // idempotent after a test's own Close

	return db, emitter, repo, broker
}

// One fee request's eight obligations commit in one transaction: events through the lib-streaming producer,
// private rows through the lib-commons repository. A rolled-back request leaves no row; no broker is called.
func TestOutboxOnlyEmitBatch(t *testing.T) {
	env := requireDB(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	// The guarded rebuild empties the outbox and drops any seeded balances; these rows need no seed.
	require.NoError(t, bootstrapSchema(ctx, env))

	db, emitter, repo, broker := newTestOutbox(ctx, t, env)

	in, err := decodeCreateV2Body()
	require.NoError(t, err)

	fx := obligationFixtures[2]
	require.Equal(t, "feeLeg", fx.name)

	committed, err := fixtureObligations(ctx, in, fx, 0)
	require.NoError(t, err)

	rolledBack, err := fixtureObligations(ctx, in, fx, 1)
	require.NoError(t, err)

	for i, ob := range []requestObligations{committed, rolledBack} {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { // a failed assertion below must not leave the transaction open
			if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
				assert.NoError(t, err, "roll back request %d", i)
			}
		})

		require.NoError(t, emitter.EmitBatch(libStreaming.WithOutboxTx(ctx, tx), ob.events))

		_, err = repo.CreateManyWithTx(ctx, tx, ob.private)
		require.NoError(t, err)

		end := tx.Commit
		if i > 0 {
			end = tx.Rollback
		}

		require.NoError(t, end())
	}

	require.NoError(t, emitter.Close())
	assert.Zero(t, broker.publishes.Load(), "outbox-only emission must never reach the broker")

	scanned, streaming := assertStoredOutbox(ctx, t, db, committed)
	assert.Equal(t, [2]int{8, 5}, [2]int{scanned, streaming}, "the committed request's rows only: the rolled-back one left none")
}

// assertStoredOutbox reads every outbox row back, requiring each to be one of the request's obligations, stored
// once with its kind and payload intact, and returns the rows scanned and how many the producer wrote.
func assertStoredOutbox(ctx context.Context, t *testing.T, db *sql.DB, ob requestObligations) (scanned, streaming int) {
	t.Helper()

	type stored struct{ kind, payload string }

	want := make(map[string]stored, len(ob.events)+len(ob.private)) // by EventID or private row id
	for _, ev := range ob.events {
		want[ev.EventID] = stored{ev.DefinitionKey, string(ev.Payload)}
	}

	for _, row := range ob.private {
		want[row.ID.String()] = stored{row.EventType, string(row.Payload)}
	}

	rows, err := db.QueryContext(ctx, `SELECT id, event_type, aggregate_id, payload::text, octet_length(payload::text),
		pg_column_size(payload) FROM outbox_events ORDER BY event_type, id`)
	require.NoError(t, err)

	defer func() { assert.NoError(t, rows.Close()) }()

	seen := map[string]bool{}

	for rows.Next() {
		var (
			id, aggregate      uuid.UUID
			got                stored
			textBytes, colSize int
		)

		require.NoError(t, rows.Scan(&id, &got.kind, &aggregate, &got.payload, &textBytes, &colSize))

		key := id.String()

		if got.kind == libStreaming.StreamingOutboxEventType {
			var envelope libStreaming.OutboxEnvelope
			require.NoError(t, json.Unmarshal([]byte(got.payload), &envelope))

			key, streaming = envelope.Event.EventID, streaming+1
			got = stored{envelope.DefinitionKey, string(envelope.Event.Payload)}
		} else {
			assert.Equal(t, ob.requestID, aggregate, "private row %s groups by request", id)
		}

		require.Contains(t, want, key, "stored %s %s was not committed", got.kind, key)
		require.False(t, seen[key], "stored %s %s twice", got.kind, key)
		assert.Equal(t, want[key].kind, got.kind)
		assert.JSONEq(t, want[key].payload, got.payload, "%s payload persisted intact", got.kind)
		t.Logf("%s payload=%dB stored_text=%dB pg_column_size=%dB aggregate=%s", got.kind, len(want[key].payload), textBytes, colSize, aggregate)

		seen[key], scanned = true, scanned+1
	}

	require.NoError(t, rows.Err())

	return scanned, streaming
}
