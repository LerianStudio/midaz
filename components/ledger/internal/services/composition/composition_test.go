// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package composition

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAccountCreator records its call and returns canned values.
type stubAccountCreator struct {
	called    bool
	gotOrg    uuid.UUID
	gotLedger uuid.UUID
	gotInput  *mmodel.CreateAccountInput
	gotToken  string
	account   *mmodel.Account
	err       error
}

func (s *stubAccountCreator) CreateAccount(_ context.Context, organizationID, ledgerID uuid.UUID, in *mmodel.CreateAccountInput, token string) (*mmodel.Account, error) {
	s.called = true
	s.gotOrg = organizationID
	s.gotLedger = ledgerID
	s.gotInput = in
	s.gotToken = token

	return s.account, s.err
}

// stubInstrumentCreator records its call and returns canned values.
type stubInstrumentCreator struct {
	called     bool
	gotOrg     string
	gotHolder  uuid.UUID
	gotInput   *mmodel.CreateInstrumentInput
	instrument *mmodel.Instrument
	err        error
}

func (s *stubInstrumentCreator) CreateInstrument(_ context.Context, organizationID string, holderID uuid.UUID, in *mmodel.CreateInstrumentInput) (*mmodel.Instrument, error) {
	s.called = true
	s.gotOrg = organizationID
	s.gotHolder = holderID
	s.gotInput = in

	return s.instrument, s.err
}

func ptr[T any](v T) *T { return &v }

func bankingInput() *mmodel.CreateHolderAccountInput {
	return &mmodel.CreateHolderAccountInput{
		Name:      "Corporate Checking",
		AssetCode: "USD",
		Type:      "deposit",
		BankingDetails: &mmodel.BankingDetails{
			Branch: ptr("0001"),
		},
	}
}

// TestCreateHolderAccount_AccountError proves an account-create error is
// returned verbatim and the instrument is never attempted.
func TestCreateHolderAccount_AccountError(t *testing.T) {
	wantErr := pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, "Account")

	acc := &stubAccountCreator{err: wantErr}
	inst := &stubInstrumentCreator{}
	svc := NewService(acc, inst)

	resp, err := svc.CreateHolderAccount(context.Background(), uuid.New(), uuid.New(), uuid.New(), bankingInput(), "token")

	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, resp)
	assert.True(t, acc.called, "account creator must be called")
	assert.False(t, inst.called, "instrument creator must NOT be called when account fails")
}

// TestCreateHolderAccount_AccountOnly proves that with no instrument fields the
// instrument is never created and no failure block appears.
func TestCreateHolderAccount_AccountOnly(t *testing.T) {
	acc := &stubAccountCreator{account: &mmodel.Account{ID: uuid.NewString()}}
	inst := &stubInstrumentCreator{}
	svc := NewService(acc, inst)

	in := &mmodel.CreateHolderAccountInput{Name: "Acc", AssetCode: "USD", Type: "deposit"}

	resp, err := svc.CreateHolderAccount(context.Background(), uuid.New(), uuid.New(), uuid.New(), in, "token")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Same(t, acc.account, resp.Account)
	assert.Nil(t, resp.Instrument)
	assert.Nil(t, resp.InstrumentError)
	assert.False(t, inst.called, "instrument creator must NOT be called on the account-only path")
}

// TestCreateHolderAccount_Happy proves both primitives compose and the IDs are
// threaded correctly: the account's HolderID is the path holder, and the
// instrument is linked to the just-created account ID and the path ledger ID.
func TestCreateHolderAccount_Happy(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()
	accountID := uuid.NewString()

	acc := &stubAccountCreator{account: &mmodel.Account{ID: accountID}}
	inst := &stubInstrumentCreator{instrument: &mmodel.Instrument{}}
	svc := NewService(acc, inst)

	in := bankingInput()

	resp, err := svc.CreateHolderAccount(context.Background(), orgID, ledgerID, holderID, in, "token")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Same(t, acc.account, resp.Account)
	assert.Same(t, inst.instrument, resp.Instrument)
	assert.Nil(t, resp.InstrumentError)

	// Account leg: org/ledger/token threaded; HolderID is the PATH holder.
	assert.Equal(t, orgID, acc.gotOrg)
	assert.Equal(t, ledgerID, acc.gotLedger)
	assert.Equal(t, "token", acc.gotToken)
	require.NotNil(t, acc.gotInput.HolderID)
	assert.Equal(t, holderID.String(), *acc.gotInput.HolderID)

	// Instrument leg: org/holder threaded; account+ledger IDs from the create.
	assert.Equal(t, orgID.String(), inst.gotOrg)
	assert.Equal(t, holderID, inst.gotHolder)
	assert.Equal(t, accountID, inst.gotInput.AccountID)
	assert.Equal(t, ledgerID.String(), inst.gotInput.LedgerID)
	assert.Same(t, in.BankingDetails, inst.gotInput.BankingDetails, "the request banking details are forwarded onto the instrument input")
}

