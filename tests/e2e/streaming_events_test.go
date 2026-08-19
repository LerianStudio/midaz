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

	libStreaming "github.com/LerianStudio/lib-streaming/v3"

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

// strmServiceName is the ledger's name on the streaming roster: its ce-source,
// and the single segment its ONE topic is derived from. Every event the ledger
// emits rides "lerian.streaming.ledger".
const strmServiceName = "ledger"

// strmCETypeFor is the ce-type header lib-streaming stamps for one event, composed
// through the library's own facade rather than by re-spelling the prefix and
// separators here: the reverse-DNS namespace, the PRODUCING APPLICATION, then the
// resource and event types. The application segment is what stops two services emitting a
// byte-identical ce-type for same-named events — a homonym collision a consumer
// reading only ce-type could not detect, and one that a single topic per
// application makes reachable in practice.
func strmCETypeFor(resourceType, eventType string) string {
	return libStreaming.CloudEventsType(strmServiceName, resourceType, eventType)
}

// strmAppTopic is the one topic the ledger publishes every fact to.
func strmAppTopic(t *testing.T) string {
	t.Helper()

	topic, err := libStreaming.AppTopic(strmServiceName)
	if err != nil {
		t.Fatalf("derive ledger application topic: %v", err)
	}

	return topic
}

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
// wire payload, asserted as an exact set (fail-closed): an extra or missing key
// means wire drift.
var strmHolderCreatedKeys = map[string]struct{}{
	"id": {}, "organizationId": {}, "type": {}, "externalId": {},
	"createdAt": {}, "updatedAt": {},
}

// strmHolderCreatedForbidden is the PII key set that MUST NEVER appear on the
// holder.created wire payload.
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

	for _, name := range strmCatalogTopics(t) {
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

// strmCatalogTopics returns every topic the ledger writes: its ONE application
// topic — carrying the whole event catalog, ledger core plus fees plus CRM — and
// its DLQ, where a failed publish is copied.
//
// The former per-event list (49 topics enumerated by hand) is gone with the topic
// collapse. That is the point of provisioning it here: lib-streaming's producer
// does not request auto-topic-creation, so a missing destination both fails the
// emit AND trips the circuit breaker, poisoning every later emit. Two names now
// cover the whole catalog instead of forty-nine that had to stay in sync with the
// registry by hand.
func strmCatalogTopics(t *testing.T) []string {
	t.Helper()

	dlqTopic, err := libStreaming.AppDLQTopic(strmServiceName)
	if err != nil {
		t.Fatalf("derive ledger DLQ topic: %v", err)
	}

	return []string{strmAppTopic(t), dlqTopic}
}

// strmConsumeMatch consumes the ledger's application topic from the beginning
// with a short poll loop and returns the decoded payload of the first record
// matching BOTH wantCEType and wantSubject.
//
// Matching on the pair is required, not belt-and-braces: selection moved from the
// broker into the consumer with the topic collapse, so one subscription now
// delivers every event the ledger emits. A subject-only filter would happily
// return transaction.committed when the caller asked for transaction.posted —
// both carry the transaction id as ce-subject — and assert the wrong contract
// against the wrong payload.
//
// A unique consumer group is used per call (group offset reset to earliest) so
// repeated runs replay from the start rather than resuming a committed offset.
func strmConsumeMatch(t *testing.T, topic, wantCEType, wantSubject string, timeout time.Duration) (payload map[string]any, found bool) {
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

			if subj, ok := strmHeader(rec, strmHeaderCESubject); !ok || subj != wantSubject {
				continue
			}

			if ct, ok := strmHeader(rec, strmHeaderCEType); !ok || ct != wantCEType {
				continue
			}

			var decoded map[string]any
			_ = json.Unmarshal(rec.Value, &decoded)

			return decoded, true
		}
	}

	return nil, false
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
// CloudEvents record on the ledger's application topic whose ce-subject is the
// account id, ce-type is studio.lerian.ledger.account.created, and whose payload
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

	topic := strmAppTopic(t)
	wantCEType := strmCETypeFor("account", "created")

	payload, ok := strmConsumeMatch(t, topic, wantCEType, accID, 15*time.Second)
	if !ok {
		t.Fatalf("no record with ce-type=%s and ce-subject=%s on %s within timeout", wantCEType, accID, topic)
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
// record on the ledger's application topic whose ce-subject is the transaction
// id, ce-type is studio.lerian.ledger.transaction.posted, and whose payload keys
// are a subset of the transaction.posted superset (optional fields are
// path-dependent) with the always-present core keys present.
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

	topic := strmAppTopic(t)
	wantCEType := strmCETypeFor("transaction", "posted")

	// The ce-type half of the filter is load-bearing here: transaction.committed
	// carries the SAME transaction id as ce-subject and rides the same topic.
	payload, ok := strmConsumeMatch(t, topic, wantCEType, txnID, 20*time.Second)
	if !ok {
		t.Fatalf("no record with ce-type=%s and ce-subject=%s on %s within timeout", wantCEType, txnID, topic)
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

// TestStreamingHolderCreateEmitsRedacted asserts that creating a holder emits a
// CloudEvents record on the ledger's application topic whose ce-subject is the
// holder id and ce-type is studio.lerian.ledger.holder.created, with PII
// redacted: the payload MUST carry only id/organizationId/type/externalId/
// timestamps and MUST NOT carry name, document, or any other PII key.
func TestStreamingHolderCreateEmitsRedacted(t *testing.T) {
	requireStack(t)
	strmRequireBroker(t)

	orgID := createOrg(t)
	holderID := createHolder(t, orgID)

	topic := strmAppTopic(t)
	wantCEType := strmCETypeFor("holder", "created")

	payload, ok := strmConsumeMatch(t, topic, wantCEType, holderID, 15*time.Second)
	if !ok {
		t.Fatalf("no record with ce-type=%s and ce-subject=%s on %s within timeout", wantCEType, holderID, topic)
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
