// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// Epic 3.1 — streaming events. These tests consume the live Kafka-compatible
// broker that the ledger emits CloudEvents to and assert the on-wire contract
// (topic, ce-* headers, payload key set) for account.created,
// transaction.posted, and holder.created (a PII-redacted CRM event), plus one
// negative contract: an IMPORTANT-posture emit failure never fails the HTTP
// request.
//
// The suite self-gates: requireStack skips when the ledger is down,
// strmRequireBroker skips when no broker is reachable at STREAMING_BROKERS.
// On the default stack (STREAMING_ENABLED=false, no broker) every test skips
// cleanly with zero failures.

// strmBrokersEnv is read once; default mirrors the documented local Redpanda
// host port (CLAUDE.md "Streaming / Local testing": bind 19092).
const strmDefaultBroker = "localhost:19092"

// strmServiceName is the ledger-core producing service segment; topics are
// rendered via pkgStreaming.TopicName and take the shape
// "lerian.streaming.<service>_<resource>.<event>". Fees route under "fee" and
// CRM under "crm" (see bootstrap/streaming.go per-product registry); those are
// passed explicitly at their call sites, not via this const.
const strmServiceName = "ledger"

// strmCEType is the reverse-DNS namespace prepended to every ce-type header by
// lib-streaming (internal/cloudevents/cloudevents.go cloudEventsTypePrefix):
// the wire ce-type is "studio.lerian.<resource>.<event>".
const strmCETypePrefix = "studio.lerian."

// CloudEvents 1.0 binary-mode Kafka header keys. CONFIRMED hyphenated against
// lib-streaming@v1.5.1 internal/cloudevents/cloudevents.go (headerCEType etc.):
// the Kafka protocol binding uses "ce-" + attribute, NOT the "ce_" underscore
// form. The record Value is the JSON payload.
const (
	strmHeaderCEType        = "ce-type"
	strmHeaderCESubject     = "ce-subject"
	strmHeaderCEID          = "ce-id"
	strmHeaderCESource      = "ce-source"
	strmHeaderCESpecVersion = "ce-specversion"
)

// strmAccountCreatedKeys is the exact 17-key top-level set of the
// account.created wire payload, copied from the JSONShape lock in
// pkg/streaming/events/account_created_test.go. Asserted as an exact set
// (fail-closed): an extra or missing key here means wire drift.
var strmAccountCreatedKeys = map[string]struct{}{
	"id": {}, "organizationId": {}, "ledgerId": {}, "name": {}, "assetCode": {},
	"type": {}, "portfolioId": {}, "segmentId": {}, "parentAccountId": {},
	"entityId": {}, "holderId": {}, "alias": {}, "status": {}, "blocked": {},
	"holderCheckSkipped": {}, "createdAt": {}, "updatedAt": {},
}

// strmTransactionPostedKeys is the FULL superset of the transaction.posted
// wire payload from pkg/streaming/events/transaction_lifecycle.go. The minimal
// posted payload omits parentTransactionId/routeId/metadata (all omitempty);
// scale is intentionally never emitted. We assert (a) every key actually on
// the wire belongs to this superset (fail-closed against additive drift) and
// (b) the always-present core keys are present — rather than pinning an exact
// count, because a live transfer's optional-field presence is path-dependent.
var strmTransactionPostedKeys = map[string]struct{}{
	"id": {}, "parentTransactionId": {}, "organizationId": {}, "ledgerId": {},
	"status": {}, "amount": {}, "assetCode": {}, "chartOfAccountsGroupName": {},
	"description": {}, "source": {}, "destination": {}, "route": {}, "routeId": {},
	"operations": {}, "metadata": {}, "feesSkipped": {}, "tracerSkipped": {},
	"createdAt": {}, "updatedAt": {},
}

// strmTransactionPostedCore is the subset that is always populated for a
// freshly-posted transaction created via the inflow/JSON paths.
var strmTransactionPostedCore = []string{
	"id", "organizationId", "ledgerId", "status", "operations", "createdAt", "updatedAt",
}

// strmHolderCreatedKeys is the exact 6-key top-level set of the holder.created
// wire payload, copied from the JSONShape lock in
// pkg/streaming/events/holder_created_test.go. Holder is a regulated entity;
// only stable identifiers, the org scope, the person-type classification, the
// nullable client externalId, and timestamps cross the wire. Asserted as an
// exact set (fail-closed): an extra or missing key means wire drift.
var strmHolderCreatedKeys = map[string]struct{}{
	"id": {}, "organizationId": {}, "type": {}, "externalId": {},
	"createdAt": {}, "updatedAt": {},
}

