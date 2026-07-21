// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import "github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"

// RuleToMap converts a Rule to a map for audit context.
// Creates an immutable snapshot by copying slices and dereferencing pointers.
func RuleToMap(rule *model.Rule) map[string]any {
	if rule == nil {
		return nil
	}

	// Create a copy of Scopes slice to avoid shared references
	scopesCopy := make([]model.Scope, len(rule.Scopes))
	copy(scopesCopy, rule.Scopes)

	// Dereference Description pointer or use nil
	var description any
	if rule.Description != nil {
		description = *rule.Description
	}

	return map[string]any{
		"id":          rule.ID.String(),
		"name":        rule.Name,
		"description": description,
		"expression":  rule.Expression,
		"action":      rule.Action,
		"scopes":      scopesCopy,
		"status":      rule.Status,
		"createdAt":   rule.CreatedAt.Format("2006-01-02T15:04:05.999Z07:00"),
		"updatedAt":   rule.UpdatedAt.Format("2006-01-02T15:04:05.999Z07:00"),
	}
}

// LimitToMap converts a Limit to a map for audit context.
// Creates an immutable snapshot by copying slices and dereferencing pointers.
func LimitToMap(limit *model.Limit) map[string]any {
	if limit == nil {
		return nil
	}

	// Create a copy of Scopes slice to avoid shared references
	scopesCopy := make([]model.Scope, len(limit.Scopes))
	copy(scopesCopy, limit.Scopes)

	// Dereference Description pointer or use nil
	var description any
	if limit.Description != nil {
		description = *limit.Description
	}

	// Dereference ResetAt pointer or use nil
	var resetAt any
	if limit.ResetAt != nil {
		resetAt = limit.ResetAt.Format("2006-01-02T15:04:05.999Z07:00")
	}

	return map[string]any{
		"id":          limit.ID.String(),
		"name":        limit.Name,
		"description": description,
		"limitType":   limit.LimitType,
		"maxAmount":   limit.MaxAmount,
		"currency":    limit.Currency,
		"scopes":      scopesCopy,
		"status":      limit.Status,
		"resetAt":     resetAt,
		"createdAt":   limit.CreatedAt.Format("2006-01-02T15:04:05.999Z07:00"),
		"updatedAt":   limit.UpdatedAt.Format("2006-01-02T15:04:05.999Z07:00"),
	}
}
