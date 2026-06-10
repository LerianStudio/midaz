// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// This file is the worker-level plugin_crm extraction parity test. It is the
// safety net for the Phase 3 engine cutover: plugin_crm extraction does NOT
// route through the generic engine (the engine queries literal collection names,
// while plugin_crm needs the holders_* org fan-out, the hash-based advanced
// filter pre-transform, and field decryption), so the golden-file engine parity
// test in generate-report-extraction_parity_integration_test.go could not cover
// it. The legacy HTTP path that used to exercise this surface is deleted, so this
// is the regression guard locking current plugin_crm behavior before the cutover
// commits.
//
// It drives the REAL composed handler method UseCase.extractPluginCRM over a real
// mongodb.ExternalDataSource backed by testcontainers MongoDB and a real
// CircuitBreakerManager — the exact production wiring the worker bootstrap builds
// for the plugin_crm datasource. Fixtures are encrypted/hashed at seed time with
// the SAME lib-commons crypto primitives the plugincrm module decrypts/hashes
// with (libCrypto.Crypto.Encrypt / GenerateHash under the configured keys), so
// the module round-trips them; no cipher is reinvented.
//
// The legacy naming "plugin_crm" is preserved deliberately: this test is the
// regression guard for the upcoming repo-wide rename to "crm", so it must lock
// current behavior faithfully.
//
// It proves, end-to-end against a live Mongo:
//
//  1. ORG FAN-OUT: extraction reads BOTH holders_<orgA> and holders_<orgB>
//     physical collections (prefix match), merges them, and injects the correct
//     organization_id per row.
//  2. DECRYPTION: decrypted output equals the known plaintext fixtures across the
//     full nested field tree the module handles (top-level document/name +
//     contact/banking_details/legal_person/natural_person/regulatory_fields/
//     related_parties).
//  3. HASH-FILTER: a filter on the logical "document" field is transformed to the
//     hashed "search.document" value and selects exactly the matching row(s) — a
//     filtered extraction returns the right subset, not the full set.
//  4. FAIL-CLOSED: a missing crypto key yields a loud precondition error (no
//     silent plaintext, no panic).
//  5. NO LEAKAGE: the path runs under a recording logger and the test asserts no
//     decrypted PII / secret / hash value appears in any emitted log line.
package services

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/trace/noop"

	pkgErr "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit"
	mongokit "github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	mongoadapter "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
)

// ----------------------------------------------------------------------------
// Fixed fixtures (no time.Now). plugin_crm crypto keys: the hash key is an
// arbitrary HMAC secret; the encrypt key is a hex-encoded 32-byte (AES-256) key,
// matching the shape libCrypto.Crypto.InitializeCipher requires.
// ----------------------------------------------------------------------------

const (
	crmParityHashKey = "plugin-crm-parity-hash-secret-key"
	// crmParityEncryptKey is a hex-encoded 32-byte (AES-256) key.
	crmParityEncryptKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

	crmParityDatasource = "plugin_crm"

	crmParityOrgA = "orgA"
	crmParityOrgB = "orgB"

	// crmParityDB is the logical mongo database the per-tenant plugin_crm
	// repository points at.
	crmParityDB = "plugin_crm"

	// crmParityCollection is the logical collection the report references; the
	// fan-out discovers its physical org-scoped variants holders_<org>.
	crmParityCollection = "holders"
)

// crmPlaintext is the known plaintext for one holder document. The seed encrypts
// the encrypted fields with the configured key; the module decrypts them back to
// exactly these values. The "document" field also drives the hash filter: the
// seed stores its HMAC under search.document, and the filter selects on it.
type crmPlaintext struct {
	document      string
	name          string
	status        string
	primaryEmail  string
	mobilePhone   string
	bankAccount   string
	iban          string
	repName       string
	repDocument   string
	repEmail      string
	motherName    string
	fatherName    string
	participantID string
	relatedDocs   []string
}

