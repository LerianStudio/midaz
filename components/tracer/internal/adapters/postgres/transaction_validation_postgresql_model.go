// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TransactionValidationPostgreSQLModel is the database representation of a TransactionValidation entity.
// It follows the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
// This model handles:
// - UUID as string for database storage
// - JSONB fields for complex nested objects (account, segment, portfolio, merchant, metadata, limit_usage_details)
// - UUID arrays as string for PostgreSQL UUID[] type (matched_rule_ids, evaluated_rule_ids)
// - Nullable fields using pointers for optional JSONB columns
type TransactionValidationPostgreSQLModel struct {
	ID                   string          `db:"id"`
	RequestID            string          `db:"request_id"`
	TransactionType      string          `db:"transaction_type"`
	SubType              *string         `db:"sub_type"`
	Amount               decimal.Decimal `db:"amount"`
	Currency             string          `db:"currency"`
	TransactionTimestamp time.Time       `db:"transaction_timestamp"`
	Account              string          `db:"account"`   // JSONB
	Segment              *string         `db:"segment"`   // JSONB (nullable)
	Portfolio            *string         `db:"portfolio"` // JSONB (nullable)
	Merchant             *string         `db:"merchant"`  // JSONB (nullable)
	Metadata             string          `db:"metadata"`  // JSONB
	Decision             string          `db:"decision"`
	Reason               string          `db:"reason"`
	MatchedRuleIds       string          `db:"matched_rule_ids"`    // UUID[] as string
	EvaluatedRuleIds     string          `db:"evaluated_rule_ids"`  // UUID[] as string
	LimitUsageDetails    string          `db:"limit_usage_details"` // JSONB
	ProcessingTimeMs     float64         `db:"processing_time_ms"`
	CreatedAt            time.Time       `db:"created_at"`
}

// ToEntity converts the database model to a domain entity.
// This method handles:
// - Parsing UUIDs from strings
// - Unmarshaling JSONB fields to domain types
// - Converting string arrays to UUID slices
// - Converting string enums to typed constants
// Returns an error if JSON unmarshaling fails (e.g., corrupted data in database).
// All JSONB fields use fail-fast approach to catch data corruption early.
func (m *TransactionValidationPostgreSQLModel) ToEntity() (*model.TransactionValidation, error) {
	// Parse UUIDs from strings
	id, err := uuid.Parse(m.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid TransactionValidation ID %q: %w", m.ID, err)
	}

	requestID, err := uuid.Parse(m.RequestID)
	if err != nil {
		return nil, fmt.Errorf("invalid RequestID %q: %w", m.RequestID, err)
	}

	// Build entity with basic fields
	validation := &model.TransactionValidation{
		ID:                   id,
		RequestID:            requestID,
		TransactionType:      model.TransactionType(m.TransactionType),
		SubType:              m.SubType,
		Amount:               m.Amount,
		Currency:             m.Currency,
		TransactionTimestamp: m.TransactionTimestamp,
		EvaluationResult: model.EvaluationResult{
			Decision:         model.Decision(m.Decision),
			Reason:           m.Reason,
			MatchedRuleIDs:   []uuid.UUID{},
			EvaluatedRuleIDs: []uuid.UUID{},
		},
		LimitUsageDetails: []model.LimitUsageDetail{},
		ProcessingTimeMs:  m.ProcessingTimeMs,
		CreatedAt:         m.CreatedAt,
	}

	// Unmarshal JSONB fields
	if err := unmarshalJSONField(m.Account, &validation.Account, "account"); err != nil {
		return nil, err
	}

	validation.Segment, err = unmarshalOptionalJSON[model.SegmentContext](m.Segment, "segment")
	if err != nil {
		return nil, err
	}

	validation.Portfolio, err = unmarshalOptionalJSON[model.PortfolioContext](m.Portfolio, "portfolio")
	if err != nil {
		return nil, err
	}

	validation.Merchant, err = unmarshalOptionalJSON[model.MerchantContext](m.Merchant, "merchant")
	if err != nil {
		return nil, err
	}

	if err := unmarshalJSONField(m.Metadata, &validation.Metadata, "metadata", "{}"); err != nil {
		return nil, err
	}

	if err := unmarshalJSONField(m.LimitUsageDetails, &validation.LimitUsageDetails, "limit_usage_details", "[]"); err != nil {
		return nil, err
	}

	if validation.LimitUsageDetails == nil {
		validation.LimitUsageDetails = []model.LimitUsageDetail{}
	}

	// Parse UUID arrays from PostgreSQL format
	matchedRuleIDs, err := parseUUIDArrayString(m.MatchedRuleIds)
	if err != nil {
		return nil, fmt.Errorf("failed to parse matched_rule_ids: %w", err)
	}

	validation.MatchedRuleIDs = matchedRuleIDs

	evaluatedRuleIDs, err := parseUUIDArrayString(m.EvaluatedRuleIds)
	if err != nil {
		return nil, fmt.Errorf("failed to parse evaluated_rule_ids: %w", err)
	}

	validation.EvaluatedRuleIDs = evaluatedRuleIDs

	return validation, nil
}

