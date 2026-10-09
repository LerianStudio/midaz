//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package completion

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"testing"
	"time"

	libOutbox "github.com/LerianStudio/lib-commons/v7/commons/outbox"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libSystemFacts "github.com/LerianStudio/lib-system-facts"
	"github.com/LerianStudio/lib-system-facts/emitter"
	"github.com/LerianStudio/lib-system-facts/manifest"
	kitOutbox "github.com/LerianStudio/lib-system-facts/outbox"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/LerianStudio/midaz/v4/components/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/systemfacts"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// The names the embedded contract declares. They are spelled here on purpose,
// independently of the adapter's constants, so a drift on either side fails.
const (
	systemFactsContractsDir       = "contracts/system-facts"
	systemFactsProducer           = "midaz"
	systemFactsNamespace          = "lerian.midaz.transactions"
	systemFactsFactType           = "midaz.transaction_committed"
	systemFactsSchemaVersion      = "1.0.0"
	systemFactsSubjectType        = "midaz.transaction"
	systemFactsReferenceType      = "lerian.midaz.transaction_id"
	systemFactsScopeOrganization  = "organizationId"
	systemFactsScopeLedger        = "ledgerId"
	systemFactsEnvelopeVersion    = 1
	systemFactsProviderShutdownTO = 10 * time.Second
)

// systemFactsInfra is the harness of these proofs: the same container,
// migrations and repositories as setupRecoverySQL, plus the lib-commons client
// the kit's producer records through and the compiled embedded catalog.
type systemFactsInfra struct {
	db           *sql.DB
	client       *libPostgres.Client
	transactions transactionRepository
	operations   operationRepository
	catalog      *manifest.Catalog
}

func setupSystemFactsSQL(t *testing.T) systemFactsInfra {
	t.Helper()

	cfg := pgtestutil.DefaultContainerConfig()
	cfg.Image = "postgres:17"
	container := pgtestutil.SetupContainerWithConfig(t, cfg)
	dsn := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	migrations := pgtestutil.FindMigrationsPath(t, "transaction")
	client := pgtestutil.CreatePostgresClient(t, dsn, dsn, container.Config.DBName, migrations)

	dir, err := fs.Sub(ledger.SystemFactsContracts, systemFactsContractsDir)
	require.NoError(t, err)

	m, err := manifest.Load(dir, ".")
	require.NoError(t, err, "embedded manifest must load")

	catalog, err := m.Compile()
	require.NoError(t, err, "embedded manifest must compile")

	return systemFactsInfra{
		db:           container.DB,
		client:       client,
		transactions: transaction.NewTransactionPostgreSQLRepository(client, false),
		operations:   operation.NewOperationPostgreSQLRepository(client),
		catalog:      catalog,
	}
}

// newSystemFactsProducer builds a record-only producer (no Transport) over the
// harness database, with the given tenancy and meter provider.
func newSystemFactsProducer(t *testing.T, infra systemFactsInfra, tenancy libSystemFacts.TenancyConfig, meters *sdkmetric.MeterProvider) *libSystemFacts.Producer {
	t.Helper()

	cfg := libSystemFacts.ProducerConfig{
		Catalog:  infra.catalog,
		Postgres: infra.client,
		Tenancy:  tenancy,
		Logger:   libLog.NewNop(),
	}
	if meters != nil {
		cfg.MeterProvider = meters
	}

	producer, err := libSystemFacts.NewProducer(cfg)
	require.NoError(t, err)
	require.Nil(t, producer.Forwarder(), "a producer without a transport is record-only")

	return producer
}

// newSystemFactsStore wires the real adapter between the Store and the facts
// emitter, exactly as the composition root will.
func newSystemFactsStore(t *testing.T, infra systemFactsInfra, facts emitter.Emitter) *Store {
	t.Helper()

	recorder, err := systemfacts.NewRecorder(facts, libLog.NewNop())
	require.NoError(t, err)

	return NewStore(infra.transactions, infra.operations).WithTransactionFactRecorder(recorder)
}

