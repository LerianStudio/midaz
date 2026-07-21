// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// defaultSmokeBroker is the host-facing Redpanda listener the local
// components/infra stack advertises (external://localhost:19092). Overridable
// via STREAMING_BROKERS so CI can target another broker.
const defaultSmokeBroker = "localhost:19092"

// smokeDeadline bounds the whole emit+consume round-trip so a wedged broker
// can never hang the suite. Generous because a cold Redpanda + first-write
// topic-leader election can take a few seconds.
const smokeDeadline = 45 * time.Second

// ceType/ceSubject/ceTenantID are the CloudEvents binary-mode header keys
// lib-streaming stamps on every record. Asserted per-record after consume.
const (
	headerCEType     = "ce-type"
	headerCESubject  = "ce-subject"
	headerCETenantID = "ce-tenantid"
)

// forbiddenPayloadKeys are the fenced fields that must NEVER appear on any
// tracer event payload — free text and financial detail. Asserted absent at
// top level AND inside every nested scope object.
var forbiddenPayloadKeys = []string{"name", "description", "expression", "maxAmount", "compiledProgram"}

// smokeEvent pairs an EmitRequest with the ce-type it must surface once
// consumed back, so the assertion loop can look each record up by subject.
type smokeEvent struct {
	subject    string // aggregate UUID (ce-subject)
	wantCEType string // "studio.lerian.<resource>.<event>"
	request    libStreaming.EmitRequest
}

// TestStreamingSmoke drives the REAL tracer emitter end-to-end against a live
// Redpanda broker: it ensures the 12 tracer topics exist, emits all 12
// rule/limit lifecycle events with the forbidden fields deliberately populated
// on the domain fixtures, then CONSUMES the records back and asserts the
// CloudEvents headers and the payload fence for each one. This is the core
// contract check — not merely "emit returned no error".
//
// The test skips (never fails) when no broker is reachable, so a unit-only CI
// run without infra stays green. It runs under the `integration` build tag so
// `make test-unit` excludes it.
func TestStreamingSmoke(t *testing.T) {
	broker := strings.TrimSpace(os.Getenv("STREAMING_BROKERS"))
	if broker == "" {
		broker = defaultSmokeBroker
	}

	// First broker in a comma list is enough for a single-node dev cluster.
	broker = strings.Split(broker, ",")[0]

	ctx, cancel := context.WithTimeout(context.Background(), smokeDeadline)
	defer cancel()

	skipIfBrokerUnreachable(ctx, t, broker)

	// Unique per-run identifiers so repeat runs never read each other's
	// records. tenant is stamped into every event and matched on consume.
	runID := uuid.NewString()
	tenant := "smoke-" + runID

	smokeEvents := buildSmokeEvents(t, tenant)
	require.Len(t, smokeEvents, 12, "expected exactly 12 tracer lifecycle events")

	topics := smokeTopics(smokeEvents)
	ensureTopics(ctx, t, broker, topics)

	emitter := buildSmokeEmitter(ctx, t, broker)
	defer func() {
		require.NoError(t, emitter.Close(), "emitter Close must drain cleanly")
	}()

	emitAll(ctx, t, emitter, smokeEvents)

	consumed := consumeRecords(ctx, t, broker, topics, tenant, len(smokeEvents))
	assertConsumed(t, tenant, smokeEvents, consumed)
}

// skipIfBrokerUnreachable probes the broker's metadata once. An unreachable
// broker means "no infra" -> t.Skip (safe for unit CI). A reachable broker
// means the smoke MUST run.
func skipIfBrokerUnreachable(ctx context.Context, t *testing.T, broker string) {
	t.Helper()

	cl, err := kgo.NewClient(kgo.SeedBrokers(broker))
	if err != nil {
		t.Skipf("streaming smoke skipped: cannot build probe client for %q: %v", broker, err)
	}

	defer cl.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := cl.Ping(pingCtx); err != nil {
		t.Skipf("streaming smoke skipped: broker %q unreachable: %v", broker, err)
	}
}