// crmOrgAHolder is the fixture seeded into holders_orgA; it is the row the
// document-equals filter must select.
var crmOrgAHolder = crmPlaintext{
	document:      "12345678900",
	name:          "Jane Doe",
	status:        "ACTIVE",
	primaryEmail:  "jane@example.com",
	mobilePhone:   "+5511999990000",
	bankAccount:   "00012345",
	iban:          "BR1500000000000010932840814P2",
	repName:       "Rep Name",
	repDocument:   "99988877766",
	repEmail:      "rep@example.com",
	motherName:    "Mother Name",
	fatherName:    "Father Name",
	participantID: "55544433322",
	relatedDocs:   []string{"11122233344", "55566677788"},
}

// crmOrgBHolder is the fixture seeded into holders_orgB; it must be excluded by
// the document-equals filter (different document) but included in the unfiltered
// fan-out.
var crmOrgBHolder = crmPlaintext{
	document:      "00000000000",
	name:          "John Roe",
	status:        "ACTIVE",
	primaryEmail:  "john@example.com",
	mobilePhone:   "+5521888880000",
	bankAccount:   "00098765",
	iban:          "BR9700000000000010932840814P3",
	repName:       "Rep Two",
	repDocument:   "11100099988",
	repEmail:      "rep2@example.com",
	motherName:    "Mother Two",
	fatherName:    "Father Two",
	participantID: "77766655544",
	relatedDocs:   []string{"22233344455"},
}

// crmRequestedFields is the report field selection. It covers a top-level
// encrypted field (document, name), every nested encrypted object, and the
// related_parties array, so DecryptRecords walks the full tree.
//
// organization_id is intentionally NOT requested: it is not a stored field, it is
// synthesized by the fan-out per source collection AFTER the query. Requesting it
// as a projection would (correctly) fail the adapter's field validation — exactly
// as it would in production.
var crmRequestedFields = []string{
	"document", "name", "status",
	"contact", "banking_details", "legal_person",
	"natural_person", "regulatory_fields", "related_parties",
}

// ----------------------------------------------------------------------------
// Recording logger — proves NO PII / secret / hash leaks into logs.
// ----------------------------------------------------------------------------

// crmRecordingLogger captures every emitted log line so the test can assert no
// decrypted value, secret, or hash appears. It satisfies the log.Logger interface
// (Log/With/WithGroup/Enabled/Sync) by embedding a nop logger; it overrides Log to
// record the message AND every structured field's rendered key/value, and
// With/WithGroup to return itself so chained loggers keep recording.
type crmRecordingLogger struct {
	log.Logger
	mu    sync.Mutex
	lines []string
}

func newCRMRecordingLogger() *crmRecordingLogger {
	return &crmRecordingLogger{Logger: log.NewNop()}
}

func (l *crmRecordingLogger) Log(_ context.Context, _ log.Level, msg string, fields ...log.Field) {
	line := msg
	for _, f := range fields {
		// Render the field's key and string-ish value. The module is contractually
		// forbidden from putting PII/secrets/hashes in a field at all; rendering the
		// value here is what lets the test PROVE that — a leak would surface in the
		// recorded line and trip assertNoPIILeak.
		line += " " + f.Key + "=" + renderFieldValue(f)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.lines = append(l.lines, line)
}

func (l *crmRecordingLogger) With(_ ...log.Field) log.Logger { return l }
func (l *crmRecordingLogger) WithGroup(_ string) log.Logger  { return l }

// renderFieldValue extracts a string-comparable rendering of a log field's value
// from the public Field.Value, so the leak scan can match against fixture
// plaintext, secrets, and hashes.
func renderFieldValue(f log.Field) string {
	switch t := f.Value.(type) {
	case string:
		return t
	case error:
		if t != nil {
			return t.Error()
		}

		return ""
	case []string:
		out := ""
		for _, s := range t {
			out += s + ","
		}

		return out
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprintf("%v", f.Value)
	}
}

func (l *crmRecordingLogger) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]string, len(l.lines))
	copy(out, l.lines)

	return out
}

// ----------------------------------------------------------------------------
// Infra + seeding.
// ----------------------------------------------------------------------------

