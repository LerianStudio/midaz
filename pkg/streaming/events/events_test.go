// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
)

// TestDefinition_Key locks the canonical "<resource>.<event>" event key. The key
// is the catalog DefinitionKey, the consumer-facing dispatch selector, and the
// two trailing segments of the ce-type header — it is underscore-preserving in
// all three places, so an underscored resource or event type reaches the wire
// verbatim and is never folded to hyphens.
func TestDefinition_Key(t *testing.T) {
	tests := []struct {
		name       string
		definition events.Definition
		wantKey    string
	}{
		{
			name:       "underscored resource is preserved",
			definition: events.Definition{ResourceType: "operation_route", EventType: "created", SchemaVersion: "1.0.0"},
			wantKey:    "operation_route.created",
		},
		{
			name:       "underscored event type is preserved",
			definition: events.Definition{ResourceType: "balance", EventType: "config_changed", SchemaVersion: "1.0.0"},
			wantKey:    "balance.config_changed",
		},
		{
			name:       "underscore-free key is unchanged",
			definition: events.Definition{ResourceType: "account", EventType: "created", SchemaVersion: "1.0.0"},
			wantKey:    "account.created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantKey, tt.definition.Key())
		})
	}
}
