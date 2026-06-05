// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// TransactionValidation is the immutable audit record for compliance (SOX/GLBA).
// Stores individual fields for explicit traceability and queryability.
// Embeds EvaluationResult to avoid field duplication.
type TransactionValidation struct {
	ID              uuid.UUID       `json:"validationId" swaggertype:"string" format:"uuid"`
	RequestID       uuid.UUID       `json:"requestId" swaggertype:"string" format:"uuid"`
	TransactionType TransactionType `json:"transactionType"`
	// SubType is stored in its lowercase canonical form; matching is case-insensitive.
	SubType              *string           `json:"subType,omitempty" maxLength:"50" extensions:"x-normalization=lowercase"`
	Amount               decimal.Decimal   `json:"amount" swaggertype:"string" example:"100.00"`
	Currency             string            `json:"currency"`
	TransactionTimestamp time.Time         `json:"transactionTimestamp" format:"date-time"`
	Account              AccountContext    `json:"account"`
	Segment              *SegmentContext   `json:"segment,omitempty"`
	Portfolio            *PortfolioContext `json:"portfolio,omitempty"`
	Merchant             *MerchantContext  `json:"merchant,omitempty"`
	Metadata             map[string]any    `json:"metadata,omitempty"`
	EvaluationResult
	LimitUsageDetails []LimitUsageDetail `json:"limitUsageDetails"`
	ProcessingTimeMs  float64            `json:"processingTimeMs"`
	CreatedAt         time.Time          `json:"createdAt" format:"date-time"`
}

// NewTransactionValidation creates a TransactionValidation with initialized slices.
// Ensures JSON serialization produces [] instead of null for empty arrays.
// The createdAt parameter allows deterministic testing; use time.Now().UTC() in production.
// Returns error if:
//   - id is uuid.Nil → constant.ErrTransactionValidationIDRequired
//   - decision is not valid → constant.ErrInvalidDecision
//   - createdAt is zero → constant.ErrTransactionValidationCreatedAtRequired
func NewTransactionValidation(id uuid.UUID, decision Decision, createdAt time.Time) (*TransactionValidation, error) {
	// Validate id
	if id == uuid.Nil {
		return nil, constant.ErrTransactionValidationIDRequired
	}

	// Validate decision
	if !decision.IsValid() {
		return nil, constant.ErrInvalidDecision
	}

	// Validate createdAt
	if createdAt.IsZero() {
		return nil, constant.ErrTransactionValidationCreatedAtRequired
	}

	return &TransactionValidation{
		ID: id,
		EvaluationResult: EvaluationResult{
			Decision:         decision,
			MatchedRuleIDs:   []uuid.UUID{},
			EvaluatedRuleIDs: []uuid.UUID{},
			Reason:           "",
		},
		LimitUsageDetails: []LimitUsageDetail{},
		CreatedAt:         createdAt,
	}, nil
}

// ToValidationResponse converts the TransactionValidation entity to a ValidationResponse DTO.
// This is used for idempotency responses - when a duplicate request is detected,
// we return the previously stored validation result.
// EvaluatedAt is set from CreatedAt since that's when the original evaluation occurred.
// Returns nil if the receiver is nil (defensive guard for chaining with FindByRequestID).
func (tv *TransactionValidation) ToValidationResponse() *ValidationResponse {
	if tv == nil {
		return nil
	}

	// Defensive copy for LimitUsageDetails slice to prevent external mutation.
	// Deep copies nested Scopes slice; pointer fields within Scope (*uuid.UUID, *string)
	// are shallow-copied since the pointed-to values are immutable by convention.
	limitDetailsCopy := make([]LimitUsageDetail, len(tv.LimitUsageDetails))
	for i, detail := range tv.LimitUsageDetails {
		limitDetailsCopy[i] = detail
		if detail.Scopes != nil {
			scopesCopy := make([]Scope, len(detail.Scopes))
			copy(scopesCopy, detail.Scopes)
			limitDetailsCopy[i].Scopes = scopesCopy
		}
	}

	matchedCopy := make([]uuid.UUID, len(tv.MatchedRuleIDs))
	copy(matchedCopy, tv.MatchedRuleIDs)

	evaluatedCopy := make([]uuid.UUID, len(tv.EvaluatedRuleIDs))
	copy(evaluatedCopy, tv.EvaluatedRuleIDs)

	return &ValidationResponse{
		ValidationID: tv.ID,
		RequestID:    tv.RequestID,
		EvaluationResult: EvaluationResult{
			Decision:         tv.Decision,
			Reason:           tv.Reason,
			MatchedRuleIDs:   matchedCopy,
			EvaluatedRuleIDs: evaluatedCopy,
		},
		LimitUsageDetails: limitDetailsCopy,
		ProcessingTimeMs:  tv.ProcessingTimeMs,
		EvaluatedAt:       tv.CreatedAt,
	}
}