// crmStartMongo starts a testcontainers MongoDB, connects a host-side driver
// client for seeding, and returns it with a teardown func.
func crmStartMongo(t *testing.T) (*mongo.Client, string, func()) {
	t.Helper()

	ctx := context.Background()

	infra := mongokit.NewMongoDBInfra(mongokit.MongoDBConfig{Name: "crmparity"})

	suite, err := itestkit.New(t).WithInfra(infra).Build(ctx)
	require.NoError(t, err)

	uri, err := infra.URI()
	require.NoError(t, err)

	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)
	require.NoError(t, client.Ping(connectCtx, nil))

	return client, uri, func() {
		_ = client.Disconnect(context.Background())
		_ = suite.Terminate(context.Background())
	}
}

// crmNewCrypto builds an initialized cipher over the parity keys, used by the
// seed to encrypt fixture fields and hash the filterable document. It uses the
// SAME libCrypto.Crypto the plugincrm module instantiates internally, so the
// seed's ciphertext/hash are exactly what the module's DecryptRecords /
// TransformFilters expect.
func crmNewCrypto(t *testing.T) *libCrypto.Crypto {
	t.Helper()

	crypto := &libCrypto.Crypto{
		HashSecretKey:    crmParityHashKey,
		EncryptSecretKey: crmParityEncryptKey,
		Logger:           log.NewNop(),
	}
	require.NoError(t, crypto.InitializeCipher())

	return crypto
}

// crmEncrypt encrypts a plaintext with the parity cipher, returning the base64
// ciphertext the module decrypts.
func crmEncrypt(t *testing.T, crypto *libCrypto.Crypto, plain string) string {
	t.Helper()

	enc, err := crypto.Encrypt(&plain)
	require.NoError(t, err)

	return *enc
}

// crmEncryptedDoc builds the bson document the plugin stores for one holder: every
// sensitive field encrypted with the parity cipher, the searchable document
// HMAC-hashed under search.document (the wire contract the hash filter targets).
func crmEncryptedDoc(t *testing.T, crypto *libCrypto.Crypto, p crmPlaintext) bson.M {
	t.Helper()

	relatedParties := bson.A{}
	for _, d := range p.relatedDocs {
		relatedParties = append(relatedParties, bson.M{"document": crmEncrypt(t, crypto, d)})
	}

	return bson.M{
		"document": crmEncrypt(t, crypto, p.document),
		"name":     crmEncrypt(t, crypto, p.name),
		"status":   p.status, // not encrypted; passes through
		"contact": bson.M{
			"primary_email": crmEncrypt(t, crypto, p.primaryEmail),
			"mobile_phone":  crmEncrypt(t, crypto, p.mobilePhone),
		},
		"banking_details": bson.M{
			"account": crmEncrypt(t, crypto, p.bankAccount),
			"iban":    crmEncrypt(t, crypto, p.iban),
		},
		"legal_person": bson.M{
			"representative": bson.M{
				"name":     crmEncrypt(t, crypto, p.repName),
				"document": crmEncrypt(t, crypto, p.repDocument),
				"email":    crmEncrypt(t, crypto, p.repEmail),
			},
		},
		"natural_person": bson.M{
			"mother_name": crmEncrypt(t, crypto, p.motherName),
			"father_name": crmEncrypt(t, crypto, p.fatherName),
		},
		"regulatory_fields": bson.M{
			"participant_document": crmEncrypt(t, crypto, p.participantID),
		},
		"related_parties": relatedParties,
		// search.document carries the HMAC of the plaintext document, the exact
		// value TransformFilters("document" -> "search.document") hashes a filter
		// to. The plugin stores it as a nested object so the field projection root
		// "search" resolves.
		"search": bson.M{
			"document": crypto.GenerateHash(strPtr(p.document)),
		},
	}
}

func strPtr(s string) *string { return &s }