func newSystemFactsMeterProvider(t *testing.T) (*sdkmetric.ManualReader, *sdkmetric.MeterProvider) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), systemFactsProviderShutdownTO)
		defer cancel()

		if err := provider.Shutdown(ctx); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}
	})

	return reader, provider
}

// systemFactsRefusals reads the kit's refusal counter: the value of the point
// labelled with reason and fact type, and whether the instrument recorded it.
func systemFactsRefusals(t *testing.T, reader *sdkmetric.ManualReader, reason emitter.RefusalReason, factType string) (int64, bool) {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(t.Context(), &rm))

	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != emitter.MetricEmitRefused {
				continue
			}

			sum, isSum := m.Data.(metricdata.Sum[int64])
			require.True(t, isSum, "%s is not an int64 sum", emitter.MetricEmitRefused)

			for _, point := range sum.DataPoints {
				labels := map[string]string{}
				for _, kv := range point.Attributes.ToSlice() {
					labels[string(kv.Key)] = kv.Value.AsString()
				}

				if labels["reason"] == string(reason) && labels["type"] == factType {
					return point.Value, true
				}
			}
		}
	}

	return 0, false
}

// systemFactsEnvelope mirrors the service envelope member for member, with the
// JSON keys spelled here so a renamed key on the wire fails the decode.
type systemFactsEnvelope struct {
	EnvelopeVersion int                                     `json:"envelopeVersion"`
	FactID          uuid.UUID                               `json:"factId"`
	Producer        string                                  `json:"producer"`
	Namespace       string                                  `json:"namespace"`
	Type            string                                  `json:"type"`
	SchemaVersion   string                                  `json:"schemaVersion"`
	OccurredAt      time.Time                               `json:"occurredAt"`
	Subject         systemFactsSubject                      `json:"subject"`
	References      []systemFactsReference                  `json:"references"`
	Data            systemfacts.TransactionCommittedPayload `json:"data"`
}

type systemFactsSubject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type systemFactsReference struct {
	Type  string            `json:"type"`
	Value string            `json:"value"`
	Scope map[string]string `json:"scope"`
}

// systemFactsOutboxRow is one kit table row: the columns the forwarder
// dispatches by and the envelope it delivers.
type systemFactsOutboxRow struct {
	ID          uuid.UUID
	EventType   string
	AggregateID uuid.UUID
	Status      string
	Envelope    systemFactsEnvelope
}

func systemFactsOutboxRows(t *testing.T, db *sql.DB) []systemFactsOutboxRow {
	t.Helper()

	rows, err := db.QueryContext(t.Context(), `SELECT id, event_type, aggregate_id, status::text, payload::text FROM system_facts_outbox ORDER BY created_at, id`)
	require.NoError(t, err)

	defer rows.Close()

	var out []systemFactsOutboxRow

	for rows.Next() {
		var (
			row     systemFactsOutboxRow
			payload string
		)

		require.NoError(t, rows.Scan(&row.ID, &row.EventType, &row.AggregateID, &row.Status, &payload))

		decoder := json.NewDecoder(bytes.NewReader([]byte(payload)))
		decoder.DisallowUnknownFields()
		require.NoError(t, decoder.Decode(&row.Envelope), "the payload column carries exactly the envelope members")

		out = append(out, row)
	}

	require.NoError(t, rows.Err())

	return out
}

func systemFactsOutboxRowFor(t *testing.T, rows []systemFactsOutboxRow, transactionID string) systemFactsOutboxRow {
	t.Helper()

	for _, row := range rows {
		if row.Envelope.Subject.ID == transactionID {
			return row
		}
	}

	require.Failf(t, "outbox row missing", "no outbox row has transaction %s as its subject", transactionID)

	return systemFactsOutboxRow{}
}

func systemFactsTransactionCount(t *testing.T, db *sql.DB, transactionID string) int {
	t.Helper()

	var count int

	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM transaction WHERE id=$1`, transactionID).Scan(&count))

	return count
}

func systemFactsTransactionStatus(t *testing.T, db *sql.DB, transactionID string) string {
	t.Helper()

	var status string

	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT status FROM transaction WHERE id=$1`, transactionID).Scan(&status))

	return status
}

