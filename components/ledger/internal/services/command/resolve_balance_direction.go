// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import "github.com/LerianStudio/midaz/v4/pkg/constant"

// resolveBalanceDirection decides a balance's direction by precedence:
// an explicit caller override wins; failing that, the account type's default
// wins; failing that, the direction implied by the account type is used
// (external -> debit, others -> credit). explicit and typeDefault carry the
// direction only when set to constant.DirectionCredit or constant.DirectionDebit;
// any other value (including the empty string "not set") falls through.
func resolveBalanceDirection(explicit, typeDefault, accountType string) string {
	if explicit == constant.DirectionCredit || explicit == constant.DirectionDebit {
		return explicit
	}

	if typeDefault == constant.DirectionCredit || typeDefault == constant.DirectionDebit {
		return typeDefault
	}

	return defaultBalanceDirection(accountType)
}