// strmHolderCreatedForbidden is the PII key set that MUST NEVER appear on the
// holder.created wire payload — mirrors the forbidden set in the JSONShape unit
// test. The constructor (pkg/streaming/events/holder_created.go) redacts these
// at the source; this asserts the redaction holds end-to-end on the broker.
var strmHolderCreatedForbidden = []string{
	"name", "document", "cpf", "cnpj",
	"contact", "addresses", "address",
	"naturalPerson", "legalPerson", "representative",
	"metadata", "deletedAt",
}

// strmBrokers returns the broker address list from STREAMING_BROKERS (comma
// separated), defaulting to the local Redpanda host port.
func strmBrokers() []string {
	raw := os.Getenv("STREAMING_BROKERS")
	if raw == "" {
		return []string{strmDefaultBroker}
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}

	if len(out) == 0 {
		return []string{strmDefaultBroker}
	}

	return out
}

// strmBrokerOnce gates the streaming tests on the broker being TCP-reachable.
// A down broker skips (e2e is opt-in and needs Redpanda + STREAMING_ENABLED).
var (
	strmBrokerOnce sync.Once
	strmBrokerUp   bool
)

// strmRequireBroker skips the calling test when the first STREAMING_BROKERS
// address cannot be TCP-dialed. Mirrors the requireStack/requireTracer probe
// shape: a sync.Once dial + t.Skipf with actionable setup instructions.
func strmRequireBroker(t *testing.T) {
	t.Helper()

	brokers := strmBrokers()

	strmBrokerOnce.Do(func() {
		conn, err := net.DialTimeout("tcp", brokers[0], 3*time.Second)
		if err != nil {
			return
		}

		_ = conn.Close()
		strmBrokerUp = true

		// Pre-provision the event catalog before any test triggers a create.
		// lib-streaming's producer does NOT request auto-topic-creation (no
		// kgo.AllowAutoTopicCreation in producer_kgo.go), so a missing topic both
		// fails the emit AND trips lib-streaming's circuit breaker, poisoning
		// every later emit. Creating the topics here keeps the breaker closed.
		strmEnsureTopics(t, brokers)
	})

	if !strmBrokerUp {
		t.Skipf("streaming broker not reachable at %s — start Redpanda bound to host 19092 on infra-network "+
			"and set STREAMING_ENABLED=true + STREAMING_BROKERS (topics are auto-provisioned by this test)",
			brokers[0])
	}
}

// strmEnsureTopics idempotently creates every event-catalog topic on the broker
// via a CreateTopics admin request, so the ledger's producer — which does not
// auto-create topics — always has a destination. Single partition / single
// replica (dev broker); TOPIC_ALREADY_EXISTS (36) is ignored. Best-effort: a
// transport error is logged and a genuinely absent topic surfaces later as a
// consume miss in the test itself.
func strmEnsureTopics(t *testing.T, brokers []string) {
	t.Helper()

	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.ClientID("e2e-strm-admin"))
	if err != nil {
		t.Logf("streaming: admin client for topic provisioning failed: %v", err)
		return
	}
	defer cl.Close()

	req := kmsg.NewPtrCreateTopicsRequest()

	for _, name := range strmCatalogTopics() {
		rt := kmsg.NewCreateTopicsRequestTopic()
		rt.Topic = name
		rt.NumPartitions = 1
		rt.ReplicationFactor = 1
		req.Topics = append(req.Topics, rt)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := req.RequestWith(ctx, cl)
	if err != nil {
		t.Logf("streaming: CreateTopics request failed: %v", err)
		return
	}

	for _, ct := range resp.Topics {
		if ct.ErrorCode != 0 && ct.ErrorCode != 36 { // 36 = TOPIC_ALREADY_EXISTS
			msg := ""
			if ct.ErrorMessage != nil {
				msg = *ct.ErrorMessage
			}

			t.Logf("streaming: create topic %s: code=%d %s", ct.Topic, ct.ErrorCode, msg)
		}
	}
}

