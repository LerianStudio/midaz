// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import "github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"

// fillScopeSchemes sets Scheme from TransactionType on every scope read from
// the database that carries only the latter, so a scope stored without the
// scheme key reads back with both fields like one written today.
func fillScopeSchemes(scopes []model.Scope) {
	for i := range scopes {
		if scopes[i].Scheme != nil || scopes[i].TransactionType == nil {
			continue
		}

		scheme := string(*scopes[i].TransactionType)
		scopes[i].Scheme = &scheme
	}
}