// resetSystemFactsTables empties the business tables and the kit table so
// every subtest counts outbox rows from zero.
func resetSystemFactsTables(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.ExecContext(t.Context(), `TRUNCATE TABLE operation, transaction, system_facts_outbox CASCADE`)
	require.NoError(t, err)
}

// systemFactsInflowRecord is a direct APPROVED transaction with two legs: an
// external DEBIT and an internal CREDIT, which the adapter classifies INFLOW.
func systemFactsInflowRecord(t *testing.T) command.TransactionWriteSet {
	t.Helper()

	record := recoverySQLRecord(t)
	seed := uuid.MustParse(record.Transaction.ID)

	debit := record.Transaction.Operations[0]
	debit.AccountAlias = constant.DefaultExternalAccountAliasPrefix + record.Transaction.AssetCode

	credit := *debit
	credit.ID = uuid.NewSHA1(seed, []byte("inflow:credit")).String()
	credit.Type, credit.Direction = constant.CREDIT, constant.DirectionCredit
	credit.AccountAlias = "@destination"
	credit.AccountID = uuid.NewSHA1(seed, []byte("inflow:credit:account")).String()
	credit.BalanceID = uuid.NewSHA1(seed, []byte("inflow:credit:balance")).String()
	creditAfter := credit.Balance.Available.Add(*credit.Amount.Value)
	credit.BalanceAfter.Available = &creditAfter
	record.Transaction.Operations = append(record.Transaction.Operations, &credit)

	return record
}

// systemFactsHoldRecord is the PENDING hold of the harness fixture.
func systemFactsHoldRecord(t *testing.T) command.TransactionWriteSet {
	t.Helper()

	record := recoverySQLRecord(t)
	record.Action, record.Transaction.Status.Code = constant.ActionHold, constant.PENDING
	record.Transaction.Body = mtransaction.Transaction{Pending: true, Send: mtransaction.Send{Asset: record.Transaction.AssetCode, Value: *record.Transaction.Amount}}

	return record
}

// systemFactsTerminalRecord is the commit or cancel of hold: a CAS from
// PENDING to the terminal status with one new operation row.
func systemFactsTerminalRecord(hold command.TransactionWriteSet, action, terminal string) command.TransactionWriteSet {
	record := hold
	tran := *hold.Transaction
	record.Transaction = &tran
	record.Action, record.ExpectedStatus, record.Transaction.Status.Code = action, constant.PENDING, terminal
	record.Transaction.UpdatedAt = record.Transaction.CreatedAt.Add(time.Second)

	row := *hold.Transaction.Operations[0]
	row.ID = uuid.NewSHA1(uuid.MustParse(hold.Transaction.ID), []byte("terminal:"+action)).String()
	row.CreatedAt, row.UpdatedAt = record.Transaction.UpdatedAt, record.Transaction.UpdatedAt
	record.Transaction.Operations = []*operation.Operation{&row}

	return record
}

// systemFactsRevertRecord is the revert of origin: a new transaction whose
// parent is origin, with every leg re-identified.
func systemFactsRevertRecord(origin command.TransactionWriteSet) command.TransactionWriteSet {
	record := origin
	tran := *origin.Transaction
	record.Transaction = &tran
	record.Action = constant.ActionRevert
	record.Transaction.ID = uuid.NewSHA1(uuid.MustParse(origin.Transaction.ID), []byte("revert")).String()
	record.Transaction.ParentTransactionID = &origin.Transaction.ID
	record.Transaction.CreatedAt = origin.Transaction.CreatedAt.Add(time.Second)
	record.Transaction.UpdatedAt = record.Transaction.CreatedAt

	seed := uuid.MustParse(record.Transaction.ID)
	record.Transaction.Operations = make([]*operation.Operation, 0, len(origin.Transaction.Operations))

	for index, leg := range origin.Transaction.Operations {
		row := *leg
		row.ID = uuid.NewSHA1(seed, []byte(fmt.Sprintf("revert:operation:%d", index))).String()
		row.TransactionID = record.Transaction.ID
		row.CreatedAt, row.UpdatedAt = record.Transaction.CreatedAt, record.Transaction.UpdatedAt
		record.Transaction.Operations = append(record.Transaction.Operations, &row)
	}

	return record
}