// strmCatalogTopics builds the lerian.streaming.* names for the full event
// catalog (mirrors pkg/streaming/events). Every topic the ledger may emit to
// during a test's fixtures is created so no missing-topic emit trips the
// producer circuit breaker.
func strmCatalogTopics() []string {
	families := map[string][]string{
		"organization":      {"created", "updated", "deleted"},
		"ledger":            {"created", "updated", "deleted"},
		"account":           {"created", "updated", "deleted"},
		"asset":             {"created", "updated", "deleted"},
		"portfolio":         {"created", "updated", "deleted"},
		"segment":           {"created", "updated", "deleted"},
		"operation_route":   {"created", "updated", "deleted"},
		"transaction_route": {"created", "updated", "deleted"},
		"balance":           {"created", "config_changed", "deleted", "overdraft-drawn", "overdraft-repaid", "overdraft-cleared"},
		"transaction":       {"posted", "committed", "canceled", "reverted"},
	}

	var topics []string

	for resource, events := range families {
		for _, e := range events {
			topics = append(topics, pkgStreaming.TopicName(strmServiceName, resource+"."+e))
		}
	}

	// CRM resources (holder/instrument) are folded into the ledger binary but
	// emit under the "crm" service segment, so their topics carry the "crm_"
	// prefix. Provision them too so a holder create's IMPORTANT-posture emit has
	// a live destination and never trips the producer circuit breaker.
	for _, e := range []string{"created", "updated", "deleted"} {
		topics = append(topics, pkgStreaming.TopicName("crm", "holder."+e))
	}

	return topics
}

// strmConsumeMatch consumes topic from the beginning with a short poll loop
// and returns the first record whose ce-subject header equals wantSubject. It
// returns the record's ce-type, ce-subject, decoded JSON payload, and whether
// a match was found within timeout. A unique consumer group is used per call
// (group offset reset to earliest) so repeated runs replay from the start
// rather than resuming a committed offset.
func strmConsumeMatch(t *testing.T, topic, wantSubject string, timeout time.Duration) (ceType, subject string, payload map[string]any, found bool) {
	t.Helper()

	cl, err := kgo.NewClient(
		kgo.SeedBrokers(strmBrokers()...),
		kgo.ConsumeTopics(topic),
		// Replay the whole topic every run: the contract assertion needs the
		// record produced by THIS test's create call, which may already be in
		// the log before the consumer starts.
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		// Unique client/group so a prior run's committed offset never hides
		// the record we are looking for.
		kgo.ClientID("e2e-strm-"+uuid.NewString()[:8]),
	)
	if err != nil {
		t.Fatalf("kgo client for %s: %v", topic, err)
	}
	defer cl.Close()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		fetches := cl.PollFetches(ctx)
		cancel()

		if errs := fetches.Errors(); len(errs) > 0 {
			// Topic-not-yet-created and transient fetch errors are expected
			// while the broker catches up; keep polling until the deadline.
			continue
		}

		iter := fetches.RecordIter()
		for !iter.Done() {
			rec := iter.Next()

			if subj, ok := strmHeader(rec, strmHeaderCESubject); ok && subj == wantSubject {
				ct, _ := strmHeader(rec, strmHeaderCEType)

				var decoded map[string]any
				_ = json.Unmarshal(rec.Value, &decoded)

				return ct, subj, decoded, true
			}
		}
	}

	return "", "", nil, false
}

// strmHeader returns the (last-wins) value of a Kafka record header by key.
func strmHeader(rec *kgo.Record, key string) (string, bool) {
	var (
		val   string
		found bool
	)

	for _, h := range rec.Headers {
		if h.Key == key {
			val = string(h.Value)
			found = true
		}
	}

	return val, found
}

// strmAssertKeySet fails when actual contains any key outside allowed
// (fail-closed against additive wire drift).
func strmAssertKeySet(t *testing.T, label string, actual map[string]any, allowed map[string]struct{}) {
	t.Helper()

	for k := range actual {
		if _, ok := allowed[k]; !ok {
			t.Errorf("%s: unexpected top-level wire key %q (drift?)", label, k)
		}
	}
}

