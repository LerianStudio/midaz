// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ReserveOperationIdentity scopes lifecycle coordination within the resolved
// tenant database. Completion may precede Reserve, so it has no request ID,
// content fingerprint or evaluation ID and cannot imply a validation decision.
type ReserveOperationIdentity struct {
	IntegrationID string
	TransactionID uuid.UUID
}

func (k ReserveOperationIdentity) Validate() error {
	if k.TransactionID == uuid.Nil || k.IntegrationID == "" || len(k.IntegrationID) > 256 ||
		!utf8.ValidString(k.IntegrationID) || strings.TrimSpace(k.IntegrationID) != k.IntegrationID || strings.ContainsRune(k.IntegrationID, 0) {
		return constant.ErrInvalidRequestBody
	}

	return nil
}

// ReserveOperationStatus records the producer's known accounting outcome.
// OPEN means no terminal outcome was recorded; there is deliberately no TTL
// transition, and elapsed time cannot prove that accounting did not commit.
type ReserveOperationStatus string

const (
	OperationOpen      ReserveOperationStatus = "OPEN"
	OperationConfirmed ReserveOperationStatus = "CONFIRMED"
	OperationReleased  ReserveOperationStatus = "RELEASED"
)

// ReserveOperationState is separate from the immutable decision. CompletedAt
// records first completion, and repeated completion must not refresh it.
type ReserveOperationState struct {
	Status      ReserveOperationStatus
	CompletedAt *time.Time
}

func (s ReserveOperationState) Validate() error {
	if s.Status == OperationOpen {
		if s.CompletedAt != nil {
			return constant.ErrInvalidRequestBody
		}

		return nil
	}

	if (s.Status != OperationConfirmed && s.Status != OperationReleased) || s.CompletedAt == nil || s.CompletedAt.IsZero() {
		return constant.ErrInvalidRequestBody
	}

	if year := s.CompletedAt.UTC().Year(); year < 1 || year > 9999 {
		return constant.ErrInvalidRequestBody
	}

	return nil
}

// ReserveReservationOwner is immutable addressing data, not another operation
// state. A reservation ID in the coordinated profile addresses its full operation.
type ReserveReservationOwner struct {
	ReservationID uuid.UUID
	EvaluationID  uuid.UUID
	Operation     ReserveOperationIdentity
}

func (o ReserveReservationOwner) Validate() error {
	if o.ReservationID == uuid.Nil || o.EvaluationID == uuid.Nil {
		return constant.ErrInvalidRequestBody
	}

	return o.Operation.Validate()
}
