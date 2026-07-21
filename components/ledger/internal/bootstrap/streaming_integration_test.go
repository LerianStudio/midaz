//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// This smoke test exercises the REAL ledger fee-streaming path end-to-end: it
// builds the producer via BuildStreamingEmitter, emits all 7 fee events through
// pkgStreaming.EmitImportant with the real event constructors, then consumes
// them back off Kafka with a franz-go consumer and asserts the CloudEvents
// binary-mode headers (ce-type, ce-source, ce-subject, ce-tenantid) plus the
// absence of fee-detail / PII keys on every body.
//
// It requires a LIVE Kafka-compatible broker supplied via STREAMING_BROKERS
// (e.g. the infra `midaz-redpanda` service on localhost:19092 after `make up`).
// When STREAMING_BROKERS is empty the test skips cleanly — it never starts a
// broker itself.
//
// Build/run: this file is gated behind `//go:build integration`, so the default
// unit suite (`go test ./...` with no tag) never compiles or runs it and stays
// broker-free. Run it explicitly with:
//
//	STREAMING_BROKERS=localhost:19092 \
//	  go test -tags=integration -run Streaming ./internal/bootstrap/... -v
//
// NOTE: the timestamp is fixed (no time.Now()) so ce-time round-trips are
// exact-match, but the aggregate IDs are freshly generated per run via
// uuid.New() and the consumer matches only records whose ce-subject belongs to
// THIS run. A broker that still holds records from a prior run (the infra
// Redpanda persists) therefore cannot mask a later regression with a stale
// record: those records carry different UUIDs and are skipped.

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
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// errCodeTopicAlreadyExists is the Kafka TOPIC_ALREADY_EXISTS error code,
// tolerated so a broker with the topics pre-created is a no-op.
const errCodeTopicAlreadyExists int16 = 36

const (
	streamingITCEType    = "ce-type"
	streamingITCESource  = "ce-source"
	streamingITCESubject = "ce-subject"
	streamingITCETenant  = "ce-tenantid"

	streamingITSource = "lerian.midaz.ledger"
)

// streamingITFixedTime is the deterministic timestamp stamped on every emitted
// event so ce-time round-trips are exact-match. No time.Now() anywhere.
var streamingITFixedTime = time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

// streamingITForbiddenKeys is the union of fee-detail / PII keys DELIBERATELY
// held off the wire by the fee event payloads. No event body may ever carry any
// of these; the per-event JSONShape unit tests lock the same absence.
var streamingITForbiddenKeys = []string{
	"feeGroupLabel", "description", "minimumAmount", "maximumAmount", "fees",
	"waivedAccounts", "label", "assetCode", "feeAmount", "tiers", "discountTiers",
	"freeQuota", "eventFilter", "accountTarget", "debitAccountAlias",
	"creditAccountAlias", "maintenanceCreditAccount", "amount", "source",
	"destination", "operations", "metadata",
}

// streamingITExpectation describes one emitted event: the built EmitRequest
// closure, the topic it routes to, the ce-type it must carry, and the aggregate
// id that must appear as ce-subject.
type streamingITExpectation struct {
	name       string
	topic      string
	ceType     string
	subject    string
	emitReq    func(tenantID string) (libStreaming.EmitRequest, error)
	requireKey []string // keys that MUST be present in the body
}

