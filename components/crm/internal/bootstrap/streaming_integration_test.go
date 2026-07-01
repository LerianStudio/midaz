//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// This smoke test exercises the REAL CRM streaming path end-to-end: it builds
// the producer via BuildStreamingEmitter, emits all 7 CRM events through
// pkgStreaming.EmitImportant with the real event constructors, then consumes
// them back off Kafka with a franz-go consumer and asserts the CloudEvents
// binary-mode headers (ce-type, ce-subject, ce-tenantid) plus the absence of
// PII on every body.
//
// It requires a LIVE Kafka-compatible broker. Two mechanics are supported:
//
//   - Default (testcontainers): with no STREAMING_BROKERS set, the test starts
//     a self-contained Redpanda container (redpandadata/redpanda) via the
//     testcontainers redpanda module, so it needs Docker but not `make
//     streaming-up`.
//   - External broker (alternative): set STREAMING_BROKERS to an already-
//     running broker (e.g. localhost:19092 from `make streaming-up`, Task
//     3.1.1). The test then skips the container and dials that broker instead,
//     so the same assertions run against the local compose stack without
//     Docker-in-test.
//
// Build/run: this file is gated behind `//go:build integration`, so the default
// unit suite (`go test ./...` with no tag) never compiles or runs it and stays
// broker-free. Run it explicitly with:
//
//	go test -tags=integration -run Streaming ./internal/bootstrap/... -v
//
// or via `make test-streaming-integration`.
//
// NOTE: fixed IDs/timestamps only — no time.Now() — so ce-subject / ce-time
// assertions are exact-match.

package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredpanda "github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	streamingITCEType    = "ce-type"
	streamingITCESubject = "ce-subject"
	streamingITCETenant  = "ce-tenantid"

	streamingITSource   = "lerian.midaz.crm"
	streamingITRedpanda = "redpandadata/redpanda:v24.2.7"
)

// streamingITFixedTime is the deterministic timestamp stamped on every emitted
// event so ce-time round-trips are exact-match. No time.Now() anywhere.
var streamingITFixedTime = time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

// streamingITForbiddenKeys is the union of PII / off-wire keys locked out by
// the JSONShape unit tests. No event body may ever carry any of these.
var streamingITForbiddenKeys = []string{
	"document", "cpf", "cnpj", "name", "contact", "addresses",
	"naturalPerson", "legalPerson", "representative",
	"bankingDetails", "iban", "branch", "account",
	"regulatoryFields", "participantDocument", "metadata",
	"startDate", "endDate",
}

// streamingITExpectation describes one emitted event: the built EmitRequest, the
// topic it routes to, the ce-type it must carry, and the aggregate id that must
// appear as ce-subject.
type streamingITExpectation struct {
	name       string
	topic      string
	ceType     string
	subject    string
	emitReq    func(tenantID string) (libStreaming.EmitRequest, error)
	requireKey []string // keys that MUST be present in the body
}

// TestStreamingEmitter_Integration_AllSevenEvents emits every CRM event through
// the real BuildStreamingEmitter + EmitImportant path and asserts the wire
// contract (ce-type / ce-subject / ce-tenantid + PII absence) per event.
func TestStreamingEmitter_Integration_AllSevenEvents(t *testing.T) {
	ctx := context.Background()

	brokers := brokersFromEnvOrRedpanda(t, ctx)

	expectations := streamingITExpectations()

	// Pre-create the 7 topics so the test never depends on broker auto-create
	// (a typo would otherwise become a silent ghost topic).
	topics := make([]string, 0, len(expectations))
	for _, e := range expectations {
		topics = append(topics, e.topic)
	}

	createTopics(t, ctx, brokers, topics)

	// Build the emitter through the REAL bootstrap path.
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", strings.Join(brokers, ","))
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", streamingITSource)

	cfg := &Config{StreamingEnabled: true}

	emitter, closeFn, err := BuildStreamingEmitter(ctx, cfg, libLog.NewNop(), nil)
	require.NoError(t, err)
	require.NotNil(t, emitter)

	t.Cleanup(func() { _ = closeFn() })

	// Emit all 7 through the IMPORTANT-posture helper (the same call the use
	// cases make). IMPORTANT never propagates errors, so failures surface via
	// the consumer timing out on a missing topic below.
	for _, e := range expectations {
		pkgStreaming.EmitImportant(ctx, nil, libLog.NewNop(), emitter, e.ceType, e.emitReq)
	}

	// Consume each topic and assert the wire contract.
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	require.NoError(t, err)

	t.Cleanup(client.Close)

	got := drainOnePerTopic(t, ctx, client, len(expectations))

	for _, e := range expectations {
		rec, ok := got[e.topic]
		require.Truef(t, ok, "no record consumed from topic %q (ghost topic?)", e.topic)

		assertRecord(t, e, rec)
	}
}

