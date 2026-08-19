// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// TestMidazCatalogRoutesAssembly locks the catalog/routes assembly path under one
// topic per producing application: the catalog carries one entry per event, and
// the whole catalog is served by a SINGLE required catch-all route pointing at
// "lerian.streaming.ledger". Every event the ledger owns — ledger core, fees, CRM
// — rides that one stream.
func TestMidazCatalogRoutesAssembly(t *testing.T) {
	t.Parallel()

	defs := midazEventDefinitions()

	catalog, err := buildCatalog()
	require.NoError(t, err)

	routes, err := buildRoutes(streamingPrimaryTargetName, streamingServiceName)
	require.NoError(t, err)

	// One catalog entry per domain definition, plus the single shared
	// billing_recorded entry appended by buildCatalog (owned by lib-streaming's
	// billing package, not part of midazEventDefinitions). Routes from buildRoutes
	// remain domain-only; the billing route is wired via Builder.RouteOverrides.
	assert.Equal(t, len(defs)+1, catalog.Len(), "catalog = one entry per domain definition plus the shared billing entry")

	// One route, not one per event: the collapse is the contract. A route count
	// that tracks the definition count again would mean the per-event fan-out came
	// back, and with it 49 topics no consumer subscribes to.
	require.Len(t, routes, 1, "the ledger publishes through exactly one catch-all route")

	route := routes[0]
	assert.Empty(t, route.DefinitionKey,
		"an empty DefinitionKey is what makes the route a catch-all serving the whole catalog")
	assert.Equal(t, libStreaming.RouteRequired, route.Requirement,
		"the only route carrying every fact must be required, or a lost publish would report success")
	assert.Equal(t, streamingPrimaryTargetName, route.Target)

	wantTopic, err := libStreaming.AppTopic(streamingServiceName)
	require.NoError(t, err)
	assert.Equal(t, libStreaming.KafkaTopic(wantTopic), route.Destination,
		"the catch-all must target the application topic derived from the ce-source")

	// Literal lock on the derived name: the whole point of the collapse is that
	// this ONE string is the ledger's entire write surface (plus its .dlq).
	assert.Equal(t, "lerian.streaming.ledger", wantTopic)

	// No duplicate definition key in the registry.
	defKeys := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		key := def.Key()
		_, dup := defKeys[key]
		require.Falsef(t, dup, "duplicate definition key %q in midazEventDefinitions", key)
		defKeys[key] = struct{}{}
	}

	// Registration lock: an INDEPENDENT literal enumeration of every event key the
	// ledger publishes, deliberately NOT derived from the Definition vars or from
	// midazEventDefinitions (the code under test). Under a collapsed topic the
	// event key is the ONLY selector a consumer has, so a dropped registration or
	// a renamed key is a silent break of somebody's subscription — this map turns
	// either into a unit-test failure.
	expectedEventKeys := map[string]struct{}{
		// Ledger core (35).
		"organization.created":      {},
		"organization.updated":      {},
		"organization.deleted":      {},
		"ledger.created":            {},
		"ledger.updated":            {},
		"ledger.deleted":            {},
		"account.created":           {},
		"account.updated":           {},
		"account.deleted":           {},
		"asset.created":             {},
		"asset.updated":             {},
		"asset.deleted":             {},
		"portfolio.created":         {},
		"portfolio.updated":         {},
		"portfolio.deleted":         {},
		"segment.created":           {},
		"segment.updated":           {},
		"segment.deleted":           {},
		"operation_route.created":   {},
		"operation_route.updated":   {},
		"operation_route.deleted":   {},
		"transaction_route.created": {},
		"transaction_route.updated": {},
		"transaction_route.deleted": {},
		"balance.created":           {},
		"balance.changed":           {},
		"balance.config_changed":    {},
		"balance.deleted":           {},
		"balance.overdraft_drawn":   {},
		"balance.overdraft_repaid":  {},
		"balance.overdraft_cleared": {},
		"transaction.posted":        {},
		"transaction.committed":     {},
		"transaction.canceled":      {},
		"transaction.reverted":      {},
		// Fees (7).
		"fee_packages.created":         {},
		"fee_packages.updated":         {},
		"fee_packages.deleted":         {},
		"fee_billing_packages.created": {},
		"fee_billing_packages.updated": {},
		"fee_billing_packages.deleted": {},
		"fee_charge.applied":           {},
		// CRM (7).
		"holder.created":                   {},
		"holder.updated":                   {},
		"holder.deleted":                   {},
		"instrument.created":               {},
		"instrument.updated":               {},
		"instrument.deleted":               {},
		"instrument.related_party_deleted": {},
	}

	assert.Equal(t, len(expectedEventKeys), len(defKeys),
		"expectedEventKeys must enumerate every registered event exactly once")

	for key := range defKeys {
		_, ok := expectedEventKeys[key]
		assert.Truef(t, ok, "registered event %q missing from the independent expectedEventKeys map", key)
	}

	for key := range expectedEventKeys {
		_, ok := defKeys[key]
		assert.Truef(t, ok, "expectedEventKeys names %q, which is no longer registered", key)
	}
}