// TestStreamingAccountCreatedEmitted asserts an account create produces a
// CloudEvents record on lerian.streaming.account.created whose ce-subject is
// the account id, ce-type is studio.lerian.account.created, and whose payload
// top-level key set EXACTLY matches the 17-key account.created contract.
func TestStreamingAccountCreatedEmitted(t *testing.T) {
	requireStack(t)
	strmRequireBroker(t)

	f := newFixture(t, false)

	alias := "@strm-acc-" + uuid.NewString()[:8]
	acc := mustCreate(t, f.ledgers()+"/accounts", map[string]any{
		"name": "Strm Acct", "assetCode": "USD", "type": "deposit", "alias": alias,
	})

	accID := str(t, acc, "id")

	topic := pkgStreaming.TopicName(strmServiceName, "account.created")

	ceType, subject, payload, ok := strmConsumeMatch(t, topic, accID, 15*time.Second)
	if !ok {
		t.Fatalf("no account.created record with ce-subject=%s on %s within timeout", accID, topic)
	}

	if subject != accID {
		t.Errorf("ce-subject = %q, want account id %q", subject, accID)
	}

	if want := strmCETypePrefix + "account.created"; ceType != want {
		t.Errorf("ce-type = %q, want %q", ceType, want)
	}

	// Exact-set lock (fail-closed) + count, mirroring the JSONShape unit test.
	strmAssertKeySet(t, "account.created", payload, strmAccountCreatedKeys)

	for k := range strmAccountCreatedKeys {
		if _, present := payload[k]; !present {
			t.Errorf("account.created payload missing key %q", k)
		}
	}

	if len(payload) != len(strmAccountCreatedKeys) {
		t.Errorf("account.created payload has %d top-level keys, want %d", len(payload), len(strmAccountCreatedKeys))
	}
}

// TestStreamingTransactionPostedEmitted funds then transfers, and asserts a
// record on lerian.streaming.transaction.posted whose ce-subject is the
// transaction id, ce-type is studio.lerian.transaction.posted, and whose
// payload keys are a subset of the transaction.posted superset (optional
// fields are path-dependent) with the always-present core keys present.
//
// FINDING (supervisor, live-verified): transaction lifecycle streaming events
// have TWO preconditions beyond STREAMING_ENABLED, both off by default:
//  1. RABBITMQ_TRANSACTION_EVENTS_ENABLED=true — a cutover master flag that
//     short-circuits BOTH the legacy rabbit publish AND the lib-streaming Kafka
//     emit together when false (send_transaction_events.go:58,71).
//  2. the async balance-op path — SendTransactionEvents is called only from
//     create_balance_transaction_operations_async.go:145 and the bulk async
//     path, with NO synchronous caller, so the event fires only under
//     RABBITMQ_TRANSACTION_ASYNC=true.
//
// Onboarding events (account/org/ledger) sit behind NEITHER gate — they emit
// synchronously on STREAMING_ENABLED alone (verified: account.created lands in
// the default sync stack). This test therefore needs the operator to enable
// async + transaction events; E2E_ASYNC=1 is the "I configured the async +
// transaction-events stack" signal. Not a defect — a deliberate cutover gate.
func TestStreamingTransactionPostedEmitted(t *testing.T) {
	requireStack(t)
	strmRequireBroker(t)

	if os.Getenv("E2E_ASYNC") != "1" {
		t.Skip("transaction lifecycle streaming events require RABBITMQ_TRANSACTION_EVENTS_ENABLED=true + RABBITMQ_TRANSACTION_ASYNC=true (async-only emit) + STREAMING_ENABLED + topics provisioned; run with E2E_ASYNC=1")
	}

	f := newFixture(t, false)
	createAccount(t, f, "@strm-src")
	createAccount(t, f, "@strm-dst")
	fund(t, f, "@strm-src", "1000")

	// The transfer's response id is the posted transaction's subject.
	txn := mustCreate(t, f.ledgers()+"/transactions/json", transferBody("@strm-src", "@strm-dst", "100", nil))
	txnID := str(t, txn, "id")

	topic := pkgStreaming.TopicName(strmServiceName, "transaction.posted")

	ceType, subject, payload, ok := strmConsumeMatch(t, topic, txnID, 20*time.Second)
	if !ok {
		t.Fatalf("no transaction.posted record with ce-subject=%s on %s within timeout", txnID, topic)
	}

	if subject != txnID {
		t.Errorf("ce-subject = %q, want transaction id %q", subject, txnID)
	}

	if want := strmCETypePrefix + "transaction.posted"; ceType != want {
		t.Errorf("ce-type = %q, want %q", ceType, want)
	}

	// Subset lock (fail-closed): no key may fall outside the declared superset.
	strmAssertKeySet(t, "transaction.posted", payload, strmTransactionPostedKeys)

	for _, k := range strmTransactionPostedCore {
		if _, present := payload[k]; !present {
			t.Errorf("transaction.posted payload missing core key %q", k)
		}
	}

	// scale is intentionally never on the wire (asset-level property).
	if _, hasScale := payload["scale"]; hasScale {
		t.Errorf("transaction.posted payload must not carry scale")
	}
}