// crmSeed inserts the two org holder fixtures into their physical collections
// holders_orgA / holders_orgB, plus an unrelated accounts_orgA collection that
// the fan-out prefix match must ignore.
func crmSeed(t *testing.T, client *mongo.Client, crypto *libCrypto.Crypto) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db := client.Database(crmParityDB)

	_, err := db.Collection("holders_"+crmParityOrgA).InsertOne(ctx, crmEncryptedDoc(t, crypto, crmOrgAHolder))
	require.NoError(t, err)

	_, err = db.Collection("holders_"+crmParityOrgB).InsertOne(ctx, crmEncryptedDoc(t, crypto, crmOrgBHolder))
	require.NoError(t, err)

	// Unrelated collection: must NOT match the holders_ prefix.
	_, err = db.Collection("accounts_"+crmParityOrgA).InsertOne(ctx, bson.M{"unrelated": "ignore-me"})
	require.NoError(t, err)
}

// crmUseCase wires the real production plugin_crm dependencies: a real
// mongodb.ExternalDataSource over the testcontainer, a real CircuitBreakerManager,
// and the parity crypto keys. encryptKey/hashKey are parameters so the
// fail-closed test can null one out.
func crmUseCase(t *testing.T, mongoURI string, logger log.Logger, hashKey, encryptKey string) *UseCase {
	t.Helper()

	// The testcontainer speaks plaintext mongo; lib-commons enforces TLS by
	// default and refuses to connect over plaintext without this bypass. It is a
	// test-infra-only knob — production datasources connect over TLS.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	repo, err := mongoadapter.NewDataSourceRepository(mongoURI, crmParityDB, logger)
	require.NoError(t, err)

	t.Cleanup(func() { _ = repo.CloseConnection(context.Background()) })

	return &UseCase{
		Logger: logger,
		Tracer: noop.NewTracerProvider().Tracer("crm-parity"),
		ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
			crmParityDatasource: {
				DatabaseType:      "mongodb",
				MongoDBRepository: repo,
			},
		}),
		CircuitBreakerManager:           pkg.NewCircuitBreakerManager(logger),
		CryptoHashSecretKeyPluginCRM:    hashKey,
		CryptoEncryptSecretKeyPluginCRM: encryptKey,
	}
}

// crmCollections builds the per-collection field selection the handler consumes.
func crmCollections() map[string][]string {
	return map[string][]string{crmParityCollection: crmRequestedFields}
}

// findRowByDocument returns the merged row whose decrypted document matches; the
// merge order is deterministic (sorted physical collection name) but the test
// keys on org/document rather than slice index to stay robust.
func findRowByDocument(rows []map[string]any, document string) (map[string]any, bool) {
	for _, r := range rows {
		if r["document"] == document {
			return r, true
		}
	}

	return nil, false
}

// ----------------------------------------------------------------------------
// Tests.
// ----------------------------------------------------------------------------

// TestIntegration_PluginCRMParity_FanOutDecryptUnfiltered proves org fan-out +
// organization_id injection + full-tree decryption with no filter: BOTH org
// collections are read and every encrypted field round-trips to plaintext.
func TestIntegration_PluginCRMParity_FanOutDecryptUnfiltered(t *testing.T) {
	client, uri, teardown := crmStartMongo(t)
	defer teardown()

	crypto := crmNewCrypto(t)
	crmSeed(t, client, crypto)

	logger := newCRMRecordingLogger()
	uc := crmUseCase(t, uri, logger, crmParityHashKey, crmParityEncryptKey)

	result := make(map[string]map[string][]map[string]any)

	err := uc.extractPluginCRM(context.Background(), crmParityDatasource, crmCollections(), nil, result)
	require.NoError(t, err)

	rows := result[crmParityDatasource][crmParityCollection]
	require.Len(t, rows, 2, "fan-out must read BOTH holders_orgA and holders_orgB and merge them")

	// ORG FAN-OUT: organization_id is injected per source collection.
	rowA, okA := findRowByDocument(rows, crmOrgAHolder.document)
	require.True(t, okA, "orgA holder must be present after decryption")
	assert.Equal(t, crmParityOrgA, rowA["organization_id"], "orgA row must carry orgA's id")

	rowB, okB := findRowByDocument(rows, crmOrgBHolder.document)
	require.True(t, okB, "orgB holder must be present after decryption")
	assert.Equal(t, crmParityOrgB, rowB["organization_id"], "orgB row must carry orgB's id")

	// DECRYPTION: the full nested field tree round-trips for orgA.
	assertHolderDecrypted(t, rowA, crmOrgAHolder)
	assertHolderDecrypted(t, rowB, crmOrgBHolder)

	// NO LEAKAGE: no decrypted value / secret / hash in any recorded log line.
	assertNoPIILeak(t, logger.snapshot())
}

