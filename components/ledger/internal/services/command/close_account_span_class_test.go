// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestCloseAccount_ReadsAnUnavailableAccountStoreAsTechnical pins the refusal class
// of a read that failed rather than answered. The repository reports a missing row
// as a business error and everything else with the driver's own, so an unwrapped
// failure says the closing state could not be established at all: it is the
// dependency refusal, and it flips the command span red instead of passing an
// unclassified error through to a generic failure.
func TestCloseAccount_ReadsAnUnavailableAccountStoreAsTechnical(t *testing.T) {
	m := newCloseAccountMocks(t)
	ctx, recorder := recordingContext()

	m.account.EXPECT().Find(gomock.Any(), closeOrgID, closeLedgerID, nil, closeAccountID, gomock.Any()).
		Return(nil, errors.New("connection reset by peer"))

	closedAt, err := m.uc.CloseAccount(ctx, closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.True(t, closedAt.IsZero())

	span := findSpan(t, recorder, "command.close_account")
	assert.Equal(t, codes.Error, span.Status().Code)
}

// TestCloseAccount_ReadsAMissingAccountAsBusiness is the other side of the same
// branch: a row the repository proved absent is an answer about this account, so
// it keeps its own code and leaves the span green.
func TestCloseAccount_ReadsAMissingAccountAsBusiness(t *testing.T) {
	m := newCloseAccountMocks(t)
	ctx, recorder := recordingContext()

	notFound := midazpkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAccount)

	m.account.EXPECT().Find(gomock.Any(), closeOrgID, closeLedgerID, nil, closeAccountID, gomock.Any()).
		Return(nil, notFound)

	closedAt, err := m.uc.CloseAccount(ctx, closeOrgID, closeLedgerID, closeAccountID)

	require.ErrorIs(t, err, notFound)
	assert.True(t, closedAt.IsZero())

	span := findSpan(t, recorder, "command.close_account")
	assert.Equal(t, codes.Unset, span.Status().Code)
}

// TestCloseAccount_RecordsAnUnreadableProtectionAsTechnical pins that the two
// classes the acquisition can refuse with are not collapsed: a protection surface
// that could not be read is a dependency failing, and recording it as a business
// outcome would leave this span green over the red one the guard already recorded.
func TestCloseAccount_RecordsAnUnreadableProtectionAsTechnical(t *testing.T) {
	m := newCloseAccountMocks(t)
	ctx, recorder := recordingContext()

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return("", false, nil)
	m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(false, errors.New("cache unavailable"))

	closedAt, err := m.uc.CloseAccount(ctx, closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.True(t, closedAt.IsZero())

	span := findSpan(t, recorder, "command.close_account")
	assert.Equal(t, codes.Error, span.Status().Code)
}

// TestCloseAccount_RecordsAContendedAccountAsBusiness is the other class of the
// same refusal: another operation holding the account is the coordination doing
// its job, so the span stays green.
func TestCloseAccount_RecordsAContendedAccountAsBusiness(t *testing.T) {
	m := newCloseAccountMocks(t)
	ctx, recorder := recordingContext()

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return("", false, nil)
	m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(false, nil)

	closedAt, err := m.uc.CloseAccount(ctx, closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingInProgress)
	assert.True(t, closedAt.IsZero())

	span := findSpan(t, recorder, "command.close_account")
	assert.Equal(t, codes.Unset, span.Status().Code)
}
