// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tracerreservation defines the Ledger's durable coordination with
// Tracer. It carries accounting identities and facts, never accounting execution.
package tracerreservation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type Key struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	TransactionID  uuid.UUID
}

func (k Key) Validate() error {
	if k.OrganizationID == uuid.Nil || k.LedgerID == uuid.Nil || k.TransactionID == uuid.Nil {
		return constant.ErrInvalidRequestBody
	}

	return nil
}

type Config struct {
	Bounds       tracercontract.Limits
	MaxBodyBytes int
}

func (c Config) Validate() error {
	if c.MaxBodyBytes <= 0 {
		return constant.ErrInvalidRequestBody
	}

	return c.Bounds.Validate()
}

// Intent is persisted before any network Reserve. Payload is the frozen request;
// recovery never reloads current facts/settings to construct another evaluation.
// PrepareDeadline fences a request which never acquired permission to dispatch
// accounting. Once Executing, expiry is not evidence of an accounting abort.
type Intent struct {
	Key             Key
	ExecutionID     uuid.UUID
	Scope           tracercontract.ReserveScope
	Fingerprint     [sha256.Size]byte
	Payload         []byte
	CreatedAt       time.Time
	PrepareDeadline time.Time
}

func NewIntent(ctx context.Context, key Key, executionID uuid.UUID, scope tracercontract.ReserveScope, request tracercontract.ReserveRequest, createdAt, deadline time.Time, cfg Config) (Intent, error) {
	if err := ctx.Err(); err != nil {
		return Intent{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Intent{}, err
	}

	fingerprint, err := request.Fingerprint(ctx, scope, cfg.Bounds)
	if err != nil {
		return Intent{}, err
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return Intent{}, err
	}

	intent := Intent{Key: key, ExecutionID: executionID, Scope: scope, Fingerprint: fingerprint, Payload: payload, CreatedAt: createdAt.UTC().Truncate(time.Microsecond), PrepareDeadline: deadline.UTC().Truncate(time.Microsecond)}
	if err := intent.Validate(ctx, cfg); err != nil {
		return Intent{}, err
	}

	return intent, nil
}

func (i Intent) Validate(ctx context.Context, cfg Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	if err := i.Key.Validate(); err != nil {
		return err
	}

	if i.ExecutionID == uuid.Nil || i.CreatedAt.IsZero() || !i.PrepareDeadline.After(i.CreatedAt) {
		return constant.ErrInvalidRequestBody
	}

	request, err := i.Request(ctx, cfg)
	if err != nil {
		return err
	}

	if request.TransactionID != i.Key.TransactionID || request.ContextID != i.Key.LedgerID.String() {
		return constant.ErrInvalidRequestBody
	}

	hash, err := request.Fingerprint(ctx, i.Scope, cfg.Bounds)
	if err != nil {
		return err
	}

	if hash != i.Fingerprint {
		return constant.ErrInvalidRequestBody
	}

	return nil
}

func (i Intent) Request(ctx context.Context, cfg Config) (tracercontract.ReserveRequest, error) {
	return tracercontract.DecodeReserveJSON(ctx, i.Payload, cfg.MaxBodyBytes, cfg.Bounds)
}

type State string

const (
	Prepared  State = "PREPARED"
	Executing State = "EXECUTING"
	Confirmed State = "CONFIRMED"
	Released  State = "RELEASED"
)

// CanTransition validates a known outcome supplied by the producer. It does not
// infer that outcome from timeouts, absent receipts or a failed network request.
// Reacquiring execution is forbidden: replay may settle, never rerun accounting.
func (s State) CanTransition(to State) bool {
	switch s {
	case Prepared:
		return to == Executing || to == Released
	case Executing:
		return to == Confirmed || to == Released
	case Confirmed, Released:
		return to == s
	default:
		return false
	}
}

func (s State) Terminal() bool { return s == Confirmed || s == Released }

// Record retains the intent after delivery for replay/conflict diagnosis. No
// cleanup or deployment rollback may discard unresolved obligations.
type Record struct {
	Intent      Intent
	State       State
	UpdatedAt   time.Time
	DeliveredAt *time.Time
}

// Pending is enough to reconcile/deliver an outcome without loading the frozen
// body or applying current admission bounds to an older obligation.
type Pending struct {
	CreatedAt        time.Time
	Key              Key
	ExecutionID      uuid.UUID
	Scope            tracercontract.ReserveScope
	ContractRevision string
	State            State
	PrepareDeadline  time.Time
}
