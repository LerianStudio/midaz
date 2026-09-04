//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// This smoke test exercises the REAL ledger fee-streaming path end-to-end: it
// builds the producer via BuildStreamingEmitter, emits all 7 fee events through
// pkgStreaming.EmitBrokerBestEffort with the real event constructors, then consumes
// them back off Kafka with a franz-go consumer and asserts the CloudEvents
// binary-mode headers (ce-type, ce-source, ce-subject, ce-tenantid) plus the
// absence of fee-detail / PII keys on every body.
//
// It uses the Kafka-compatible broker supplied via STREAMING_BROKERS when set.
// Otherwise it starts an in-process protocol broker so the required integration
// shard always executes the capability instead of silently skipping it.
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

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// errCodeTopicAlreadyExists is the Kafka TOPIC_ALREADY_EXISTS error code,
// tolerated so a broker with the topics pre-created is a no-op.
const errCodeTopicAlreadyExists int16 = 36

const (
	streamingITCEType    = "ce-type"
	streamingITCESource  = "ce-source"
	streamingITCESubject = "ce-subject"
	streamingITCETenant  = "ce-tenantid"

	// streamingITSource is the ledger's roster name — a single dot-free lowercase
	// segment, which is the only shape lib-streaming accepts as a ce-source. Every
	// event below rides the one topic derived from it.
	streamingITSource = "ledger"
)

// streamingITCETypeFor is the ce-type header lib-streaming stamps for one event,
// composed through the library's own facade rather than by re-spelling the prefix
// and separators here: the reverse-DNS namespace, the PRODUCING APPLICATION, then
// the resource and event types. The application segment is what stops two services
// emitting a byte-identical ce-type for same-named events — a homonym collision a
// consumer reading only ce-type could not detect, and one that a single shared
// topic per application makes reachable in practice.
func streamingITCETypeFor(resourceType, eventType string) string {
	return libStreaming.CloudEventsType(streamingITSource, resourceType, eventType)
}

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
// closure, the ce-type it must carry, and the aggregate id that must appear as
// ce-subject.
//
// There is no per-event topic any more: all seven fee events ride the ledger's
// single application topic, so the pair (ce-type, ce-subject) is what identifies a
// record inside the stream. Both halves are load-bearing — created and updated on
// the same package share a ce-subject and differ only in ce-type.
type streamingITExpectation struct {
	name       string
	ceType     string
	subject    string
	emitReq    func(tenantID string) (libStreaming.EmitRequest, error)
	requireKey []string // keys that MUST be present in the body
}