// buildSmokeEmitter constructs the production emitter via BuildStreamingEmitter
// with a non-empty catalog so the live producer path (not Noop) is taken.
func buildSmokeEmitter(ctx context.Context, t *testing.T, broker string) libStreaming.Emitter {
	t.Helper()

	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", broker)
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.tracer")

	cfg := &Config{
		StreamingEnabled:           true,
		StreamingCloudEventsSource: "lerian.midaz.tracer",
	}

	emitter, closeFn, err := BuildStreamingEmitter(ctx, cfg, nil, nil)
	require.NoError(t, err, "BuildStreamingEmitter must succeed against a live broker")
	require.NotNil(t, emitter)
	require.NotNil(t, closeFn)

	// A live producer must NOT be the Noop fallback: assert the concrete live
	// type positively rather than DeepEqual against an empty Noop struct.
	require.IsType(t, &libStreaming.Producer{}, emitter,
		"expected a live *libStreaming.Producer, got the NoopEmitter fallback")

	// And it must report healthy against the reachable broker.
	require.NoError(t, emitter.Healthy(ctx), "live emitter must be healthy against the broker")

	return emitter
}

// buildSmokeEvents constructs the 12 domain fixtures with name/description/
// expression/maxAmount populated ON PURPOSE, runs them through the events.New*
// constructors, and assembles the EmitRequests. Deterministic where it matters:
// the subject UUIDs and timestamps are fixed per event; only the run tenant is
// unique.
func buildSmokeEvents(t *testing.T, tenant string) []smokeEvent {
	t.Helper()

	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	rule := newSmokeRule(ts)
	limit := newSmokeLimit(t, ts)

	// reqFn defers the ToEmitRequest call so the two-value return can be
	// unpacked and its error checked in one place per event.
	type reqFn func(tenant string, ts time.Time) (libStreaming.EmitRequest, error)

	specs := []struct {
		subject    string
		wantCEType string
		ts         time.Time
		build      reqFn
	}{
		{rule.ID.String(), "studio.lerian.rule.created", rule.CreatedAt, events.NewRuleCreated(rule).ToEmitRequest},
		{rule.ID.String(), "studio.lerian.rule.updated", rule.UpdatedAt, events.NewRuleUpdated(rule).ToEmitRequest},
		{rule.ID.String(), "studio.lerian.rule.activated", rule.UpdatedAt, events.NewRuleActivated(rule).ToEmitRequest},
		{rule.ID.String(), "studio.lerian.rule.deactivated", rule.UpdatedAt, events.NewRuleDeactivated(rule).ToEmitRequest},
		{rule.ID.String(), "studio.lerian.rule.drafted", rule.UpdatedAt, events.NewRuleDrafted(rule).ToEmitRequest},
		{rule.ID.String(), "studio.lerian.rule.deleted", ts, events.NewRuleDeleted(rule.ID, ts).ToEmitRequest},
		{limit.ID.String(), "studio.lerian.limit.created", limit.CreatedAt, events.NewLimitCreated(limit).ToEmitRequest},
		{limit.ID.String(), "studio.lerian.limit.updated", limit.UpdatedAt, events.NewLimitUpdated(limit).ToEmitRequest},
		{limit.ID.String(), "studio.lerian.limit.activated", limit.UpdatedAt, events.NewLimitActivated(limit).ToEmitRequest},
		{limit.ID.String(), "studio.lerian.limit.deactivated", limit.UpdatedAt, events.NewLimitDeactivated(limit).ToEmitRequest},
		{limit.ID.String(), "studio.lerian.limit.drafted", limit.UpdatedAt, events.NewLimitDrafted(limit).ToEmitRequest},
		{limit.ID.String(), "studio.lerian.limit.deleted", ts, events.NewLimitDeleted(limit).ToEmitRequest},
	}

	out := make([]smokeEvent, 0, len(specs))

	for _, s := range specs {
		req, err := s.build(tenant, s.ts)
		require.NoErrorf(t, err, "ToEmitRequest failed for %s", s.wantCEType)

		out = append(out, smokeEvent{
			subject:    s.subject,
			wantCEType: s.wantCEType,
			request:    req,
		})
	}

	return out
}

// newSmokeRule builds a Rule with the fenced fields (Name, Description,
// Expression, CompiledProgram) deliberately populated, plus a fully populated
// scope, so the payload-fence assertion proves those never reach the wire.
func newSmokeRule(ts time.Time) *model.Rule {
	desc := "fenced description that must never hit the wire"
	activated := ts
	deactivated := ts

	return &model.Rule{
		ID:              uuid.New(),
		Name:            "fenced rule name",
		Description:     &desc,
		Expression:      "transaction.amount > 1000",
		Action:          model.DecisionDeny,
		Scopes:          []model.Scope{fencedScope()},
		Status:          model.RuleStatusActive,
		CreatedAt:       ts,
		UpdatedAt:       ts,
		ActivatedAt:     &activated,
		DeactivatedAt:   &deactivated,
		CompiledProgram: struct{ compiled bool }{compiled: true},
	}
}