// systemFactsTenantPools is a TenantPoolResolver over one database per
// tenant; an unknown tenant fails closed.
type systemFactsTenantPools map[string]*sql.DB

func (p systemFactsTenantPools) PoolForTenant(_ context.Context, tenantID string) (*sql.DB, error) {
	db, ok := p[tenantID]
	if !ok {
		return nil, fmt.Errorf("tenant %q has no pool", tenantID)
	}

	return db, nil
}

func (p systemFactsTenantPools) ListTenants(context.Context) ([]string, error) {
	tenants := make([]string, 0, len(p))
	for tenant := range p {
		tenants = append(tenants, tenant)
	}

	return tenants, nil
}

func TestIntegration_CompletionStore_RecordsTransactionFactInOutbox(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL")
	}

	infra := setupSystemFactsSQL(t)
	producer := newSystemFactsProducer(t, infra, libSystemFacts.TenancyConfig{}, nil)
	store := newSystemFactsStore(t, infra, producer.Emitter())

	dispatchKey, err := kitOutbox.DispatchKey(producer.Routes().Active())
	require.NoError(t, err)

	t.Run("direct inflow commits the row and one envelope together", func(t *testing.T) {
		resetSystemFactsTables(t, infra.db)
		record := systemFactsInflowRecord(t)
		transactionID := record.Transaction.ID

		outcome, err := store.PersistWithOutcome(t.Context(), record)
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseCreated, outcome.LifecyclePhase)
		require.Equal(t, 1, systemFactsTransactionCount(t, infra.db, transactionID))

		// The jsonb layout, read with the operators a forwarder or an operator
		// would use, before the Go decode reads the rest.
		var factType, schemaVersion, factID, direction, action string
		require.NoError(t, infra.db.QueryRowContext(t.Context(),
			`SELECT payload->>'type', payload->>'schemaVersion', payload->>'factId', payload->'data'->>'direction', payload->'data'->>'action' FROM system_facts_outbox`).
			Scan(&factType, &schemaVersion, &factID, &direction, &action))
		assert.Equal(t, systemFactsFactType, factType)
		assert.Equal(t, systemFactsSchemaVersion, schemaVersion)
		assert.Equal(t, systemfacts.TransactionCommittedFactID(uuid.MustParse(transactionID)).String(), factID)
		assert.Equal(t, "INFLOW", direction)
		assert.Equal(t, constant.ActionDirect, action)

		rows := systemFactsOutboxRows(t, infra.db)
		require.Len(t, rows, 1, "exactly one outbox row per committed transaction")

		row := rows[0]
		wantFactID := systemfacts.TransactionCommittedFactID(uuid.MustParse(transactionID))
		assert.Equal(t, wantFactID, row.ID, "the row id is the fact id")
		assert.Equal(t, wantFactID, row.AggregateID, "the aggregate id is the fact id")
		assert.Equal(t, dispatchKey, row.EventType, "the event type is the active route's dispatch key")
		assert.Equal(t, libOutbox.OutboxStatusPending, row.Status)

		env := row.Envelope
		assert.Equal(t, systemFactsEnvelopeVersion, env.EnvelopeVersion)
		assert.Equal(t, wantFactID, env.FactID)
		assert.Equal(t, systemFactsProducer, env.Producer)
		assert.Equal(t, systemFactsNamespace, env.Namespace)
		assert.Equal(t, systemFactsFactType, env.Type)
		assert.Equal(t, systemFactsSchemaVersion, env.SchemaVersion)
		assert.True(t, env.OccurredAt.Equal(record.Transaction.UpdatedAt), "occurredAt is the completion's updated_at")
		assert.Equal(t, systemFactsSubject{Type: systemFactsSubjectType, ID: transactionID}, env.Subject)
		assert.Equal(t, []systemFactsReference{{
			Type:  systemFactsReferenceType,
			Value: transactionID,
			Scope: map[string]string{
				systemFactsScopeOrganization: record.Transaction.OrganizationID,
				systemFactsScopeLedger:       record.Transaction.LedgerID,
			},
		}}, env.References)
		assert.Equal(t, constant.ActionDirect, env.Data.Action)
		assert.Equal(t, constant.APPROVED, env.Data.Status)
		assert.Equal(t, "INFLOW", env.Data.Direction)
		assert.Empty(t, env.Data.ParentTransactionID)
		require.Len(t, env.Data.Operations, 2)
		assert.True(t, env.Data.Operations[0].External, "the external DEBIT leg is marked external")
		assert.False(t, env.Data.Operations[1].External, "the internal CREDIT leg is not")
	})

	t.Run("hold records nothing and the commit records the commit fact", func(t *testing.T) {
		resetSystemFactsTables(t, infra.db)
		hold := systemFactsHoldRecord(t)
		transactionID := hold.Transaction.ID

		holdOutcome, err := store.PersistWithOutcome(t.Context(), hold)
		require.NoError(t, err)
		require.Equal(t, constant.PENDING, holdOutcome.TransactionStatus)
		require.Equal(t, 1, systemFactsTransactionCount(t, infra.db, transactionID))
		assert.Empty(t, systemFactsOutboxRows(t, infra.db), "a hold is not a fact")

		commit := systemFactsTerminalRecord(hold, constant.ActionCommit, constant.APPROVED)

		commitOutcome, err := store.PersistWithOutcome(t.Context(), commit)
		require.NoError(t, err)
		require.Equal(t, constant.APPROVED, commitOutcome.TransactionStatus)
		require.Equal(t, command.TransactionLifecyclePhaseUpdated, commitOutcome.LifecyclePhase)
		assert.Equal(t, constant.APPROVED, systemFactsTransactionStatus(t, infra.db, transactionID))

		rows := systemFactsOutboxRows(t, infra.db)
		require.Len(t, rows, 1, "exactly one fact for the committed hold")
		assert.Equal(t, systemfacts.TransactionCommittedFactID(uuid.MustParse(transactionID)), rows[0].ID)
		assert.Equal(t, transactionID, rows[0].Envelope.Subject.ID)
		assert.Equal(t, constant.ActionCommit, rows[0].Envelope.Data.Action)
		assert.Equal(t, constant.APPROVED, rows[0].Envelope.Data.Status)
		assert.True(t, rows[0].Envelope.OccurredAt.Equal(commit.Transaction.UpdatedAt))
	})

	t.Run("revert records a revert fact naming its parent", func(t *testing.T) {
		resetSystemFactsTables(t, infra.db)
		origin := systemFactsInflowRecord(t)
		revert := systemFactsRevertRecord(origin)

		_, err := store.PersistWithOutcome(t.Context(), origin)
		require.NoError(t, err)

		outcome, err := store.PersistWithOutcome(t.Context(), revert)
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseCreated, outcome.LifecyclePhase)

		rows := systemFactsOutboxRows(t, infra.db)
		require.Len(t, rows, 2, "the origin and the revert are two facts")

		row := systemFactsOutboxRowFor(t, rows, revert.Transaction.ID)
		assert.Equal(t, systemfacts.TransactionCommittedFactID(uuid.MustParse(revert.Transaction.ID)), row.ID)
		assert.Equal(t, constant.ActionRevert, row.Envelope.Data.Action)
		assert.Equal(t, origin.Transaction.ID, row.Envelope.Data.ParentTransactionID)
		assert.Equal(t, origin.Transaction.ID, systemFactsOutboxRowFor(t, rows, origin.Transaction.ID).Envelope.Subject.ID)
	})

	t.Run("cancel records nothing", func(t *testing.T) {
		resetSystemFactsTables(t, infra.db)
		hold := systemFactsHoldRecord(t)
		transactionID := hold.Transaction.ID

		_, err := store.PersistWithOutcome(t.Context(), hold)
		require.NoError(t, err)

		cancel := systemFactsTerminalRecord(hold, constant.ActionCancel, constant.CANCELED)

		outcome, err := store.PersistWithOutcome(t.Context(), cancel)
		require.NoError(t, err)
		require.Equal(t, constant.CANCELED, outcome.TransactionStatus)
		assert.Equal(t, constant.CANCELED, systemFactsTransactionStatus(t, infra.db, transactionID))
		assert.Empty(t, systemFactsOutboxRows(t, infra.db), "a cancel is not a fact")
	})

	t.Run("bulk records one fact per transaction", func(t *testing.T) {
		resetSystemFactsTables(t, infra.db)
		first := recoverySQLRecord(t)
		second := recoverySQLSecondRecord(first)

		outcomes, err := store.PersistBulkWithOutcome(t.Context(), []command.TransactionWriteSet{first, second})
		require.NoError(t, err)
		require.Len(t, outcomes, 2)

		rows := systemFactsOutboxRows(t, infra.db)
		require.Len(t, rows, 2)

		for _, record := range []command.TransactionWriteSet{first, second} {
			require.Equal(t, 1, systemFactsTransactionCount(t, infra.db, record.Transaction.ID))

			row := systemFactsOutboxRowFor(t, rows, record.Transaction.ID)
			assert.Equal(t, systemfacts.TransactionCommittedFactID(uuid.MustParse(record.Transaction.ID)), row.ID)
			assert.Equal(t, constant.ActionDirect, row.Envelope.Data.Action)
		}
	})

	t.Run("replay of a persisted record records no second fact", func(t *testing.T) {
		resetSystemFactsTables(t, infra.db)
		record := systemFactsInflowRecord(t)

		_, err := store.PersistWithOutcome(t.Context(), record)
		require.NoError(t, err)

		outcome, err := store.PersistWithOutcome(t.Context(), record)
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseNoop, outcome.LifecyclePhase)
		require.Len(t, systemFactsOutboxRows(t, infra.db), 1, "a verified replay must not record a second fact")
	})

	t.Run("a failed fact record rolls the transaction row back", func(t *testing.T) {
		resetSystemFactsTables(t, infra.db)
		record := systemFactsInflowRecord(t)
		transactionID := record.Transaction.ID

		_, err := infra.db.ExecContext(t.Context(), `ALTER TABLE system_facts_outbox RENAME TO system_facts_outbox_broken`)
		require.NoError(t, err)

		t.Cleanup(func() {
			_, err := infra.db.ExecContext(context.Background(), `ALTER TABLE IF EXISTS system_facts_outbox_broken RENAME TO system_facts_outbox`)
			require.NoError(t, err)
		})

		_, err = store.PersistWithOutcome(t.Context(), record)
		require.ErrorIs(t, err, kitOutbox.ErrRecordScopeMismatch, "the kit refuses a transaction that cannot reach its table")
		assert.Equal(t, 0, systemFactsTransactionCount(t, infra.db, transactionID), "the business row must not outlive the lost fact")

		_, err = infra.db.ExecContext(t.Context(), `ALTER TABLE system_facts_outbox_broken RENAME TO system_facts_outbox`)
		require.NoError(t, err)

		outcome, err := store.PersistWithOutcome(t.Context(), record)
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseCreated, outcome.LifecyclePhase)
		require.Equal(t, 1, systemFactsTransactionCount(t, infra.db, transactionID))

		rows := systemFactsOutboxRows(t, infra.db)
		require.Len(t, rows, 1)
		assert.Equal(t, transactionID, rows[0].Envelope.Subject.ID)
	})
}

