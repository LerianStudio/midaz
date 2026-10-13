// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"time"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// auditTimeLayout is the timestamp format of every time value in an audit snapshot.
const auditTimeLayout = "2006-01-02T15:04:05.999Z07:00"

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
		"createdAt":   rule.CreatedAt.Format(auditTimeLayout),
		"updatedAt":   rule.UpdatedAt.Format(auditTimeLayout),
	}
}

// LimitToMap converts a Limit to a map for audit context.
// Creates an immutable snapshot by copying slices and dereferencing pointers.
// resetAt is the end of the period that contains now, because the stored
// ResetAt of a periodic limit is the boundary after its creation and does not
// describe the period an event happened in.
func LimitToMap(limit *model.Limit, now time.Time) map[string]any {
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

	return map[string]any{
		"id":              limit.ID.String(),
		"name":            limit.Name,
		"description":     description,
		"limitType":       limit.LimitType,
		"maxAmount":       limit.MaxAmount,
		"asset":           limit.Asset,
		"scopes":          scopesCopy,
		"status":          limit.Status,
		"activeTimeStart": timeOfDayOrNil(limit.ActiveTimeStart),
		"activeTimeEnd":   timeOfDayOrNil(limit.ActiveTimeEnd),
		"resetTime":       timeOfDayOrNil(limit.ResetTime),
		"customStartDate": timeOrNil(limit.CustomStartDate),
		"customEndDate":   timeOrNil(limit.CustomEndDate),
		"resetAt":         timeOrNil(limit.NextResetAt(now)),
		"createdAt":       limit.CreatedAt.Format(auditTimeLayout),
		"updatedAt":       limit.UpdatedAt.Format(auditTimeLayout),
	}
}

// timeOfDayOrNil returns t as "HH:MM", or an untyped nil so absent values
// serialize as JSON null instead of an empty string.
func timeOfDayOrNil(t *model.TimeOfDay) any {
	if t == nil {
		return nil
	}

	return t.String()
}

// timeOrNil returns t in UTC and auditTimeLayout, or an untyped nil when t is
// absent. UTC keeps a date the client sent with an offset identical to the
// same date read back from the database, so snapshots compare key by key.
func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}

	return t.UTC().Format(auditTimeLayout)
}