// newSmokeLimit builds a Limit with the fenced fields (Name, Description,
// MaxAmount) deliberately populated so the payload-fence assertion proves the
// financial value and free text never reach the wire.
func newSmokeLimit(t *testing.T, ts time.Time) *model.Limit {
	t.Helper()

	desc := "fenced limit description"

	start, err := model.NewTimeOfDay("09:00")
	require.NoError(t, err)

	end, err := model.NewTimeOfDay("17:00")
	require.NoError(t, err)

	resetAt := ts.Add(24 * time.Hour)

	return &model.Limit{
		ID:              uuid.New(),
		Name:            "fenced limit name",
		Description:     &desc,
		LimitType:       model.LimitTypeDaily,
		MaxAmount:       decimal.RequireFromString("1000.00"),
		Currency:        "USD",
		Scopes:          []model.Scope{fencedScope()},
		Status:          model.LimitStatusActive,
		ActiveTimeStart: &start,
		ActiveTimeEnd:   &end,
		ResetAt:         &resetAt,
		CreatedAt:       ts,
		UpdatedAt:       ts,
	}
}

// fencedScope returns a fully populated scope so the nested-scope fence check
// runs against real values rather than nils.
func fencedScope() model.Scope {
	segment := uuid.New()
	portfolio := uuid.New()
	account := uuid.New()
	merchant := uuid.New()
	txType := model.TransactionTypeCard
	subType := "purchase"

	return model.Scope{
		SegmentID:       &segment,
		PortfolioID:     &portfolio,
		AccountID:       &account,
		MerchantID:      &merchant,
		TransactionType: &txType,
		SubType:         &subType,
	}
}

// smokeTopics maps each event's ce-type back to its canonical topic name
// "lerian.streaming.tracer_<resource>.<event>" (service segment folded into
// the first topic segment, hyphens normalized to underscores).
func smokeTopics(evs []smokeEvent) []string {
	out := make([]string, 0, len(evs))

	for _, e := range evs {
		key := strings.TrimPrefix(e.wantCEType, "studio.lerian.")
		out = append(out, pkgStreaming.TopicName("tracer", key))
	}

	return out
}

// ensureTopics idempotently creates the given topics (1 partition, RF 1) via
// a kadm admin client. Already-exists is not an error.
func ensureTopics(ctx context.Context, t *testing.T, broker string, topics []string) {
	t.Helper()

	cl, err := kgo.NewClient(kgo.SeedBrokers(broker))
	require.NoError(t, err)

	defer cl.Close()

	admin := kadm.NewClient(cl)

	createCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := admin.CreateTopics(createCtx, 1, 1, nil, topics...)
	require.NoError(t, err, "CreateTopics call must not fail at the RPC level")

	for _, ct := range resp.Sorted() {
		if ct.Err != nil && !strings.Contains(ct.Err.Error(), "already exists") {
			t.Fatalf("failed to ensure topic %q: %v", ct.Topic, ct.Err)
		}
	}
}

// emitAll emits every event through the real producer. Each Emit must succeed;
// the whole point of the smoke test is that the live path works.
func emitAll(ctx context.Context, t *testing.T, emitter libStreaming.Emitter, evs []smokeEvent) {
	t.Helper()

	for _, e := range evs {
		require.NoErrorf(t, emitter.Emit(ctx, e.request), "Emit failed for %s", e.wantCEType)
	}
}

// consumedRecord is the decoded shape used by the assertion loop.
type consumedRecord struct {
	ceType   string
	subject  string
	tenantID string
	value    []byte
}

