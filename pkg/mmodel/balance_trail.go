// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BalanceTrailPair is one balance row next to the head of its operation trail.
// Trail is nil when the balance has no balance-affecting operation.
type BalanceTrailPair struct {
	BalanceID      uuid.UUID
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	AccountID      uuid.UUID
	Version        int64
	Available      decimal.Decimal
	OnHold         decimal.Decimal
	UpdatedAt      time.Time
	Trail          *BalanceTrailHead
}

// BalanceTrailHead is the highest-version balance-affecting operation of a balance.
type BalanceTrailHead struct {
	Version   int64
	Available decimal.Decimal
	OnHold    decimal.Decimal
	// RecordedAt is when the movement was recorded (recorded_at, falling back to
	// created_at). Unlike created_at it cannot be back-dated by the client.
	RecordedAt time.Time
	// ValidatedCancelCredit marks the CREDIT row of a cancelled pending
	// transaction. Rows written before the trail fix can project an after-state
	// that differs from the balance, so only its version is comparable.
	ValidatedCancelCredit bool
}

// BalanceTrailDirection classifies one pair. Values are metric label values.
type BalanceTrailDirection string

const (
	BalanceTrailOK            BalanceTrailDirection = "ok"
	BalanceTrailDeferred      BalanceTrailDirection = "deferred"
	BalanceTrailBalanceAhead  BalanceTrailDirection = "balance_ahead"
	BalanceTrailTrailAhead    BalanceTrailDirection = "trail_ahead"
	BalanceTrailValueMismatch BalanceTrailDirection = "value_mismatch"
	// BalanceTrailUnverifiable is a balance whose trail head predates the
	// operation version columns (version 0), so nothing can be compared.
	BalanceTrailUnverifiable BalanceTrailDirection = "unverifiable"
)