func TestIntegration_CompletionStore_RefusedFactCommitsRowAndCounts(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL")
	}

	infra := setupSystemFactsSQL(t)
	reader, meters := newSystemFactsMeterProvider(t)
	producer := newSystemFactsProducer(t, infra, libSystemFacts.TenancyConfig{}, meters)
	store := newSystemFactsStore(t, infra, producer.Emitter())

	resetSystemFactsTables(t, infra.db)

	// The Store verifies scope, amounts and identities of a leg, never its
	// type; the contract's enum does, so this write passes the Store and is
	// refused by the schema.
	record := recoverySQLRecord(t)
	record.Transaction.Operations[0].Type = "BOGUS"
	transactionID := record.Transaction.ID

	_, counted := systemFactsRefusals(t, reader, emitter.RefusalInvalidFact, systemFactsFactType)
	require.False(t, counted, "no refusal before the write")

	outcome, err := store.PersistWithOutcome(t.Context(), record)
	require.NoError(t, err, "a refused fact never fails the business write")
	require.Equal(t, command.TransactionLifecyclePhaseCreated, outcome.LifecyclePhase)
	assert.Equal(t, 1, systemFactsTransactionCount(t, infra.db, transactionID))
	assert.Empty(t, systemFactsOutboxRows(t, infra.db), "a refused fact is not recorded")

	refused, counted := systemFactsRefusals(t, reader, emitter.RefusalInvalidFact, systemFactsFactType)
	require.True(t, counted, "the refusal is counted on %s", emitter.MetricEmitRefused)
	assert.Equal(t, int64(1), refused)
}