// streamingITExpectations builds the 7 EmitRequests from the real event
// constructors with fixed IDs/times, and pins each to its topic, ce-type, and
// expected ce-subject.
func streamingITExpectations() []streamingITExpectation {
	orgID := "01J7K7XB9C2D3E4F5G6H7J8K9L"

	holderID := uuid.MustParse("0190d9e1-7c2a-7000-8000-0000000000a1")
	holderType := "NATURAL_PERSON"
	holder := &mmodel.Holder{
		ID:        &holderID,
		Type:      &holderType,
		CreatedAt: streamingITFixedTime,
		UpdatedAt: streamingITFixedTime,
	}

	aliasID := uuid.MustParse("0190d9e1-7c2a-7000-8000-0000000000b2")
	aliasHolderID := uuid.MustParse("0190d9e1-7c2a-7000-8000-0000000000c3")
	relatedPartyID := uuid.MustParse("0190d9e1-7c2a-7000-8000-0000000000d4")
	aliasType := "LEGAL_PERSON"
	ledgerID := "01J7K7XB9C2D3E4F5G6H7LEDGR"
	accountID := "01J7K7XB9C2D3E4F5G6H7ACCNT"
	// Seed PII on the domain object to PROVE the wire body drops it.
	document := "91315026015"
	alias := &mmodel.Alias{
		ID:        &aliasID,
		HolderID:  &aliasHolderID,
		Type:      &aliasType,
		LedgerID:  &ledgerID,
		AccountID: &accountID,
		Document:  &document,
		RelatedParties: []*mmodel.RelatedParty{
			{ID: &relatedPartyID, Document: "11122233344", Name: "Jane Roe", Role: "PRIMARY_HOLDER"},
		},
		CreatedAt: streamingITFixedTime,
		UpdatedAt: streamingITFixedTime,
	}

	return []streamingITExpectation{
		{
			name:       "holder.created",
			topic:      streamingTopicPrefix + events.HolderCreatedDefinition.Key(),
			ceType:     "studio.lerian." + events.HolderCreatedDefinition.Key(),
			subject:    holderID.String(),
			requireKey: []string{"id", "organizationId", "type", "createdAt", "updatedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewHolderCreated(holder, orgID).ToEmitRequest(tenantID, holder.CreatedAt)
			},
		},
		{
			name:       "holder.updated",
			topic:      streamingTopicPrefix + events.HolderUpdatedDefinition.Key(),
			ceType:     "studio.lerian." + events.HolderUpdatedDefinition.Key(),
			subject:    holderID.String(),
			requireKey: []string{"id", "organizationId", "type", "createdAt", "updatedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewHolderUpdated(holder, orgID).ToEmitRequest(tenantID, holder.UpdatedAt)
			},
		},
		{
			name:       "holder.deleted",
			topic:      streamingTopicPrefix + events.HolderDeletedDefinition.Key(),
			ceType:     "studio.lerian." + events.HolderDeletedDefinition.Key(),
			subject:    holderID.String(),
			requireKey: []string{"id", "organizationId", "deletionType", "deletedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewHolderDeleted(holderID.String(), orgID, false, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			name:       "alias.created",
			topic:      streamingTopicPrefix + events.AliasCreatedDefinition.Key(),
			ceType:     "studio.lerian." + events.AliasCreatedDefinition.Key(),
			subject:    aliasID.String(),
			requireKey: []string{"id", "holderId", "organizationId", "ledgerId", "accountId", "type", "relatedParties"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewAliasCreated(alias, orgID).ToEmitRequest(tenantID, alias.CreatedAt)
			},
		},
		{
			name:       "alias.updated",
			topic:      streamingTopicPrefix + events.AliasUpdatedDefinition.Key(),
			ceType:     "studio.lerian." + events.AliasUpdatedDefinition.Key(),
			subject:    aliasID.String(),
			requireKey: []string{"id", "holderId", "organizationId", "ledgerId", "accountId", "type", "relatedParties"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewAliasUpdated(alias, orgID).ToEmitRequest(tenantID, alias.UpdatedAt)
			},
		},
		{
			name:       "alias.deleted",
			topic:      streamingTopicPrefix + events.AliasDeletedDefinition.Key(),
			ceType:     "studio.lerian." + events.AliasDeletedDefinition.Key(),
			subject:    aliasID.String(),
			requireKey: []string{"id", "holderId", "organizationId", "deletionType", "deletedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewAliasDeleted(aliasID.String(), aliasHolderID.String(), orgID, true, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			// The hyphenated key AND the subject-is-alias-id subtlety live here:
			// ce-subject MUST be the alias id, NOT the related-party id.
			name:       "alias.related-party-deleted",
			topic:      streamingTopicPrefix + events.AliasRelatedPartyDeletedDefinition.Key(),
			ceType:     "studio.lerian." + events.AliasRelatedPartyDeletedDefinition.Key(),
			subject:    aliasID.String(),
			requireKey: []string{"aliasId", "holderId", "organizationId", "relatedPartyId", "deletedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewAliasRelatedPartyDeleted(
					aliasID.String(), aliasHolderID.String(), orgID, relatedPartyID.String(), streamingITFixedTime,
				).ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
	}
}

// assertRecord locks the CloudEvents wire contract for one consumed record.
func assertRecord(t *testing.T, e streamingITExpectation, rec *kgo.Record) {
	t.Helper()

	headers := map[string]string{}
	for _, h := range rec.Headers {
		headers[h.Key] = string(h.Value)
	}

	assert.Equalf(t, e.ceType, headers[streamingITCEType], "%s: ce-type", e.name)
	assert.Equalf(t, e.subject, headers[streamingITCESubject],
		"%s: ce-subject must be the aggregate id", e.name)

	tenant, present := headers[streamingITCETenant]
	assert.Truef(t, present, "%s: ce-tenantid header must be present", e.name)
	assert.Equalf(t, pkgStreaming.DefaultTenantID, tenant, "%s: ce-tenantid == default", e.name)

	var body map[string]any
	require.NoErrorf(t, json.Unmarshal(rec.Value, &body), "%s: body must be JSON", e.name)

	for _, key := range e.requireKey {
		_, ok := body[key]
		assert.Truef(t, ok, "%s: body must include %q", e.name, key)
	}

	for _, forbidden := range streamingITForbiddenKeys {
		_, present := body[forbidden]
		assert.Falsef(t, present, "%s: body must NOT include PII key %q", e.name, forbidden)
	}
}

// brokersFromEnvOrRedpanda returns the broker list to test against. When
// STREAMING_BROKERS is set it is used verbatim (external-broker mode);
// otherwise a Redpanda testcontainer is started and its seed broker returned.
func brokersFromEnvOrRedpanda(t *testing.T, ctx context.Context) []string {
	t.Helper()

	if external := strings.TrimSpace(os.Getenv("STREAMING_BROKERS")); external != "" {
		return strings.Split(external, ",")
	}

	container, err := tcredpanda.Run(ctx, streamingITRedpanda)
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	seed, err := container.KafkaSeedBroker(ctx)
	require.NoError(t, err)

	return []string{seed}
}

// createTopics pre-provisions the given topics via the franz-go admin client,
// tolerating "already exists" so external-broker mode (topics pre-created by
// make streaming-up) is a no-op.
func createTopics(t *testing.T, ctx context.Context, brokers, topics []string) {
	t.Helper()

	admClient, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	require.NoError(t, err)

	defer admClient.Close()

	adm := kadm.NewClient(admClient)

	_, err = adm.CreateTopics(ctx, 1, 1, nil, topics...)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		require.NoError(t, err)
	}
}

// drainOnePerTopic polls until it has captured one record per topic (or the
// context deadline fires), returning the first record seen on each topic.
func drainOnePerTopic(t *testing.T, ctx context.Context, client *kgo.Client, want int) map[string]*kgo.Record {
	t.Helper()

	pollCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	got := map[string]*kgo.Record{}

	for len(got) < want {
		fetches := client.PollFetches(pollCtx)
		if err := pollCtx.Err(); err != nil {
			t.Fatalf("timed out consuming events: got %d of %d topics: %v", len(got), want, err)
		}

		if errs := fetches.Errors(); len(errs) > 0 {
			t.Fatalf("fetch errors: %+v", errs)
		}

		fetches.EachRecord(func(rec *kgo.Record) {
			if _, seen := got[rec.Topic]; !seen {
				got[rec.Topic] = rec
			}
		})
	}

	return got
}
