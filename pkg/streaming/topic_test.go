// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming_test

import (
	"testing"

	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/stretchr/testify/assert"
)

// TestTopicName locks the {service}.{resource}.{event} topic grammar: the
// service is the first segment and the underscore-canonical DefinitionKey
// ("<resource>.<event>") supplies the remaining two. No prefix, no fold — the
// key is emitted verbatim after the service segment.
func TestTopicName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		service string
		key     string
		want    string
	}{
		{
			name:    "ledger single-word segments",
			service: "ledger",
			key:     "balance.changed",
			want:    "ledger.balance.changed",
		},
		{
			name:    "ledger underscore resource",
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
			name:    "ledger overdraft event segment",
			service: "ledger",
			key:     "balance.overdraft_drawn",
			want:    "ledger.balance.overdraft_drawn",
		},
		{
			name:    "crm multi-word event segment",
			service: "crm",
			key:     "alias.related_party_deleted",
			want:    "crm.alias.related_party_deleted",
		},
		{
			name:    "crm single-word segments",
			service: "crm",
			key:     "holder.created",
			want:    "crm.holder.created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, pkgStreaming.TopicName(tt.service, tt.key))
		})
	}
}