func TestIntegration_CompletionStore_DisabledEmitterRecordsNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL")
	}

	infra := setupSystemFactsSQL(t)

	facts, err := libSystemFacts.New(libSystemFacts.Config{Enabled: false})
	require.NoError(t, err)

	store := newSystemFactsStore(t, infra, facts)
	resetSystemFactsTables(t, infra.db)
	record := systemFactsInflowRecord(t)

	outcome, err := store.PersistWithOutcome(t.Context(), record)
	require.NoError(t, err)
	require.Equal(t, command.TransactionLifecyclePhaseCreated, outcome.LifecyclePhase)
	assert.Equal(t, 1, systemFactsTransactionCount(t, infra.db, record.Transaction.ID))
	assert.Empty(t, systemFactsOutboxRows(t, infra.db), "a disabled emitter records nothing")
}

func TestIntegration_CompletionStore_MultiTenantRequiresTenantInContext(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL")
	}

	infra := setupSystemFactsSQL(t)
	tenant := uuid.NewString()
	producer := newSystemFactsProducer(t, infra, libSystemFacts.TenancyConfig{
		MultiTenantEnabled: true,
		Pools:              systemFactsTenantPools{tenant: infra.db},
	}, nil)
	store := newSystemFactsStore(t, infra, producer.Emitter())

	resetSystemFactsTables(t, infra.db)

	t.Run("records under the tenant-manager tenant", func(t *testing.T) {
		record := systemFactsInflowRecord(t)
		ctx := tmcore.ContextWithTenantID(t.Context(), tenant)

		outcome, err := store.PersistWithOutcome(ctx, record)
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseCreated, outcome.LifecyclePhase)
		assert.Equal(t, 1, systemFactsTransactionCount(t, infra.db, record.Transaction.ID))

		rows := systemFactsOutboxRows(t, infra.db)
		require.Len(t, rows, 1)
		assert.Equal(t, record.Transaction.ID, rows[0].Envelope.Subject.ID)
	})

	t.Run("refuses a write without a tenant and rolls the row back", func(t *testing.T) {
		record := systemFactsInflowRecord(t)

		_, err := store.PersistWithOutcome(t.Context(), record)
		require.ErrorIs(t, err, libOutbox.ErrTenantIDRequired)
		assert.Equal(t, 0, systemFactsTransactionCount(t, infra.db, record.Transaction.ID), "the business row must not commit without its fact")
		require.Len(t, systemFactsOutboxRows(t, infra.db), 1, "only the tenant-scoped write recorded a fact")
	})
}
