// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming_test

import (
	"testing"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/stretchr/testify/assert"
)

// TestTopicName locks the service-folding + hyphen-to-underscore transform that
// keeps wire topic names inside the streaming-hub consumer regex
// (^lerian.streaming.<seg>.<seg>$ over [a-z0-9_]) while route keys / ce-type
// stay hyphenated.
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, pkgStreaming.TopicName(tt.service, tt.key))
		})
	}
}

// TestTopicPrefix pins the exported prefix constant so callers that build or
// assert topic names against it cannot drift from the wire contract.
func TestTopicPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "lerian.streaming.", pkgStreaming.TopicPrefix)
}
