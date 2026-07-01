// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TransactionValidation is the immutable audit record for compliance (SOX/GLBA).
// Stores individual fields for explicit traceability and queryability.
// Embeds EvaluationResult to avoid field duplication.
type TransactionValidation struct {
	// Unique identifier for this validation record
	// format: uuid
	ID uuid.UUID `json:"validationId" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Idempotency key from the originating validation request
	// format: uuid
	RequestID uuid.UUID `json:"requestId" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Type of the transaction that was validated
	// example: CARD
	TransactionType TransactionType `json:"transactionType" swaggertype:"string" enums:"CARD,WIRE,PIX,CRYPTO" example:"CARD"`

	// SubType is stored in its lowercase canonical form; matching is case-insensitive.
	// example: purchase
	// maxLength: 50
	SubType *string `json:"subType,omitempty" maxLength:"50" extensions:"x-normalization=lowercase" example:"purchase"`

	// Transaction amount that was validated
	Amount decimal.Decimal `json:"amount" swaggertype:"string" example:"100.00"`

	// ISO 4217 currency code of the transaction
	// example: USD
	Currency string `json:"currency" example:"USD"`

	// Timestamp of the original transaction
	// format: date-time
	TransactionTimestamp time.Time `json:"transactionTimestamp" format:"date-time" example:"2021-01-01T00:00:00Z"`

	// Account context from the validation request
	Account AccountContext `json:"account"`

	// Segment context from the validation request (optional)
	Segment *SegmentContext `json:"segment,omitempty"`

	// Portfolio context from the validation request (optional)
	Portfolio *PortfolioContext `json:"portfolio,omitempty"`

	// Merchant context from the validation request (optional)
	Merchant *MerchantContext `json:"merchant,omitempty"`

	// Additional metadata from the validation request
	Metadata map[string]any `json:"metadata,omitempty"`

	EvaluationResult

	// Per-limit usage details captured at validation time
	LimitUsageDetails []LimitUsageDetail `json:"limitUsageDetails"`

	// Time taken to evaluate rules and limits in milliseconds
	// example: 12.5
	ProcessingTimeMs float64 `json:"processingTimeMs" example:"12.5"`

	// Timestamp when this validation record was created
	// format: date-time
	CreatedAt time.Time `json:"createdAt" format:"date-time" example:"2021-01-01T00:00:00Z"`
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
