// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"fmt"

	"github.com/google/uuid"
)

const (
	TracerOutcomeVersion = 1

	TracerOutcomePrepared    = "PREPARED"
	TracerOutcomePendingHeld = "PENDING_HELD"
	TracerOutcomeCommitted   = "COMMITTED"
	TracerOutcomeAborted     = "ABORTED"
	TracerOutcomeDelivered   = "DELIVERED"

	TracerOutcomeEconomicPhasePendingHold = "PENDING_HOLD"
)

// TracerOutcomeRecord is the durable Ledger-owned delivery projection of the
// balance Lua's economic fact. EconomicOutcome is populated by that same Lua
// command; it is never reconstructed by the dispatcher.
type TracerOutcomeRecord struct {
	Version             int                      `json:"version"`
	TransactionID       uuid.UUID                `json:"transaction_id"`
	OutcomeID           uuid.UUID                `json:"outcome_id"`
	OrganizationID      uuid.UUID                `json:"organization_id"`
	LedgerID            uuid.UUID                `json:"ledger_id"`
	State               string                   `json:"state"`
	Owner               string                   `json:"owner"`
	EconomicPlanVersion string                   `json:"economic_plan_version"`
	EconomicPlanDigest  string                   `json:"economic_plan_digest"`
	EconomicPhase       string                   `json:"economic_phase,omitempty"`
	EconomicOutcome     *BalanceExecutionOutcome `json:"economic_outcome,omitempty"`
	PreparedAtUnixMS    int64                    `json:"prepared_at_unix_ms"`
	UpdatedAtUnixMS     int64                    `json:"updated_at_unix_ms"`
	DeliveryAttempts    int                      `json:"delivery_attempts"`
	LastError           string                   `json:"last_error,omitempty"`
}

func (r TracerOutcomeRecord) Terminal() bool {
	return r.State == TracerOutcomeCommitted || r.State == TracerOutcomeAborted || r.State == TracerOutcomeDelivered
}

func (r TracerOutcomeRecord) Validate() error {
	if !r.identityComplete() {
		return fmt.Errorf("incomplete tracer outcome record")
	}

	if r.EconomicPhase != "" && r.EconomicPhase != TracerOutcomeEconomicPhasePendingHold {
		return fmt.Errorf("invalid tracer outcome economic phase %q", r.EconomicPhase)
	}

	switch r.State {
	case TracerOutcomePrepared:
		if r.EconomicOutcome != nil {
			return fmt.Errorf("prepared tracer outcome already carries an economic result")
		}
	case TracerOutcomePendingHeld, TracerOutcomeCommitted, TracerOutcomeAborted, TracerOutcomeDelivered:
		if !r.economicResultMatches() {
			return fmt.Errorf("tracer outcome differs from its economic result")
		}
	default:
		return fmt.Errorf("invalid tracer outcome state %q", r.State)
	}

	return nil
}

func (r TracerOutcomeRecord) identityComplete() bool {
	return r.Version == TracerOutcomeVersion && r.TransactionID != uuid.Nil && r.OutcomeID != uuid.Nil &&
		r.OrganizationID != uuid.Nil && r.LedgerID != uuid.Nil && r.Owner != "" &&
		r.EconomicPlanVersion != "" && r.EconomicPlanDigest != "" &&
		r.PreparedAtUnixMS > 0 && r.UpdatedAtUnixMS > 0
}

func (r TracerOutcomeRecord) economicResultMatches() bool {
	if r.EconomicOutcome == nil || r.EconomicOutcome.Identity != r.TransactionID ||
		r.EconomicOutcome.Owner != r.Owner ||
		r.EconomicOutcome.EconomicPlanVersion != r.EconomicPlanVersion ||
		r.EconomicOutcome.EconomicPlanDigest != r.EconomicPlanDigest {
		return false
	}

	if r.State == TracerOutcomeAborted {
		return r.EconomicOutcome.Outcome == TransactionOutcomeAborted
	}

	if r.State == TracerOutcomePendingHeld || r.State == TracerOutcomeCommitted {
		return r.EconomicOutcome.Outcome == TransactionOutcomeCommitted
	}

	return true
}