// TestFeesEventsRegistered locks the fee events into the assembled catalog: the
// fee package / billing-package keys plus fee_charge.applied must be a subset of the
// catalog keys, so a dropped fee registration is caught before it becomes a
// silent gap.
func TestFeesEventsRegistered(t *testing.T) {
	t.Parallel()

	expected := []string{
		"fee_packages.created",
		"fee_packages.updated",
		"fee_packages.deleted",
		"fee_billing_packages.created",
		"fee_billing_packages.updated",
		"fee_billing_packages.deleted",
		"fee_charge.applied",
	}

	catalog, err := buildCatalog()
	require.NoError(t, err)

	catalogKeys := make(map[string]struct{}, catalog.Len())
	for _, d := range catalog.Definitions() {
		catalogKeys[d.Key] = struct{}{}
	}

	for _, key := range expected {
		_, ok := catalogKeys[key]
		assert.True(t, ok, "fee event %q must be registered in the streaming catalog", key)
	}

	// Guard against the key strings drifting from the Definition vars.
	assert.Equal(t, "fee_packages.created", events.FeesPackageCreatedDefinition.Key())
	assert.Equal(t, "fee_billing_packages.deleted", events.FeesBillingPackageDeletedDefinition.Key())
	assert.Equal(t, "fee_charge.applied", events.FeesAppliedDefinition.Key())
}

// TestEventKeyConvergesWithEventDefinition proves midaz's Definition.Key() is the
// SAME string lib-streaming publishes as the dispatch selector
// (EventDefinition.EventKey) for every registered event.
//
// This replaced a topic-convergence test. Under one topic per application a
// definition has no topic of its own; what a consumer selects on inside the
// stream is this key, carried on the wire as ce-resourcetype / ce-eventtype and
// as the trailing two segments of ce-type. Underscores now survive end to end —
// the hyphen-folding both sides used to apply is gone — so the two forms can no
// longer drift apart.
func TestEventKeyConvergesWithEventDefinition(t *testing.T) {
	t.Parallel()

	for _, def := range midazEventDefinitions() {
		entry := libStreaming.EventDefinition{
			Key:           def.Key(),
			ResourceType:  def.ResourceType,
			EventType:     def.EventType,
			SchemaVersion: def.SchemaVersion,
		}

		assert.Equalf(t, def.Key(), entry.EventKey(),
			"Definition.Key and EventDefinition.EventKey must converge for %q", def.Key())
	}

	// The shared mapper is the single place a Definition becomes a catalog entry;
	// lock that it carries the key through unchanged.
	entries := pkgStreaming.CatalogEntriesFromDefinitions(midazEventDefinitions())
	require.Len(t, entries, len(midazEventDefinitions()))

	for i, def := range midazEventDefinitions() {
		assert.Equal(t, def.Key(), entries[i].Key)
		assert.Equal(t, def.Key(), entries[i].EventKey())
	}
}
