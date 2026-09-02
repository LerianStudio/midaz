// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg_test

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateBusinessError_AccountExceptionNotFound pins the 404 family of the two
// account-exception lookup codes:
//
//   - 0503 ErrAccountExceptionNotFound — a single exception that does not exist or was
//     removed.
//   - 0504 ErrNoAccountExceptionsFound — an empty list result, the repo-wide convention
//     already used by ErrNoAccountsFound and ErrNoAccountTypesFound.
//
// Both MUST render as EntityNotFoundError, the same family as 0109, so the HTTP 404
// contract is identical and integrators branch on the code, not on the status.
// The end-to-end status mapping is swept by TestGolden_BusinessErrorCodeStatus in
// pkg/net/http; asserting it here would require importing the net/http layer that
// imports this package.
func TestValidateBusinessError_AccountExceptionNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sentinel   error
		wantCode   string
		entityType string
	}{
		{
			name:       "ErrAccountExceptionNotFound maps to EntityNotFoundError (0503)",
			sentinel:   constant.ErrAccountExceptionNotFound,
			wantCode:   "0503",
			entityType: constant.EntityAccountException,
		},
		{
			name:       "ErrNoAccountExceptionsFound maps to EntityNotFoundError (0504)",
			sentinel:   constant.ErrNoAccountExceptionsFound,
			wantCode:   "0504",
			entityType: constant.EntityAccountException,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := pkg.ValidateBusinessError(tt.sentinel, tt.entityType)
			require.Error(t, got, "ValidateBusinessError must return a mapped error for %s", tt.sentinel.Error())

			mapped, ok := got.(pkg.EntityNotFoundError)
			require.Truef(t, ok, "%s must map to EntityNotFoundError (HTTP 404), got %T", tt.sentinel.Error(), got)

			assert.Equal(t, tt.wantCode, mapped.Code, "mapped error code must match the sentinel error string")
			assert.Equal(t, tt.entityType, mapped.EntityType, "entityType must be propagated through ValidateBusinessError")
			assert.NotEmpty(t, mapped.Title, "mapped error must have a non-empty Title")
			assert.NotEmpty(t, mapped.Message, "mapped error must have a non-empty Message")
		})
	}
}

// TestValidateBusinessError_AccountExceptionValidityWindow pins 0505 to the
// ValidationError (HTTP 400) family of 0500: an expiration that is not strictly after
// the start is a malformed request the caller can fix, never a 404 or a 422.
func TestValidateBusinessError_AccountExceptionValidityWindow(t *testing.T) {
	t.Parallel()

	got := pkg.ValidateBusinessError(constant.ErrInvalidAccountExceptionValidityWindow, constant.EntityAccountException)
	require.Error(t, got)

	mapped, ok := got.(pkg.ValidationError)
	require.Truef(t, ok, "0505 must map to ValidationError (HTTP 400), got %T", got)

	assert.Equal(t, "0505", mapped.Code)
	assert.Equal(t, constant.EntityAccountException, mapped.EntityType)
	assert.NotEmpty(t, mapped.Title)
	assert.NotEmpty(t, mapped.Message)
}

// TestAccountExceptionSentinelCodes pins the three new sentinel strings themselves.
// The codes are a published API contract: a renumbering is a breaking change for every
// integrator branching on them, so the literals are asserted here rather than derived.
func TestAccountExceptionSentinelCodes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0503", constant.ErrAccountExceptionNotFound.Error())
	assert.Equal(t, "0504", constant.ErrNoAccountExceptionsFound.Error())
	assert.Equal(t, "0505", constant.ErrInvalidAccountExceptionValidityWindow.Error())
	assert.Equal(t, "AccountException", constant.EntityAccountException)
}

// TestAccountBlockedSentinelUnchanged is the RF-03 regression pin carried forward:
// introducing 0503-0505 MUST NOT disturb 0502 or its 422 rendering.
func TestAccountBlockedSentinelUnchanged(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0502", constant.ErrAccountBlockedTransactionRestriction.Error())

	got := pkg.ValidateBusinessError(constant.ErrAccountBlockedTransactionRestriction, "Transaction")

	mapped, ok := got.(pkg.UnprocessableOperationError)
	require.Truef(t, ok, "0502 must still map to UnprocessableOperationError, got %T", got)
	assert.Equal(t, "0502", mapped.Code)
	assert.Equal(t, "Account Blocked Transaction Restriction", mapped.Title)
}