// TestStreamingEmitter_Integration_AllSevenFeeEvents emits every fee event
// through the real BuildStreamingEmitter + EmitImportant path and asserts the
// wire contract (ce-type / ce-source / ce-subject / ce-tenantid + fee-detail
// absence) per event.
func TestStreamingEmitter_Integration_AllSevenFeeEvents(t *testing.T) {
	brokersEnv := strings.TrimSpace(os.Getenv("STREAMING_BROKERS"))
	if brokersEnv == "" {
		t.Skip("set STREAMING_BROKERS (e.g. localhost:19092, e.g. via make up) to run the streaming smoke test")
	}

	ctx := context.Background()
	brokers := strings.Split(brokersEnv, ",")

	expectations := streamingITExpectations()

	// Pre-create the 7 topics so the test never depends on broker auto-create
	// (a typo would otherwise become a silent ghost topic).
	topics := make([]string, 0, len(expectations))
	for _, e := range expectations {
		topics = append(topics, e.topic)
	}

	createTopics(t, ctx, brokers, topics)

	// Build the emitter through the REAL bootstrap path. LoadConfig reads
	// STREAMING_BROKERS / STREAMING_CLOUDEVENTS_SOURCE from env.
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", brokersEnv)
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

	// Match only records whose ce-subject is THIS run's freshly-generated id for
	// the topic. Stale records from a prior run carry different UUIDs and are
	// skipped, so a persistent broker cannot mask a later regression.
	wantSubjectByTopic := make(map[string]string, len(expectations))
	for _, e := range expectations {
		wantSubjectByTopic[e.topic] = e.subject
	}

	got := drainOnePerTopic(t, ctx, client, wantSubjectByTopic)

	for _, e := range expectations {
		rec, ok := got[e.topic]
		require.Truef(t, ok, "no record consumed from topic %q (ghost topic?)", e.topic)

		assertRecord(t, e, rec)
	}
}