// TestIntegration_PluginCRMParity_HashFilterSelectsSubset proves the hash-based
// advanced-filter transform: a filter on the logical "document" field is rewritten
// to the hashed search.document value and selects exactly the orgA row, NOT the
// full two-row set.
func TestIntegration_PluginCRMParity_HashFilterSelectsSubset(t *testing.T) {
	client, uri, teardown := crmStartMongo(t)
	defer teardown()

	crypto := crmNewCrypto(t)
	crmSeed(t, client, crypto)

	logger := newCRMRecordingLogger()
	uc := crmUseCase(t, uri, logger, crmParityHashKey, crmParityEncryptKey)

	// Logical filter on the plaintext document of the orgA holder. The handler
	// runs TransformFilters, which maps document -> search.document and HMAC-hashes
	// the plaintext; the seed stored that exact hash under search.document.
	filters := map[string]map[string]map[string]model.FilterCondition{
		crmParityDatasource: {
			crmParityCollection: {
				"document": {Equals: []any{crmOrgAHolder.document}},
			},
		},
	}

	result := make(map[string]map[string][]map[string]any)

	err := uc.extractPluginCRM(context.Background(), crmParityDatasource, crmCollections(), filters, result)
	require.NoError(t, err)

	rows := result[crmParityDatasource][crmParityCollection]
	require.Len(t, rows, 1, "hash filter must select exactly the matching subset, not the full set")

	got := rows[0]
	assert.Equal(t, crmOrgAHolder.document, got["document"], "the selected row must be the orgA holder")
	assert.Equal(t, crmParityOrgA, got["organization_id"])

	// The selected row is still fully decrypted.
	assertHolderDecrypted(t, got, crmOrgAHolder)

	assertNoPIILeak(t, logger.snapshot())
}

// TestIntegration_PluginCRMParity_FailsClosedOnMissingEncryptKey proves the path
// fails LOUD when a crypto key is absent: a precondition error, never silent
// plaintext or a panic.
func TestIntegration_PluginCRMParity_FailsClosedOnMissingEncryptKey(t *testing.T) {
	client, uri, teardown := crmStartMongo(t)
	defer teardown()

	crypto := crmNewCrypto(t)
	crmSeed(t, client, crypto)

	logger := newCRMRecordingLogger()
	// Empty encrypt key: decryption must fail closed.
	uc := crmUseCase(t, uri, logger, crmParityHashKey, "")

	result := make(map[string]map[string][]map[string]any)

	err := uc.extractPluginCRM(context.Background(), crmParityDatasource, crmCollections(), nil, result)
	require.Error(t, err, "a missing encrypt key must fail closed, never return plaintext")

	// The handler wraps decryption precondition failures as a business decryption
	// error (ValidateBusinessError(ErrDecryptionData)); the underlying cause is the
	// encrypt-key-not-configured precondition. Assert the loud, classified failure.
	assertCRMFailClosed(t, err)

	// No decrypted/plaintext rows were written into the result map.
	rows := result[crmParityDatasource][crmParityCollection]
	assert.Empty(t, rows, "no rows must be emitted when the path fails closed")

	assertNoPIILeak(t, logger.snapshot())
}

