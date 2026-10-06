// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/scheme"
)

// errInvalidSchemeFilter reports a scheme query filter that is not a valid
// scheme; each list endpoint maps it to its own invalid-filter error.
var errInvalidSchemeFilter = errors.New("invalid scheme filter")

// resolveSchemeFilter resolves the scheme query filter from the scheme param
// and its deprecated alias transaction_type. Both are trimmed and upper-cased;
// a nil or empty param is absent. It returns nil when neither carries a value,
// constant.ErrValidationSchemeAliasConflict when both do with different values,
// and errInvalidSchemeFilter when a present value is not a valid scheme.
func resolveSchemeFilter(alias, primary *string) (*model.TransactionType, error) {
	normalizedAlias, aliasOK := scheme.Normalize(stringValue(alias))
	normalizedPrimary, primaryOK := scheme.Normalize(stringValue(primary))

	if normalizedAlias != "" && normalizedPrimary != "" && normalizedAlias != normalizedPrimary {
		return nil, constant.ErrValidationSchemeAliasConflict
	}

	if !aliasOK || !primaryOK {
		return nil, errInvalidSchemeFilter
	}

	value := normalizedPrimary
	if value == "" {
		value = normalizedAlias
	}

	if value == "" {
		return nil, nil
	}

	transactionType := model.TransactionType(value)

	return &transactionType, nil
}

// normalizeScopeSchemes canonicalizes the scheme and its deprecated alias
// transactionType of every scope in place, so both carry the same trimmed,
// upper-cased value. On the first scope that cannot be normalized it returns
// that scope's index, the JSON name of the offending field and either
// constant.ErrValidationSchemeAliasConflict or constant.ErrLimitInvalidScope.
func normalizeScopeSchemes(scopes []model.Scope) (int, string, error) {
	for i := range scopes {
		normalized, err := model.NormalizeScope(scopes[i])
		if err != nil {
			return i, invalidSchemeField(scopes[i]), err
		}

		scopes[i] = normalized
	}

	return -1, "", nil
}

// invalidSchemeField names the scope field that holds an invalid scheme:
// scheme when it is present and invalid, transactionType otherwise.
func invalidSchemeField(scope model.Scope) string {
	if scope.Scheme != nil {
		if _, ok := model.NewTransactionType(*scope.Scheme); !ok {
			return "scheme"
		}
	}

	return "transactionType"
}

// stringValue returns *s, or "" when s is nil.
func stringValue(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
