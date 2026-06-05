// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
)

// buildMongoFilter converts FilterCondition map to MongoDB filter format.
func (ds *ExternalDataSource) buildMongoFilter(filter map[string]model.FilterCondition) (bson.M, error) {
	mongoFilter := bson.M{}

	for field, condition := range filter {
		if isFilterConditionEmpty(condition) {
			continue
		}

		fieldFilter, err := ds.convertFilterConditionToMongoFilter(field, condition)
		if err != nil {
			return nil, fmt.Errorf("error converting filter for field '%s': %w", field, err)
		}

		for k, v := range fieldFilter {
			mongoFilter[k] = v
		}
	}

	return mongoFilter, nil
}

// convertFilterConditionToMongoFilter converts a FilterCondition to MongoDB filter.
func (ds *ExternalDataSource) convertFilterConditionToMongoFilter(field string, condition model.FilterCondition) (map[string]any, error) {
	if isFilterConditionEmpty(condition) {
		return nil, nil
	}

	if err := ds.validateFilterCondition(field, condition); err != nil {
		return nil, err
	}

	filter := make(map[string]any)
	fieldFilter := make(map[string]any)

	if len(condition.Equals) > 0 {
		if len(condition.Equals) == 1 {
			fieldFilter["$eq"] = condition.Equals[0]
		} else {
			fieldFilter["$in"] = condition.Equals
		}
	}

	if len(condition.GreaterThan) > 0 {
		fieldFilter["$gt"] = condition.GreaterThan[0]
	}

	if len(condition.GreaterOrEqual) > 0 {
		fieldFilter["$gte"] = condition.GreaterOrEqual[0]
	}

	if len(condition.LessThan) > 0 {
		fieldFilter["$lt"] = condition.LessThan[0]
	}

	if len(condition.LessOrEqual) > 0 {
		fieldFilter["$lte"] = condition.LessOrEqual[0]
	}

	if len(condition.Between) > 0 {
		// Between sets $gte and $lte — reject if those operators are already set
		// by GreaterOrEqual or LessOrEqual to avoid silent override.
		if _, exists := fieldFilter["$gte"]; exists {
			return nil, fmt.Errorf("conflicting operators for field '%s': 'between' conflicts with 'gte'", field)
		}

		if _, exists := fieldFilter["$lte"]; exists {
			return nil, fmt.Errorf("conflicting operators for field '%s': 'between' conflicts with 'lte'", field)
		}

		fieldFilter["$gte"] = condition.Between[0]
		fieldFilter["$lte"] = condition.Between[1]
	}

	if len(condition.In) > 0 {
		if _, exists := fieldFilter["$in"]; exists {
			return nil, fmt.Errorf("conflicting operators for field '%s': 'in' conflicts with multi-value 'equals'", field)
		}

		fieldFilter["$in"] = condition.In
	}

	if len(condition.NotIn) > 0 {
		fieldFilter["$nin"] = condition.NotIn
	}

	if len(fieldFilter) > 0 {
		filter[field] = fieldFilter
	}

	return filter, nil
}

// isFilterConditionEmpty checks if a FilterCondition has no active filters.
func isFilterConditionEmpty(condition model.FilterCondition) bool {
	return len(condition.Equals) == 0 &&
		len(condition.GreaterThan) == 0 &&
		len(condition.GreaterOrEqual) == 0 &&
		len(condition.LessThan) == 0 &&
		len(condition.LessOrEqual) == 0 &&
		len(condition.Between) == 0 &&
		len(condition.In) == 0 &&
		len(condition.NotIn) == 0
}

// validateFilterCondition validates that a FilterCondition has proper values for each operator.
func (ds *ExternalDataSource) validateFilterCondition(fieldName string, condition model.FilterCondition) error {
	if len(condition.Between) > 0 && len(condition.Between) != 2 {
		return fmt.Errorf("between operator for field '%s' must have exactly 2 values, got %d", fieldName, len(condition.Between))
	}

	singleValueOps := map[string][]any{
		"gt":  condition.GreaterThan,
		"gte": condition.GreaterOrEqual,
		"lt":  condition.LessThan,
		"lte": condition.LessOrEqual,
	}
	for opName, values := range singleValueOps {
		if len(values) > 0 && len(values) != 1 {
			return fmt.Errorf("%s operator for field '%s' must have exactly 1 value, got %d", opName, fieldName, len(values))
		}
	}

	return nil
}
