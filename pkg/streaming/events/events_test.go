// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
)

// TestDefinition_RouteKey locks the underscore->hyphen fold RouteKey() applies
// to the canonical Key(). Key() and the ce-type keep the underscore-canonical
// form; only the RouteDefinition.Key uses the hyphen form, because
// lib-streaming's route-key grammar rejects underscores.
func TestDefinition_RouteKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		def          events.Definition
		wantKey      string
		wantRouteKey string
	}{
		{
			name:         "underscore resource folds to hyphen route key",
			def:          events.Definition{ResourceType: "operation_route", EventType: "created", SchemaVersion: "1.0.0"},
			wantKey:      "operation_route.created",
			wantRouteKey: "operation-route.created",
		},
		{
			name:         "underscore event segment folds to hyphen route key",
			def:          events.Definition{ResourceType: "balance", EventType: "config_changed", SchemaVersion: "1.0.0"},
			wantKey:      "balance.config_changed",
			wantRouteKey: "balance.config-changed",
		},
		{
			name:         "multi-underscore event segment folds every underscore",
			def:          events.Definition{ResourceType: "alias", EventType: "related_party_deleted", SchemaVersion: "1.0.0"},
			wantKey:      "alias.related_party_deleted",
			wantRouteKey: "alias.related-party-deleted",
		},
		{
			name:         "no underscore is a no-op",
			def:          events.Definition{ResourceType: "account", EventType: "created", SchemaVersion: "1.0.0"},
			wantKey:      "account.created",
			wantRouteKey: "account.created",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.wantKey, tt.def.Key())
			assert.Equal(t, tt.wantRouteKey, tt.def.RouteKey())
		})
	}
}