// TestCreateHolderAccount_InstrumentError proves the non-compensating
// partial-failure contract: the account survives, no error is returned, and a
// typed InstrumentError with a stable reason code is surfaced. The stub never
// receives a delete call (there is none), so the absence of compensation is
// structural.
func TestCreateHolderAccount_InstrumentError(t *testing.T) {
	acc := &stubAccountCreator{account: &mmodel.Account{ID: uuid.NewString()}}
	inst := &stubInstrumentCreator{err: pkg.ValidateBusinessError(constant.ErrHolderNotFound, "Instrument")}
	svc := NewService(acc, inst)

	resp, err := svc.CreateHolderAccount(context.Background(), uuid.New(), uuid.New(), uuid.New(), bankingInput(), "token")

	require.NoError(t, err, "instrument error must NOT fail the request")
	require.NotNil(t, resp)
	assert.Same(t, acc.account, resp.Account, "account must remain in the response (persisted, no rollback)")
	assert.Nil(t, resp.Instrument)
	require.NotNil(t, resp.InstrumentError)
	assert.Equal(t, instrumentFailureStatus, resp.InstrumentError.Status)
	assert.Equal(t, constant.ErrHolderNotFound.Error(), resp.InstrumentError.Reason, "reason is the stable business code, not raw error text")
}

// TestHasInstrumentFields documents the D-8 explicit-only gate contract,
// including the F4-local 2 edge: an empty-but-present BankingDetails{} (a
// non-nil pointer to a zero value) DOES trigger an instrument, because the
// caller explicitly asked for one. The predicate is nil-pointer based.
func TestHasInstrumentFields(t *testing.T) {
	tests := []struct {
		name string
		in   *mmodel.CreateHolderAccountInput
		want bool
	}{
		{
			name: "all nil/empty -> no instrument",
			in:   &mmodel.CreateHolderAccountInput{Name: "Acc", AssetCode: "USD", Type: "deposit"},
			want: false,
		},
		{
			name: "banking details present -> instrument",
			in:   &mmodel.CreateHolderAccountInput{BankingDetails: &mmodel.BankingDetails{Branch: ptr("0001")}},
			want: true,
		},
		{
			name: "EMPTY-but-present bankingDetails{} -> instrument (explicit contract)",
			in:   &mmodel.CreateHolderAccountInput{BankingDetails: &mmodel.BankingDetails{}},
			want: true,
		},
		{
			name: "regulatory fields present -> instrument",
			in:   &mmodel.CreateHolderAccountInput{RegulatoryFields: &mmodel.RegulatoryFields{}},
			want: true,
		},
		{
			name: "related parties present -> instrument",
			in:   &mmodel.CreateHolderAccountInput{RelatedParties: []*mmodel.RelatedParty{{}}},
			want: true,
		},
		{
			name: "empty related-parties slice -> no instrument",
			in:   &mmodel.CreateHolderAccountInput{RelatedParties: []*mmodel.RelatedParty{}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasInstrumentFields(tt.in))
		})
	}
}

// TestInstrumentFailureReason proves the reason code is the business code for
// recognized typed errors and a stable fallback otherwise, never raw text.
func TestInstrumentFailureReason(t *testing.T) {
	assert.Equal(t, constant.ErrHolderNotFound.Error(), instrumentFailureReason(pkg.ValidateBusinessError(constant.ErrHolderNotFound, "Instrument")))
	assert.Equal(t, constant.ErrInstrumentNotFound.Error(), instrumentFailureReason(pkg.ValidateBusinessError(constant.ErrInstrumentNotFound, "Instrument")))
	assert.Equal(t, instrumentFailureFallbackReason, instrumentFailureReason(errors.New("some opaque internal failure")))
}
