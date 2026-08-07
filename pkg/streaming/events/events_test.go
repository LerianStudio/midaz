// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events_test

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
)

func TestDefinition_RouteKey_and_Key(t *testing.T) {
	tests := []struct {
		name         string
		definition   events.Definition
		wantKey      string
		wantRouteKey string
	}{
		{
			name:         "underscored resource folds to hyphen in route key",
			definition:   events.Definition{ResourceType: "operation_route", EventType: "created", SchemaVersion: "1.0.0"},
			wantKey:      "operation_route.created",
			wantRouteKey: "operation-route.created",
		},
		{
			name:         "underscored event folds to hyphen in route key",
			definition:   events.Definition{ResourceType: "balance", EventType: "config_changed", SchemaVersion: "1.0.0"},
			wantKey:      "balance.config_changed",
			wantRouteKey: "balance.config-changed",
		},
		{
			name:         "hyphen-free key leaves route key unchanged",
			definition:   events.Definition{ResourceType: "account", EventType: "created", SchemaVersion: "1.0.0"},
			wantKey:      "account.created",
			wantRouteKey: "account.created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantKey, tt.definition.Key())
			assert.Equal(t, tt.wantRouteKey, tt.definition.RouteKey())
		})
	}
}
