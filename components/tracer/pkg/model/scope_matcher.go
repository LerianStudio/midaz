// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

// RuleScopesMatch checks if ANY rule scope matches the transaction scope.
// A rule with multiple scopes uses OR logic: matches if ANY scope matches.
// Empty or nil ruleScopes = global rule (matches all transactions).
// Per API Design v1.3.1: transaction has exactly one scope derived from its contexts.
func RuleScopesMatch(ruleScopes []Scope, txScope *Scope) bool {
	if len(ruleScopes) == 0 {
		return true // global rule matches all
	}

	if txScope == nil {
		return false
	}

	for i := range ruleScopes {
		if ruleScopes[i].Matches(txScope) {
			return true
		}
	}

	return false
}
