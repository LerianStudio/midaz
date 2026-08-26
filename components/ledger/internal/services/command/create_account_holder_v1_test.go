// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// This file locks the /v1 half of the account holder seam: HolderOffV1 is the FIRST
// gate, so a /v1 create must reach NONE of the seam's four effects — no settings read,
// no skip resolution, no requireHolder enforcement, no self-holder derivation. The
// counterpart HolderOnV2 behaviour is covered in create_account_holder_test.go; the two
// together are what make the seam a version boundary rather than a disabled feature.

// TestCreateAccountHolderOffV1_LeavesAccountUnowned proves the /v1 create persists a NULL
// holder_id and a false holder_check_skipped, on the same input that would materialise the
// org self-holder under /v2. Asserting the pair on one input is what makes this a contract
// difference rather than an incidental nil.
func TestCreateAccountHolderOffV1_LeavesAccountUnowned(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	input := func() *mmodel.CreateAccountInput {
		return &mmodel.CreateAccountInput{Name: "A", Type: "deposit", AssetCode: "USD"}
	}

	var v1Captured *mmodel.Account
	v1UC, _, v1Settings := setupHolderAccountTest(ctrl, &v1Captured)

	acc, err := v1UC.CreateAccount(ctx, organizationID, ledgerID, input(), "Bearer test", HolderOffV1)
	require.NoError(t, err)
	require.NotNil(t, acc)

	assert.Nil(t, acc.HolderID, "a /v1 create must not link a holder")
	assert.False(t, acc.HolderCheckSkipped, "a /v1 create records no honored holder skip")
	require.NotNil(t, v1Captured)
	assert.Nil(t, v1Captured.HolderID, "the persisted row must carry a NULL holder_id")
	assert.False(t, v1Captured.HolderCheckSkipped, "the persisted row must carry holder_check_skipped=false")

	// The seam is gated BEFORE the settings read, so /v1 pays none of its cost.
	assert.Zero(t, v1Settings.calls, "a /v1 create must not read the ledger holder settings")

	// Same input on /v2 DOES materialise the self-holder: the difference is the contract,
	// not the payload.
	var v2Captured *mmodel.Account
	v2UC, _, _ := setupHolderAccountTest(ctrl, &v2Captured)

	v2Acc, err := v2UC.CreateAccount(ctx, organizationID, ledgerID, input(), "Bearer test", HolderOnV2)
	require.NoError(t, err)
	require.NotNil(t, v2Acc.HolderID)
	assert.Equal(t, deriveSelfHolderID(organizationID).String(), *v2Acc.HolderID,
		"a /v2 create must still default to the org self-holder")
}

// TestCreateAccountHolderOffV1_IgnoresHolderIDInBody proves an explicit holderId in a /v1
// body is inert: it is neither honored nor validated. The /v1 response withholds both
// holder keys, so a linked value would be write-only — and validating it would hand a v1
// client the ErrHolderNotFound rejection the contract never had.
func TestCreateAccountHolderOffV1_IgnoresHolderIDInBody(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	explicitHolder := uuid.New().String()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var captured *mmodel.Account
	uc, holderReader, settingsReader := setupHolderAccountTest(ctrl, &captured)
	settingsReader.requireHolder = true
	holderReader.exists = false

	acc, err := uc.CreateAccount(ctx, organizationID, ledgerID,
		&mmodel.CreateAccountInput{Name: "A", Type: "deposit", AssetCode: "USD", HolderID: &explicitHolder},
		"Bearer test", HolderOffV1)

	require.NoError(t, err, "a /v1 create must not be rejected by the requireHolder gate")
	assert.Nil(t, acc.HolderID, "a /v1 create must not link the holder named in the body")
	assert.Zero(t, holderReader.calls, "a /v1 create must not resolve a holder")
	assert.Zero(t, settingsReader.calls, "a /v1 create must not read the ledger holder settings")
}

// TestCreateAccountHolderOffV1_IgnoresHolderSkipInBody proves a skip.holder in a /v1 body
// cannot raise ErrSkipNotPermitted. The two-key skip gate is part of the seam, so a v1
// client would otherwise acquire a 422 from a version upgrade it never asked for.
func TestCreateAccountHolderOffV1_IgnoresHolderSkipInBody(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var captured *mmodel.Account
	uc, _, settingsReader := setupHolderAccountTest(ctrl, &captured)
	// allowHolderSkip stays false, which is what makes a requested skip a 422 on /v2.
	settingsReader.allowHolderSkip = false

	acc, err := uc.CreateAccount(ctx, organizationID, ledgerID,
		&mmodel.CreateAccountInput{Name: "A", Type: "deposit", AssetCode: "USD", Skip: &mmodel.AccountSkip{Holder: true}},
		"Bearer test", HolderOffV1)

	require.NoError(t, err, "a /v1 create must not be rejected for requesting an unpermitted skip")
	assert.False(t, acc.HolderCheckSkipped, "an inert /v1 skip records no honored skip")
	assert.Zero(t, settingsReader.calls, "a /v1 create must not read the skip override")
}

// TestCreateAccountHolderOffV1_SurvivesSettingsReadFailure proves the gate sits BEFORE the
// settings read, not after it: the read that fails CLOSED on /v2 is never performed on /v1,
// so a transient PostgreSQL failure cannot fail a /v1 create the seam does not apply to.
func TestCreateAccountHolderOffV1_SurvivesSettingsReadFailure(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var captured *mmodel.Account
	uc, _, settingsReader := setupHolderAccountTest(ctrl, &captured)
	settingsReader.err = errors.New("postgres settings read failed")

	acc, err := uc.CreateAccount(ctx, organizationID, ledgerID,
		&mmodel.CreateAccountInput{Name: "A", Type: "deposit", AssetCode: "USD"},
		"Bearer test", HolderOffV1)

	require.NoError(t, err)
	require.NotNil(t, acc)
	assert.Nil(t, acc.HolderID)
	assert.Zero(t, settingsReader.calls, "the holder settings read must be gated off entirely on /v1")
}