// TestStreamingEmitter_Integration_AllSevenFeeEvents emits every fee event
// through the real BuildStreamingEmitter + EmitBrokerBestEffort path and asserts the
// wire contract (ce-type / ce-source / ce-subject / ce-tenantid + fee-detail
// absence) per event.
func TestStreamingEmitter_Integration_AllSevenFeeEvents(t *testing.T) {
	ctx := context.Background()
	brokers := streamingITBrokers(t)
	brokersEnv := strings.Join(brokers, ",")

	expectations := streamingITExpectations()

	// Pre-create the ledger's application topic and its DLQ so the test never
	// depends on broker auto-create. Under one topic per producing application
	// these two names are the ledger's whole write surface.
	appTopic, err := libStreaming.AppTopic(streamingITSource)
	require.NoError(t, err)

	dlqTopic, err := libStreaming.AppDLQTopic(streamingITSource)
	require.NoError(t, err)

	createTopics(t, ctx, brokers, []string{appTopic, dlqTopic})

	// Build the emitter through the REAL bootstrap path. LoadConfig reads
	// STREAMING_BROKERS / STREAMING_CLOUDEVENTS_SOURCE from env; the stamped
	// ce-source, however, comes from resolveStreamingSource(cfg), whose
	// StreamingCloudEventsSource field is env-parsed in production
	// (InitServersWithOptions), so the test populates it the same way.
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", brokersEnv)
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", streamingITSource)

	cfg := &Config{StreamingEnabled: true, StreamingCloudEventsSource: streamingITSource}

	emitter, closeFn, err := BuildStreamingEmitter(ctx, cfg, libLog.NewNop(), nil)
	require.NoError(t, err)
	require.NotNil(t, emitter)

	t.Cleanup(func() { _ = closeFn() })

	// Emit all 7 through the IMPORTANT-posture helper (the same call the use
	// cases make). IMPORTANT never propagates errors, so failures surface via
	// the consumer timing out on a missing record below.
	for _, e := range expectations {
		pkgStreaming.EmitBrokerBestEffort(ctx, nil, libLog.NewNop(), emitter, e.ceType, e.emitReq)
	}

	// Consume the one application topic and assert the wire contract per event.
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(appTopic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	require.NoError(t, err)

	t.Cleanup(client.Close)

	// Match only records whose ce-subject is THIS run's freshly-generated id for
	// the ce-type. Stale records from a prior run carry different UUIDs and are
	// skipped, so a persistent broker cannot mask a later regression.
	wantSubjectByCEType := make(map[string]string, len(expectations))
	for _, e := range expectations {
		wantSubjectByCEType[e.ceType] = e.subject
	}

	got := drainOnePerCEType(t, ctx, client, wantSubjectByCEType)

	for _, e := range expectations {
		rec, ok := got[e.ceType]
		require.Truef(t, ok, "no record with ce-type %q consumed from %q", e.ceType, appTopic)

		assert.Equalf(t, appTopic, rec.Topic,
			"%s: every fact must ride the ledger application topic", e.name)

		assertRecord(t, e, rec)
	}
}

func streamingITBrokers(t *testing.T) []string {
	t.Helper()

	if configured := strings.TrimSpace(os.Getenv("STREAMING_BROKERS")); configured != "" {
		return strings.Split(configured, ",")
	}

	cluster, err := kfake.NewCluster(kfake.NumBrokers(1))
	require.NoError(t, err)
	t.Cleanup(cluster.Close)
	return cluster.ListenAddrs()
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
			name:       "fee_packages.created",
			ceType:     streamingITCETypeFor(events.FeesPackageCreatedDefinition.ResourceType, events.FeesPackageCreatedDefinition.EventType),
			subject:    packageID,
			requireKey: []string{"id", "organizationId", "ledgerId", "enable", "createdAt", "updatedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesPackageCreated(packageID, orgID, ledgerID, &segmentID, &transactionRoute, true, streamingITFixedTime, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			name:       "fee_packages.updated",
			ceType:     streamingITCETypeFor(events.FeesPackageUpdatedDefinition.ResourceType, events.FeesPackageUpdatedDefinition.EventType),
			subject:    packageID,
			requireKey: []string{"id", "organizationId", "ledgerId", "enable", "createdAt", "updatedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesPackageUpdated(packageID, orgID, ledgerID, &segmentID, &transactionRoute, true, streamingITFixedTime, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			name:       "fee_packages.deleted",
			ceType:     streamingITCETypeFor(events.FeesPackageDeletedDefinition.ResourceType, events.FeesPackageDeletedDefinition.EventType),
			subject:    packageID,
			requireKey: []string{"id", "organizationId", "ledgerId", "deletedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesPackageDeleted(packageID, orgID, ledgerID, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			name:       "fee_billing_packages.created",
			ceType:     streamingITCETypeFor(events.FeesBillingPackageCreatedDefinition.ResourceType, events.FeesBillingPackageCreatedDefinition.EventType),
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
			name:       "fee_billing_packages.updated",
			ceType:     streamingITCETypeFor(events.FeesBillingPackageUpdatedDefinition.ResourceType, events.FeesBillingPackageUpdatedDefinition.EventType),
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
			name:       "fee_billing_packages.deleted",
			ceType:     streamingITCETypeFor(events.FeesBillingPackageDeletedDefinition.ResourceType, events.FeesBillingPackageDeletedDefinition.EventType),
			subject:    billingID,
			requireKey: []string{"id", "organizationId", "ledgerId", "deletedAt"},
			emitReq: func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewFeesBillingPackageDeleted(billingID, orgID, ledgerID, streamingITFixedTime).
					ToEmitRequest(tenantID, streamingITFixedTime)
			},
		},
		{
			// ce-subject for fee_charge.applied is the TRANSACTION id, not a package id.
			name:       "fee_charge.applied",
			ceType:     streamingITCETypeFor(events.FeesAppliedDefinition.ResourceType, events.FeesAppliedDefinition.EventType),
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

// drainOnePerCEType polls the single application topic until it has captured, for
// every expected ce-type, the record whose ce-subject matches this run's expected
// subject (or the context deadline fires).
//
// Selection moved from the broker into the consumer with the topic collapse: one
// subscription now delivers the producer's entire stream, so a record is
// identified by (ce-type, ce-subject) rather than by the topic it arrived on.
// Records carrying an unexpected ce-type, or a ce-subject from a prior run on a
// persistent broker, are skipped and cannot mask a regression.
func drainOnePerCEType(t *testing.T, ctx context.Context, client *kgo.Client, wantSubjectByCEType map[string]string) map[string]*kgo.Record {
	t.Helper()

	pollCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	want := len(wantSubjectByCEType)
	got := map[string]*kgo.Record{}

	for len(got) < want {
		fetches := client.PollFetches(pollCtx)
		if err := pollCtx.Err(); err != nil {
			t.Fatalf("timed out consuming events: got %d of %d expected ce-types: %v", len(got), want, err)
		}

		if errs := fetches.Errors(); len(errs) > 0 {
			t.Fatalf("fetch errors: %+v", errs)
		}

		fetches.EachRecord(func(rec *kgo.Record) {
			ceType := recordHeader(rec, streamingITCEType)

			wantSubject, tracked := wantSubjectByCEType[ceType]
			if !tracked {
				return
			}

			if _, seen := got[ceType]; seen {
				return
			}

			if recordHeader(rec, streamingITCESubject) != wantSubject {
				return
			}

			got[ceType] = rec
		})
	}

	return got
}

// recordHeader returns the value of a record header by key, or "" when absent.
func recordHeader(rec *kgo.Record, key string) string {
	for _, h := range rec.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}

	return ""
}