// assertCRMFailClosed asserts the error is the loud fail-closed classification
// the handler produces. extractPluginCRMCollection wraps a decryption-stage
// precondition failure with ValidateBusinessError(ErrDecryptionData, ...), which
// yields a typed InternalServerError carrying code "0264"; the underlying
// encrypt-key-not-configured precondition message is interpolated into it. The
// assertion locks both: the classified decryption error code, and that the
// underlying cause is the missing encrypt key (no silent plaintext).
func assertCRMFailClosed(t *testing.T, err error) {
	t.Helper()

	var ise pkgErr.InternalServerError
	require.ErrorAs(t, err, &ise, "fail-closed error must be the typed business decryption error")

	assert.Equal(t, cnErr.ErrDecryptionData.Error(), ise.Code,
		"fail-closed error must classify as the decryption error family (0264)")

	assert.ErrorContains(t, err, "CRYPTO_ENCRYPT_SECRET_KEY_PLUGIN_CRM not configured",
		"fail-closed cause must be the encrypt-key-not-configured precondition")
}

// assertHolderDecrypted asserts every encrypted field on a merged row decrypted
// back to its known plaintext, across the full nested tree the module handles.
func assertHolderDecrypted(t *testing.T, row map[string]any, p crmPlaintext) {
	t.Helper()

	assert.Equal(t, p.document, row["document"])
	assert.Equal(t, p.name, row["name"])
	assert.Equal(t, p.status, row["status"], "non-encrypted field passes through")

	contact, ok := row["contact"].(map[string]any)
	require.True(t, ok, "contact object must be present")
	assert.Equal(t, p.primaryEmail, contact["primary_email"])
	assert.Equal(t, p.mobilePhone, contact["mobile_phone"])

	banking, ok := row["banking_details"].(map[string]any)
	require.True(t, ok, "banking_details object must be present")
	assert.Equal(t, p.bankAccount, banking["account"])
	assert.Equal(t, p.iban, banking["iban"])

	legalPerson, ok := row["legal_person"].(map[string]any)
	require.True(t, ok, "legal_person object must be present")
	rep, ok := legalPerson["representative"].(map[string]any)
	require.True(t, ok, "legal_person.representative object must be present")
	assert.Equal(t, p.repName, rep["name"])
	assert.Equal(t, p.repDocument, rep["document"])
	assert.Equal(t, p.repEmail, rep["email"])

	natural, ok := row["natural_person"].(map[string]any)
	require.True(t, ok, "natural_person object must be present")
	assert.Equal(t, p.motherName, natural["mother_name"])
	assert.Equal(t, p.fatherName, natural["father_name"])

	reg, ok := row["regulatory_fields"].(map[string]any)
	require.True(t, ok, "regulatory_fields object must be present")
	assert.Equal(t, p.participantID, reg["participant_document"])

	parties, ok := row["related_parties"].([]any)
	require.True(t, ok, "related_parties array must be present")
	require.Len(t, parties, len(p.relatedDocs))

	for i, want := range p.relatedDocs {
		party, ok := parties[i].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, want, party["document"], "related_parties[%d].document must decrypt", i)
	}
}

// assertNoPIILeak scans every recorded log message for any decrypted plaintext,
// secret key, or stored hash and fails if any appears. This is the NO LEAKAGE
// guarantee at the worker boundary.
func assertNoPIILeak(t *testing.T, lines []string) {
	t.Helper()

	crypto := crmNewCrypto(t)

	forbidden := []string{
		crmParityHashKey,
		crmParityEncryptKey,
		crypto.GenerateHash(strPtr(crmOrgAHolder.document)),
		crypto.GenerateHash(strPtr(crmOrgBHolder.document)),
	}

	for _, p := range []crmPlaintext{crmOrgAHolder, crmOrgBHolder} {
		forbidden = append(forbidden,
			p.document, p.name, p.primaryEmail, p.mobilePhone,
			p.bankAccount, p.iban, p.repName, p.repDocument, p.repEmail,
			p.motherName, p.fatherName, p.participantID,
		)
		forbidden = append(forbidden, p.relatedDocs...)
	}

	for _, line := range lines {
		for _, secret := range forbidden {
			if secret == "" {
				continue
			}

			assert.NotContains(t, line, secret, "log line leaked a sensitive value")
		}
	}
}
