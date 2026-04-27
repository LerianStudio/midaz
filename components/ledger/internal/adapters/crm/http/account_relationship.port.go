// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package crmhttp contains the Ledger-side HTTP adapter for reaching the CRM service's
// public orchestration endpoints. Its shape is governed by the account-registration
// saga (see pkg/mmodel/account_registration.go and the CRM/Ledger abstraction layer
// design document): Ledger is the orchestrator and CRM is a downstream resource.
//
// This package exposes an interface (CRMAccountRelationshipPort) and a Postgres-agnostic
// HTTP client that satisfies it. The client targets CRM's hardened "/v1/*" paths. The
// interface lets the saga orchestrator and tests depend on a mock without pulling in the
// HTTP transport.
//
// Auth is carried as a service-scoped Bearer token (M2M JWT) threaded into each call by
// the saga. Timeouts and circuit-breaker thresholds are code constants here, not env
// vars: they become configuration knobs only after ops proves a need.
package crmhttp

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CRMAccountRelationshipPort is the Ledger-side contract for reaching CRM. It is the
// only surface the saga orchestrator depends on; every concrete implementation (real
// HTTP client, test double, recording mock) must satisfy this interface.
//
// Semantics:
//   - All methods take a context.Context first; callers propagate tenant, deadline, and
//     trace context via ctx.
//   - organizationID is passed as a string because CRM receives it via a header rather
//     than as a typed parameter. The caller is responsible for validating its format.
//   - Methods that mutate CRM state (CreateAccountAlias, CloseAlias) accept an
//     idempotency key to make replayed calls safe.
//   - token is the service-scoped Bearer token used to authenticate against CRM. The
//     saga obtains it from the incoming request context (Authorization header in the
//     public endpoint path) and threads it through unchanged.
//   - Errors are returned as business errors (pkg.ValidateBusinessError wrapping a
//     constant.Err* sentinel) so callers can use errors.Is for control flow. See the
//     client implementation for the HTTP-to-sentinel mapping table.
type CRMAccountRelationshipPort interface {
	// GetHolder fetches a holder's metadata from CRM. Returns a business error wrapping
	// constant.ErrHolderNotFound on HTTP 404 and constant.ErrCRMTransient on any retryable
	// failure (5xx, timeout, connection refused, circuit-breaker open).
	GetHolder(ctx context.Context, organizationID string, holderID uuid.UUID, token string) (*mmodel.Holder, error)

	// CreateAccountAlias registers a Ledger account as an alias under the given holder.
	// The idempotencyKey must match the one the saga generated for this registration
	// attempt; CRM treats repeated calls with the same key+payload as a no-op and
	// returns the previously-created alias. A repeated call with the same key but a
	// DIFFERENT payload surfaces constant.ErrAliasHolderConflict.
	CreateAccountAlias(ctx context.Context, organizationID string, holderID uuid.UUID, input *mmodel.CreateAliasInput, idempotencyKey, token string) (*mmodel.Alias, error)

	// GetAliasByAccount resolves the CRM alias associated with a given Ledger account,
	// used by the saga during recovery to determine whether a prior attempt already
	// created the alias (in which case we treat the CRM side as done and proceed).
	// Returns (nil, ErrAliasNotFound) when no alias exists for the account.
	GetAliasByAccount(ctx context.Context, organizationID, ledgerID, accountID, token string) (*mmodel.Alias, error)

	// CloseAlias marks an alias as closed on CRM. Used during compensation (rollback
	// path). The idempotencyKey makes the compensation safely retryable.
	CloseAlias(ctx context.Context, organizationID string, holderID, aliasID uuid.UUID, idempotencyKey, token string) error
}