// TestStreamingHolderCreateEmitsRedacted asserts the holder.created wire
// contract: creating a holder DOES emit a CloudEvents record on
// lerian.streaming.crm_holder.created whose ce-subject is the holder id and
// ce-type is studio.lerian.holder.created. holder.created is a fully-modeled,
// registered IMPORTANT-posture CRM event (pkg/streaming/events/
// holder_created.go); the constructor redacts PII, so the payload MUST carry
// only id/organizationId/type/externalId/timestamps and MUST NOT carry name,
// document, or any other PII key. This mirrors the JSONShape unit lock in
// holder_created_test.go end-to-end on the live broker.
func TestStreamingHolderCreateEmitsRedacted(t *testing.T) {
	requireStack(t)
	strmRequireBroker(t)

	orgID := createOrg(t)
	holderID := createHolder(t, orgID)

	topic := pkgStreaming.TopicName("crm", "holder.created")

	ceType, subject, payload, ok := strmConsumeMatch(t, topic, holderID, 15*time.Second)
	if !ok {
		t.Fatalf("no holder.created record with ce-subject=%s on %s within timeout", holderID, topic)
	}

	if subject != holderID {
		t.Errorf("ce-subject = %q, want holder id %q", subject, holderID)
	}

	if want := strmCETypePrefix + "holder.created"; ceType != want {
		t.Errorf("ce-type = %q, want %q", ceType, want)
	}

	// Exact-set lock (fail-closed) + count, mirroring the JSONShape unit test.
	strmAssertKeySet(t, "holder.created", payload, strmHolderCreatedKeys)

	for k := range strmHolderCreatedKeys {
		if _, present := payload[k]; !present {
			t.Errorf("holder.created payload missing key %q", k)
		}
	}

	if len(payload) != len(strmHolderCreatedKeys) {
		t.Errorf("holder.created payload has %d top-level keys, want %d", len(payload), len(strmHolderCreatedKeys))
	}

	// PII redaction is the point of this event: no PII key may reach the wire.
	for _, forbidden := range strmHolderCreatedForbidden {
		if _, present := payload[forbidden]; present {
			t.Errorf("holder.created payload leaked PII key %q (must be redacted)", forbidden)
		}
	}
}

// TestStreamingEmitFailureDoesNotFailRequest proves IMPORTANT-posture
// non-propagation: a streaming emit failure logs Warn and never fails the HTTP
// request (pkg/streaming/emit.go EmitImportant, bounded by
// STREAMING_IMPORTANT_EMIT_TIMEOUT_MS). It requires the ledger to be running
// with STREAMING_ENABLED=true pointed at a DEAD, NON-EMPTY broker address, so
// it is gated behind E2E_STREAMING_DEAD_BROKER=1 (skipped otherwise) and does
// NOT use strmRequireBroker — the broker is supposed to be unreachable here.
//
// Operator note: STREAMING_ENABLED=true with an EMPTY STREAMING_BROKERS falls
// back to NoopEmitter (bootstrap/streaming.go), which would emit-succeed and
// invalidate this test. The dead-broker config MUST set a non-empty,
// unreachable address (e.g. STREAMING_BROKERS=localhost:1) so the producer is
// actually constructed and its Emit times out / errors.
func TestStreamingEmitFailureDoesNotFailRequest(t *testing.T) {
	requireStack(t)

	if os.Getenv("E2E_STREAMING_DEAD_BROKER") != "1" {
		t.Skip("set E2E_STREAMING_DEAD_BROKER=1 and run the ledger with STREAMING_ENABLED=true + a non-empty UNREACHABLE STREAMING_BROKERS (e.g. localhost:1) to exercise IMPORTANT-posture emit-failure non-propagation")
	}

	f := newFixture(t, false)

	alias := "@strm-deadbroker-" + uuid.NewString()[:8]

	// LIVE-VERIFY: with a dead non-empty broker the create still returns 201;
	// the emit failure is swallowed by EmitImportant (Warn-logged, bounded by
	// STREAMING_IMPORTANT_EMIT_TIMEOUT_MS). Supervisor confirms the Warn line
	// appears in ledger logs and the request latency stays below the emit
	// timeout ceiling.
	r := call(t, http.MethodPost, f.ledgers()+"/accounts", map[string]any{
		"name": "Dead Broker Acct", "assetCode": "USD", "type": "deposit", "alias": alias,
	})

	if r.status != http.StatusCreated {
		t.Fatalf("account create with dead streaming broker: want 201 (emit failure must not propagate), got %d\nbody: %s", r.status, r.body)
	}
}
