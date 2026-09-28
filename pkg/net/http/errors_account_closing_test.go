// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// accountClosingTuples pins the (code, status) tuple of every account-closing
// sentinel. The golden sweep derives the status from whichever typed error the
// registry maps a sentinel to, so it cannot catch a mapping that is internally
// consistent but wrong; this table states the intended status independently.
//
// The tuple is version-independent: the envelope middleware reshapes the body
// per route version, but the code and the status are the same on /v1 and /v2.
var accountClosingTuples = []struct {
	name     string
	sentinel error
	code     string
	status   int
}{
	{name: "already closed", sentinel: constant.ErrAccountAlreadyClosed, code: "0521", status: fiber.StatusConflict},
	{name: "closing in progress", sentinel: constant.ErrAccountClosingInProgress, code: "0522", status: fiber.StatusConflict},
	{name: "balance not zero", sentinel: constant.ErrAccountBalanceNotZero, code: "0523", status: fiber.StatusUnprocessableEntity},
	{name: "pending transactions", sentinel: constant.ErrAccountHasPendingTransactions, code: "0524", status: fiber.StatusUnprocessableEntity},
	{name: "persistence pending", sentinel: constant.ErrAccountClosingPersistencePending, code: "0518", status: fiber.StatusConflict},
	{name: "movement on a closed account", sentinel: constant.ErrAccountClosed, code: "0519", status: fiber.StatusUnprocessableEntity},
	{name: "protection indeterminate", sentinel: constant.ErrAccountClosingProtectionIndeterminate, code: "0520", status: fiber.StatusServiceUnavailable},
	{name: "administrative operation in progress", sentinel: constant.ErrAccountAdministrativeOperationInProgress, code: "0526", status: fiber.StatusConflict},
}

func TestAccountClosing_SentinelCodeStatusTuples(t *testing.T) {
	t.Parallel()

	for _, tc := range accountClosingTuples {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.code, tc.sentinel.Error(), "the sentinel must carry its allocated code")

			status, code := driveWithError(t, pkg.ValidateBusinessError(tc.sentinel, constant.EntityAccount))

			assert.Equal(t, tc.status, status)
			assert.Equal(t, tc.code, code)
		})
	}
}

// External manipulation keeps its own existing code: closing reuses it rather
// than minting a new one, so a caller that already handles 0074 needs no change.
func TestAccountClosing_ExternalKeepsItsExistingCode(t *testing.T) {
	t.Parallel()

	status, code := driveWithError(t, pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, constant.EntityAccount))

	assert.Equal(t, fiber.StatusUnprocessableEntity, status)
	assert.Equal(t, "0074", code)
}

// The indeterminate-protection refusal is a 5xx, and the ledger does not scrub
// 5xx titles and details. Its message must therefore stay sanitized on its own:
// it names no dependency, no key and no internal identifier.
func TestAccountClosing_ProtectionIndeterminateMessageIsSanitized(t *testing.T) {
	t.Parallel()

	err := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)

	for _, leak := range []string{"redis", "postgres", "lua", "token", "marker", "key", "%", "error:"} {
		assert.NotContains(t, unavailable.Message, leak, "the sanitized message must not name internals")
	}
}

// The two contention refusals share a status, so the message is what tells a
// caller which one it met: 0522 names a closing, 0526 names another operation
// and invites a retry. Both are static — no caller or internal value reaches
// them — so the arguments a producer passes change nothing.
func TestAccountClosing_ContentionMessagesTellClosingFromOtherOperations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		sentinel error
		title    string
		message  string
	}{
		{
			name:     "closing in progress",
			sentinel: constant.ErrAccountClosingInProgress,
			title:    "Account Closing In Progress Error",
			message:  "A closing of this account is in progress. Please wait for it to conclude and try again.",
		},
		{
			name:     "administrative operation in progress",
			sentinel: constant.ErrAccountAdministrativeOperationInProgress,
			title:    "Account Administrative Operation In Progress Error",
			message:  "Another operation on this account is in progress. Please try again shortly.",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var conflict pkg.EntityConflictError
			require.ErrorAs(t, pkg.ValidateBusinessError(tc.sentinel, constant.EntityAccount, "caller-value"), &conflict)

			assert.Equal(t, tc.sentinel.Error(), conflict.Code)
			assert.Equal(t, constant.EntityAccount, conflict.EntityType)
			assert.Equal(t, tc.title, conflict.Title)
			assert.Equal(t, tc.message, conflict.Message)
		})
	}
}
