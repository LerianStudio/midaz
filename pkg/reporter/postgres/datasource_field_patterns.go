// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/model"
)

func isLikelyUUIDField(fieldName string) bool {
	fieldLower := strings.ToLower(fieldName)

	// Exact matches
	if fieldLower == "id" || fieldLower == "uuid" {
		return true
	}

	// Suffix matches — avoids false positives like "paid_at", "validated_at"
	suffixes := []string{"_id", "_uuid"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(fieldLower, suffix) {
			return true
		}
	}

	return false
}

func validateUUIDFieldValues(fieldName string, condition model.FilterCondition) error {
	allValues := [][]any{condition.Equals, condition.GreaterThan, condition.GreaterOrEqual, condition.LessThan, condition.LessOrEqual, condition.Between, condition.In, condition.NotIn}
	for _, values := range allValues {
		for _, value := range values {
			str, ok := value.(string)
			if !ok {
				return fmt.Errorf("field '%s' appears to be a UUID field but received non-string operand of type %T. UUID fields require string values in valid UUID format", fieldName, value)
			}

			if !isValidUUIDFormat(str) {
				return fmt.Errorf("field '%s' appears to be a UUID field but received non-UUID value '%s'. UUID fields require valid UUID format (e.g., '550e8400-e29b-41d4-a716-446655440000') or use a date field for date filtering", fieldName, str)
			}
		}
	}

	return nil
}

func isValidUUIDFormat(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func isDateField(fieldName string) bool {
	fieldLower := strings.ToLower(fieldName)

	// Suffix matches — precise detection of temporal columns
	suffixes := []string{"_at", "_date", "_time"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(fieldLower, suffix) {
			return true
		}
	}

	// Exact matches for common temporal column names
	exact := []string{"date", "time", "timestamp", "created", "updated", "deleted"}
	for _, name := range exact {
		if fieldLower == name {
			return true
		}
	}

	return false
}

// dateFormats are the layouts attempted by isDateString, ordered by likelihood.
var dateFormats = []string{
	"2006-01-02",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05Z07:00",
	time.RFC3339,
	time.RFC3339Nano,
}

func isDateString(value any) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}

	for _, layout := range dateFormats {
		if _, err := time.Parse(layout, str); err == nil {
			return true
		}
	}

	return false
}
