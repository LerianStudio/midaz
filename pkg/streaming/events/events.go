// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package events holds the typed wire-format definitions for every
// lib-streaming event emitted by midaz. This package is the canonical
// source of truth for the wire contract of every event the service
// publishes — emit sites, tests, and downstream consumers all read from
// here. Each event has its own file with:
//
//   - a package-level Definition var capturing ResourceType, EventType,
//     SchemaVersion;
//   - a typed Payload struct describing the wire JSON;
//   - a constructor New<Event>(domain) <Event>Payload that maps the
//     persisted domain object into the wire payload (the place for PII
//     redaction, derived fields, and contract-locked defaults);
//   - a ToEmitRequest method that assembles a fully-populated
//     streaming.EmitRequest ready for Emit. CloudEvents envelope fields
//     (Source, ResourceType, EventType, SchemaVersion) are NOT carried on
//     the request — Source flows from the Builder at construction time;
//     the other three resolve from the Catalog by DefinitionKey at emit
//     time.
//
// Design intent: the wire contract is INTENTIONALLY decoupled from the
// domain model. mmodel.Account can evolve freely; the AccountCreatedPayload
// stays pinned to its contract until a v2 schema is published. Drift is a
// feature, not a bug.
//
// One file per event key. Group by aggregate (e.g. account_*.go) if the
// file count becomes unwieldy.
package events

// Definition captures the routing constants for a single event type:
//
//   - ResourceType  — the aggregate this event belongs to (e.g. "account")
//   - EventType     — the past-tense action (e.g. "created")
//   - SchemaVersion — semver of the wire payload contract
//
// Held as a package-level var per event so emit sites and tests share the
// same constant references rather than scattered string literals.
type Definition struct {
	ResourceType  string
	EventType     string
	SchemaVersion string
}

// Key returns the canonical event key for this definition. Composed as
// "<ResourceType>.<EventType>" (e.g. "account.created").
func (d Definition) Key() string {
	return d.ResourceType + "." + d.EventType
}

// Deletion-type values carried on every *.deleted event. "soft" sets DeletedAt
// and retains the record; "hard" purges it. Consumers branch on this to decide
// retention/erasure semantics. Package-neutral so any aggregate's deleted event
// shares the same wire literals.
const (
	deletionTypeSoft = "soft"
	deletionTypeHard = "hard"
)