// consumeRecords reads from the given topics at the start of each partition
// until `want` records CARRYING THIS RUN'S TENANT are collected or the context
// deadline fires. A fresh consumer group per run guarantees AtStart reads every
// record. The tenant filter lives inside the collection predicate so leftover
// records from prior runs on a reused broker never fill this run's budget —
// only this run's tenant-matching records count, keeping the smoke re-runnable.
func consumeRecords(ctx context.Context, t *testing.T, broker string, topics []string, tenant string, want int) []consumedRecord {
	t.Helper()

	group := "smoke-consumer-" + uuid.NewString()

	cl, err := kgo.NewClient(
		kgo.SeedBrokers(broker),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	require.NoError(t, err)

	defer cl.Close()

	collected := make([]consumedRecord, 0, want)

	for len(collected) < want {
		if err := ctx.Err(); err != nil {
			t.Fatalf("deadline reached with only %d/%d tenant-matching records consumed: %v", len(collected), want, err)
		}

		pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		fetches := cl.PollFetches(pollCtx)
		cancel()

		for _, fe := range fetches.Errors() {
			// The deadline path (ctx.Err() != nil) is the normal per-poll
			// termination between batches; keep looping. Any other fetch error
			// on a live broker is a real fault — fail fast with its cause.
			if fe.Err != nil && ctx.Err() == nil {
				t.Fatalf("fetch error on topic %q: %v", fe.Topic, fe.Err)
			}
		}

		fetches.EachRecord(func(r *kgo.Record) {
			rec := decodeRecord(r)
			if rec.tenantID != tenant {
				return
			}

			collected = append(collected, rec)
		})
	}

	return collected
}

// decodeRecord pulls the CloudEvents binary-mode headers off a kgo record and
// keeps the raw JSON value for the fence check.
func decodeRecord(r *kgo.Record) consumedRecord {
	cr := consumedRecord{value: r.Value}

	for _, h := range r.Headers {
		switch h.Key {
		case headerCEType:
			cr.ceType = string(h.Value)
		case headerCESubject:
			cr.subject = string(h.Value)
		case headerCETenantID:
			cr.tenantID = string(h.Value)
		}
	}

	return cr
}

// assertConsumed verifies that each emitted event was read back with the
// correct ce-type / ce-subject / ce-tenantid and a fenced payload. Only records
// carrying this run's tenant are considered, so leftover topic data from other
// runs is ignored.
func assertConsumed(t *testing.T, tenant string, evs []smokeEvent, consumed []consumedRecord) {
	t.Helper()

	byType := make(map[string]consumedRecord, len(consumed))

	for _, c := range consumed {
		if c.tenantID != tenant {
			continue
		}

		byType[c.ceType] = c
	}

	for _, e := range evs {
		got, ok := byType[e.wantCEType]
		require.Truef(t, ok, "no consumed record for %s (tenant %s)", e.wantCEType, tenant)

		require.Equal(t, e.wantCEType, got.ceType, "ce-type mismatch")
		require.Equal(t, e.subject, got.subject, "ce-subject must be the aggregate UUID for %s", e.wantCEType)
		require.NotEmpty(t, got.tenantID, "ce-tenantid must be present for %s", e.wantCEType)

		assertPayloadFenced(t, e.wantCEType, got.value)
	}
}

// assertPayloadFenced unmarshals the record value and asserts none of the
// forbidden keys appear at top level or inside any nested scope object.
func assertPayloadFenced(t *testing.T, ceType string, value []byte) {
	t.Helper()

	var payload map[string]any
	require.NoErrorf(t, json.Unmarshal(value, &payload), "payload for %s must be valid JSON", ceType)

	assertNoForbiddenKeys(t, ceType, "<top-level>", payload)

	// Scopes are the only nested objects on tracer payloads. Descend into each.
	if rawScopes, ok := payload["scopes"]; ok {
		scopes, isSlice := rawScopes.([]any)
		require.Truef(t, isSlice, "scopes must be an array for %s", ceType)

		for i, rawScope := range scopes {
			scope, isMap := rawScope.(map[string]any)
			require.Truef(t, isMap, "scope[%d] must be an object for %s", i, ceType)
			assertNoForbiddenKeys(t, ceType, fmt.Sprintf("scopes[%d]", i), scope)
		}
	}
}

// assertNoForbiddenKeys fails if any fenced key is present in the given object.
func assertNoForbiddenKeys(t *testing.T, ceType, scope string, obj map[string]any) {
	t.Helper()

	for _, k := range forbiddenPayloadKeys {
		_, present := obj[k]
		require.Falsef(t, present, "forbidden key %q leaked into %s of %s", k, scope, ceType)
	}
}
