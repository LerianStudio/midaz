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
// a first (service) segment [a-z0-9][a-z0-9-]* — hyphens allowed because the
// hub also accepts lib-streaming-derived source segments — then the two LAST
// segments (resource, event) over [a-z0-9_] with no hyphen, and an optional
// ".vN" schema-major suffix.
var hubTopicGrammar = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.[a-z0-9_]+\.[a-z0-9_]+(\.v[0-9]+)?$`)

// TestTopicName locks the ACL-prefix grammar produced by TopicName:
// "{sanitize(service)}.{resource}.{event}" where the key is the
// underscore-canonical Definition.Key(). The prefix "lerian.streaming." and the
// service-fold-into-first-segment are gone; the service is its own leading
// segment so a Kafka ACL granted on "{service}." matches every topic the
// service emits.
func TestTopicName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		service string
		key     string
		want    string
	}{
		{
			name:    "ledger no-underscore key",
			service: "ledger",
			key:     "balance.changed",
			want:    "ledger.balance.changed",
		},
		{
			name:    "ledger underscore resource segment",
			service: "ledger",
			key:     "operation_route.created",
			want:    "ledger.operation_route.created",
		},
		{
			name:    "ledger underscore event segment",
			service: "ledger",
			key:     "balance.config_changed",
			want:    "ledger.balance.config_changed",
		},
		{
			name:    "ledger account created",
			service: "ledger",
			key:     "account.created",
			want:    "ledger.account.created",
		},
		{
			name:    "ledger folded fee resource",
			service: "ledger",
			key:     "fee_packages.created",
			want:    "ledger.fee_packages.created",
		},
		{
			name:    "ledger folded fee billing resource",
			service: "ledger",
			key:     "fee_billing_packages.created",
			want:    "ledger.fee_billing_packages.created",
		},
		{
			name:    "ledger folded fee charge",
			service: "ledger",
			key:     "fee_charge.applied",
			want:    "ledger.fee_charge.applied",
		},
		{
			name:    "ledger crm instrument multi-word event",
			service: "ledger",
			key:     "instrument.related_party_deleted",
			want:    "ledger.instrument.related_party_deleted",
		},
		{
			name:    "tracer rule created",
			service: "tracer",
			key:     "rule.created",
			want:    "tracer.rule.created",
		},
		{
			name:    "tracer limit deactivated",
			service: "tracer",
			key:     "limit.deactivated",
			want:    "tracer.limit.deactivated",
		},
		{
			name:    "service is lowercased",
			service: "Ledger",
			key:     "balance.changed",
			want:    "ledger.balance.changed",
		},
		{
			name:    "service non-alphanumerics dropped",
			service: "lerian.midaz-ledger",
			key:     "balance.changed",
			want:    "lerianmidazledger.balance.changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := pkgStreaming.TopicName(tt.service, tt.key)
			assert.Equal(t, tt.want, got)
			assert.Regexp(t, hubTopicGrammar, got,
				"topic must match the streaming-hub ingest grammar")
		})
	}
}
