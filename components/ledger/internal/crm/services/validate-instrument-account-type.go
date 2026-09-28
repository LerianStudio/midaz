// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// validInstrumentAccountTypes is the membership set for RegulatoryFields.AccountType,
// derived from the canonical list so the accepted values are declared once.
var validInstrumentAccountTypes = func() map[string]struct{} {
	accountTypes := mmodel.InstrumentAccountTypes()

	set := make(map[string]struct{}, len(accountTypes))
	for _, accountType := range accountTypes {
		set[accountType] = struct{}{}
	}

	return set
}()

// normalizeInstrumentAccountType trims and upper-cases a regulatory account type and
// validates it against the canonical set. A nil or blank value is treated as absent and
// returns nil without error, so it is never persisted. A value outside the set returns
// the ErrInvalidInstrumentAccountType business error; the caller records it on its span.
func normalizeInstrumentAccountType(accountType *string) (*string, error) {
	if accountType == nil {
		return nil, nil
	}

	normalized := strings.ToUpper(strings.TrimSpace(*accountType))
	if normalized == "" {
		return nil, nil
	}

	if _, ok := validInstrumentAccountTypes[normalized]; !ok {
		return nil, pkg.ValidateBusinessError(cn.ErrInvalidInstrumentAccountType, cn.EntityInstrument)
	}

	return &normalized, nil
}