// FromEntity converts a domain entity to a database model.
// This method handles:
// - Converting UUIDs to strings
// - Marshaling domain types to JSONB
// - Converting UUID slices to string arrays
// - Converting typed constants to strings
// Returns an error if JSON marshaling fails.
func (m *TransactionValidationPostgreSQLModel) FromEntity(entity *model.TransactionValidation) error {
	if entity == nil {
		return fmt.Errorf("transaction validation entity cannot be nil")
	}

	m.ID = entity.ID.String()
	m.RequestID = entity.RequestID.String()
	m.TransactionType = string(entity.TransactionType)
	m.SubType = entity.SubType
	m.Amount = entity.Amount
	m.Currency = entity.Currency
	m.TransactionTimestamp = entity.TransactionTimestamp
	m.Decision = string(entity.Decision)
	m.Reason = entity.Reason
	m.ProcessingTimeMs = entity.ProcessingTimeMs
	m.CreatedAt = entity.CreatedAt

	// Marshal account to JSONB
	accountJSON, err := json.Marshal(entity.Account)
	if err != nil {
		return fmt.Errorf("failed to marshal account: %w", err)
	}

	m.Account = string(accountJSON)

	// Marshal optional JSONB fields
	m.Segment, err = marshalOptionalJSON(entity.Segment, "segment")
	if err != nil {
		return err
	}

	m.Portfolio, err = marshalOptionalJSON(entity.Portfolio, "portfolio")
	if err != nil {
		return err
	}

	m.Merchant, err = marshalOptionalJSON(entity.Merchant, "merchant")
	if err != nil {
		return err
	}

	// Marshal metadata to JSONB, defaulting to empty object for nil
	metadata := entity.Metadata
	if metadata == nil {
		m.Metadata = "{}"
	} else {
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		m.Metadata = string(metadataJSON)
	}

	// Marshal limit usage details to JSONB, defaulting to empty array for nil
	limitUsageDetails := entity.LimitUsageDetails
	if limitUsageDetails == nil {
		limitUsageDetails = []model.LimitUsageDetail{}
	}

	limitUsageDetailsJSON, err := json.Marshal(limitUsageDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal limit usage details: %w", err)
	}

	m.LimitUsageDetails = string(limitUsageDetailsJSON)

	// Convert UUID slices to PostgreSQL array format
	m.MatchedRuleIds = formatUUIDArrayString(entity.MatchedRuleIDs)
	m.EvaluatedRuleIds = formatUUIDArrayString(entity.EvaluatedRuleIDs)

	return nil
}

// unmarshalJSONField unmarshals a JSONB string into dest, skipping empty strings and any provided skip values.
func unmarshalJSONField(data string, dest any, fieldName string, skipValues ...string) error {
	if data == "" {
		return nil
	}

	for _, sv := range skipValues {
		if data == sv {
			return nil
		}
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", fieldName, err)
	}

	return nil
}

// marshalOptionalJSON marshals an optional (pointer) value to a JSONB string pointer.
// Returns nil without error if value is nil.
func marshalOptionalJSON[T any](value *T, fieldName string) (*string, error) {
	if value == nil {
		return nil, nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s: %w", fieldName, err)
	}

	s := string(data)

	return &s, nil
}

// unmarshalOptionalJSON unmarshals an optional (nullable) JSONB field to a typed pointer.
// Returns nil without error if the source is nil or empty.
func unmarshalOptionalJSON[T any](data *string, fieldName string) (*T, error) {
	if data == nil || *data == "" {
		return nil, nil
	}

	var result T
	if err := json.Unmarshal([]byte(*data), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s: %w", fieldName, err)
	}

	return &result, nil
}

// parseUUIDArrayString parses a PostgreSQL UUID array string format to []uuid.UUID.
// Format: "{uuid1,uuid2,...}" or empty string for empty array.
// Returns error if any UUID is invalid (fail-fast approach).
func parseUUIDArrayString(arrayStr string) ([]uuid.UUID, error) {
	if arrayStr == "" || arrayStr == "{}" {
		return []uuid.UUID{}, nil
	}

	// Remove curly braces
	trimmed := arrayStr
	if len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' {
		trimmed = trimmed[1 : len(trimmed)-1]
	}

	if trimmed == "" {
		return []uuid.UUID{}, nil
	}

	// Split by comma and parse UUIDs
	parts := splitUUIDArray(trimmed)
	result := make([]uuid.UUID, 0, len(parts))

	for _, part := range parts {
		id, err := uuid.Parse(part)
		if err != nil {
			return nil, fmt.Errorf("invalid UUID %q in array: %w", part, err)
		}

		result = append(result, id)
	}

	return result, nil
}

// splitUUIDArray splits a comma-separated UUID string.
func splitUUIDArray(s string) []string {
	if s == "" {
		return []string{}
	}

	var result []string

	start := 0

	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			if part != "" {
				result = append(result, part)
			}

			start = i + 1
		}
	}

	return result
}

// formatUUIDArrayString formats a []uuid.UUID to PostgreSQL array string format.
// Format: "{uuid1,uuid2,...}" for non-empty arrays, "{}" for empty/nil arrays.
// Delegates to formatStringArrayToPostgres to avoid duplicating the formatting logic.
func formatUUIDArrayString(uuids []uuid.UUID) string {
	strs := make([]string, len(uuids))
	for i, id := range uuids {
		strs[i] = id.String()
	}

	return formatStringArrayToPostgres(strs)
}