// streamingITExpectations builds the 7 EmitRequests from the real fee event
// constructors with fixed IDs/times, and pins each to its topic, ce-type, and
// expected ce-subject.
func streamingITExpectations() []streamingITExpectation {
	// Fresh valid-hex UUIDs per run so the consumer can match only THIS run's
	// records and ignore stale ones left on a persistent broker.
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	packageID := uuid.New().String()
	segmentID := uuid.New().String()
	transactionRoute := uuid.New().String()

	billingID := uuid.New().String()
	transactionID := uuid.New().String()

	// Seed fee-detail surface on the billing package to PROVE the wire body
	// drops it.
	desc := "Charges per completed transaction route"
	pricingModel := "tiered"
	countMode := "perRoute"
	assetCode := "BRL"
	enable := true
	feeAmount := decimal.NewFromInt(50)
	billing := &model.BillingPackage{
		ID:             billingID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Label:          "Monthly Volume Billing",
		Description:    &desc,
		Type:           "volume",
		Enable:         &enable,
		PricingModel:   &pricingModel,
		CountMode:      &countMode,
		AssetCode:      &assetCode,
		FeeAmount:      &feeAmount,
		CreatedAt:      streamingITFixedTime.Format(time.RFC3339),
		UpdatedAt:      streamingITFixedTime.Format(time.RFC3339),
	}

	return []streamingITExpectation{
		{
			name:       "fees-package.created",
			topic:      pkgStreaming.TopicName(streamingServiceName, events.FeesPackageCreatedDefinition.Key()),
			ceType:     "studio.lerian." + events.FeesPackageCreatedDefinition.Key(),
			subject:    packageID,
			requireKey: []string{"id", "organizationId", "ledgerId", "enable", "createdAt", "updatedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesPackageCreated(packageID, orgID, ledgerID, &segmentID, &transactionRoute, true, streamingITFixedTime, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			name:       "fees-package.updated",
			topic:      pkgStreaming.TopicName(streamingServiceName, events.FeesPackageUpdatedDefinition.Key()),
			ceType:     "studio.lerian." + events.FeesPackageUpdatedDefinition.Key(),
			subject:    packageID,
			requireKey: []string{"id", "organizationId", "ledgerId", "enable", "createdAt", "updatedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesPackageUpdated(packageID, orgID, ledgerID, &segmentID, &transactionRoute, true, streamingITFixedTime, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			name:       "fees-package.deleted",
			topic:      pkgStreaming.TopicName(streamingServiceName, events.FeesPackageDeletedDefinition.Key()),
			ceType:     "studio.lerian." + events.FeesPackageDeletedDefinition.Key(),
			subject:    packageID,
			requireKey: []string{"id", "organizationId", "ledgerId", "deletedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesPackageDeleted(packageID, orgID, ledgerID, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			name:       "fees-billing-package.created",
			topic:      pkgStreaming.TopicName(streamingServiceName, events.FeesBillingPackageCreatedDefinition.Key()),
			ceType:     "studio.lerian." + events.FeesBillingPackageCreatedDefinition.Key(),
			subject:    billingID,
			requireKey: []string{"id", "organizationId", "ledgerId", "type", "enable", "createdAt", "updatedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesBillingPackageCreated(
					billing.ID, billing.OrganizationID, billing.LedgerID, billing.Type,
					billing.PricingModel, billing.CountMode, billing.Enable != nil && *billing.Enable,
					billing.CreatedAt, billing.UpdatedAt,
				).ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			name:       "fees-billing-package.updated",
			topic:      pkgStreaming.TopicName(streamingServiceName, events.FeesBillingPackageUpdatedDefinition.Key()),
			ceType:     "studio.lerian." + events.FeesBillingPackageUpdatedDefinition.Key(),
			subject:    billingID,
			requireKey: []string{"id", "organizationId", "ledgerId", "type", "enable", "createdAt", "updatedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesBillingPackageUpdated(
					billing.ID, billing.OrganizationID, billing.LedgerID, billing.Type,
					billing.PricingModel, billing.CountMode, billing.Enable != nil && *billing.Enable,
					billing.CreatedAt, billing.UpdatedAt,
				).ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			name:       "fees-billing-package.deleted",
			topic:      pkgStreaming.TopicName(streamingServiceName, events.FeesBillingPackageDeletedDefinition.Key()),
			ceType:     "studio.lerian." + events.FeesBillingPackageDeletedDefinition.Key(),
			subject:    billingID,
			requireKey: []string{"id", "organizationId", "ledgerId", "deletedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesBillingPackageDeleted(billingID, orgID, ledgerID, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			// ce-subject for fees.applied is the TRANSACTION id, not a package id.
			name:       "fees.applied",
			topic:      pkgStreaming.TopicName(streamingServiceName, events.FeesAppliedDefinition.Key()),
			ceType:     "studio.lerian." + events.FeesAppliedDefinition.Key(),
			subject:    transactionID,
			requireKey: []string{"transactionId", "organizationId", "ledgerId", "feePackageId", "appliedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesApplied(transactionID, orgID, ledgerID, packageID, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
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
	assert.Equalf(t, streamingITSource, headers[streamingITCESource], "%s: ce-source", e.name)
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
		assert.Falsef(t, present, "%s: body must NOT include fee-detail/PII key %q", e.name, forbidden)
	}
}

// createTopics pre-provisions the given topics via a raw kmsg CreateTopics
// request, tolerating TOPIC_ALREADY_EXISTS so a broker with the topics
// pre-created is a no-op. Uses only kgo + kmsg (no kadm) to avoid a new module.
func createTopics(t *testing.T, ctx context.Context, brokers, topics []string) {
	t.Helper()

	client, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	require.NoError(t, err)

	defer client.Close()

	req := kmsg.NewPtrCreateTopicsRequest()
	for _, topic := range topics {
		rt := kmsg.NewCreateTopicsRequestTopic()
		rt.Topic = topic
		rt.NumPartitions = 1
		rt.ReplicationFactor = 1
		req.Topics = append(req.Topics, rt)
	}

	resp, err := req.RequestWith(ctx, client)
	require.NoError(t, err)

	for _, topic := range resp.Topics {
		if topic.ErrorCode != 0 && topic.ErrorCode != errCodeTopicAlreadyExists {
			t.Fatalf("create topic %q failed: error code %d", topic.Topic, topic.ErrorCode)
		}
	}
}

// drainOnePerTopic polls until it has captured, for every topic, the record
// whose ce-subject matches this run's expected subject (or the context deadline
// fires). Records whose ce-subject is not this run's expected subject for their
// topic — e.g. stale records from a prior run on a persistent broker — are
// skipped, so they cannot mask a regression.
func drainOnePerTopic(t *testing.T, ctx context.Context, client *kgo.Client, wantSubjectByTopic map[string]string) map[string]*kgo.Record {
	t.Helper()

	pollCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	want := len(wantSubjectByTopic)
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
			wantSubject, tracked := wantSubjectByTopic[rec.Topic]
			if !tracked {
				return
			}

			if _, seen := got[rec.Topic]; seen {
				return
			}

			if recordSubject(rec) != wantSubject {
				return
			}

			got[rec.Topic] = rec
		})
	}

	return got
}

// recordSubject returns the ce-subject header value of a record, or "" when
// absent.
func recordSubject(rec *kgo.Record) string {
	for _, h := range rec.Headers {
		if h.Key == streamingITCESubject {
			return string(h.Value)
		}
	}

	return ""
}
