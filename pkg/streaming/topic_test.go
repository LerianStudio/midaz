// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming_test

import (
	"regexp"
	"testing"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/stretchr/testify/assert"
)

// hubTopicGrammar mirrors the streaming-hub ingest consumer subscription:
// exactly two [a-z0-9_] segments after the prefix, no hyphen.
var hubTopicGrammar = regexp.MustCompile(`^lerian\.streaming\.[a-z0-9_]+\.[a-z0-9_]+$`)

// TestTopicName locks the service-folding + hyphen-to-underscore transform that
// keeps wire topic names inside the streaming-hub consumer regex
// (^lerian.streaming.<seg>.<seg>$ over [a-z0-9_]) while route keys / ce-type
// stay hyphenated. It also locks the leading-"<service>-" strip that keeps
// fee-folded topics from becoming "fee_fee_*".
func TestTopicName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		service string
		key     string
		want    string
	}{
		{
			name:    "ledger no-hyphen key",
			service: "ledger",
			key:     "balance.changed",
			want:    "lerian.streaming.ledger_balance.changed",
		},
		{
			name:    "ledger single-hyphen resource",
			service: "ledger",
			key:     "operation-route.created",
			want:    "lerian.streaming.ledger_operation_route.created",
		},
		{
			name:    "ledger hyphen in event segment",
			service: "ledger",
			key:     "balance.config-changed",
			want:    "lerian.streaming.ledger_balance.config_changed",
		},
		{
			name:    "ledger account created no-op proof",
			service: "ledger",
			key:     "account.created",
			want:    "lerian.streaming.ledger_account.created",
		},
		{
			name:    "ledger created resource equals service no-op proof",
			service: "ledger",
			key:     "ledger.created",
			want:    "lerian.streaming.ledger_ledger.created",
		},
		{
			name:    "crm multi-hyphen event segment",
			service: "crm",
			key:     "alias.related-party-deleted",
			want:    "lerian.streaming.crm_alias.related_party_deleted",
		},
		{
			name:    "crm no-hyphen key",
			service: "crm",
			key:     "holder.created",
			want:    "lerian.streaming.crm_holder.created",
		},
		{
			name:    "crm instrument multi-hyphen event no-op proof",
			service: "crm",
			key:     "instrument.related-party-deleted",
			want:    "lerian.streaming.crm_instrument.related_party_deleted",
		},
		{
			name:    "fee packages created strips leading service prefix",
			service: "fee",
			key:     "fee-packages.created",
			want:    "lerian.streaming.fee_packages.created",
		},
		{
			name:    "fee packages updated strips leading service prefix",
			service: "fee",
			key:     "fee-packages.updated",
			want:    "lerian.streaming.fee_packages.updated",
		},
		{
			name:    "fee packages deleted strips leading service prefix",
			service: "fee",
			key:     "fee-packages.deleted",
			want:    "lerian.streaming.fee_packages.deleted",
		},
		{
			name:    "fee billing-packages created strips leading service prefix",
			service: "fee",
			key:     "fee-billing-packages.created",
			want:    "lerian.streaming.fee_billing_packages.created",
		},
		{
			name:    "fee charge applied strips leading service prefix",
			service: "fee",
			key:     "fee-charge.applied",
			want:    "lerian.streaming.fee_charge.applied",
		},
		{
			name:    "tracer rule created no strip",
			service: "tracer",
			key:     "rule.created",
			want:    "lerian.streaming.tracer_rule.created",
		},
		{
			name:    "tracer limit deactivated no strip",
			service: "tracer",
			key:     "limit.deactivated",
			want:    "lerian.streaming.tracer_limit.deactivated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := pkgStreaming.TopicName(tt.service, tt.key)
			assert.Equal(t, tt.want, got)
			assert.Regexp(t, hubTopicGrammar, got,
				"topic must match the two-segment streaming-hub grammar")
		})
	}
}

// TestTopicPrefix pins the exported prefix constant so callers that build or
// assert topic names against it cannot drift from the wire contract.
func TestTopicPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "lerian.streaming.", pkgStreaming.TopicPrefix)
}
