// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func bankAccount(bankID, branch, account, accountType *string) *mmodel.BankingDetails {
	return &mmodel.BankingDetails{BankID: bankID, Branch: branch, Account: account, Type: accountType}
}

func strPtr(s string) *string { return &s }

// storedInstrument is a live instrument of the organization as FindAll returns it (decrypted).
func storedInstrument(bd *mmodel.BankingDetails) *mmodel.Instrument {
	id := uuid.New()

	return &mmodel.Instrument{ID: &id, BankingDetails: bd}
}

// searchesAccount matches the list query the pre-check must issue: the account token search alone.
func searchesAccount(account string) gomock.Matcher {
	return gomock.Cond(func(q http.QueryHeader) bool {
		return q.InstrumentBankingDetailsAccount != nil && *q.InstrumentBankingDetailsAccount == account &&
			q.InstrumentBankingDetailsBranch == nil && q.Limit == 0
	})
}

func requireBankAccountConflict(t *testing.T, err error) {
	t.Helper()

	var conflict pkg.EntityConflictError

	require.True(t, errors.As(err, &conflict), "want a 409 conflict, got %v", err)
	assert.Equal(t, cn.ErrBankAccountAlreadyRegistered.Error(), conflict.Code)
}

func TestCreateInstrument_BankAccountUniqueness(t *testing.T) {
	const orgID = "0193d0f0-0000-7000-8000-000000000001"

	tests := []struct {
		name    string
		input   *mmodel.BankingDetails
		stored  []*mmodel.Instrument
		refused bool
	}{
		{
			name:    "same bank, branch and account is refused",
			input:   bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), strPtr("CACC")),
			stored:  []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), strPtr("CACC")))},
			refused: true,
		},
		{
			name:    "same key under another type is refused",
			input:   bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), strPtr("PG")),
			stored:  []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), strPtr("TRAN")))},
			refused: true,
		},
		{
			name:    "bankId and branch compare trimmed",
			input:   bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), nil),
			stored:  []*mmodel.Instrument{storedInstrument(bankAccount(strPtr(" 001"), strPtr("0001 "), strPtr("123456"), nil))},
			refused: true,
		},
		{
			name:    "two branchless accounts at one bank are the same account",
			input:   bankAccount(strPtr("001"), nil, strPtr("123456"), strPtr("PG")),
			stored:  []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), nil, strPtr("123456"), strPtr("PG")))},
			refused: true,
		},
		{
			name:    "numeric branches compare without leading zeros",
			input:   bankAccount(strPtr("001"), strPtr("1"), strPtr("123456"), nil),
			stored:  []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), nil))},
			refused: true,
		},
		{
			name:    "a branchless registration matches the account held at a branch",
			input:   bankAccount(strPtr("001"), strPtr(""), strPtr("123456"), strPtr("PG")),
			stored:  []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), strPtr("CACC")))},
			refused: true,
		},
		{
			name:    "an account held branchless matches a registration at a branch",
			input:   bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), strPtr("CACC")),
			stored:  []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), strPtr(""), strPtr("123456"), strPtr("PG")))},
			refused: true,
		},
		{
			name:    "non-numeric branches compare trimmed",
			input:   bankAccount(strPtr("001"), strPtr("01A"), strPtr("123456"), nil),
			stored:  []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), strPtr(" 01A "), strPtr("123456"), nil))},
			refused: true,
		},
		{
			name:   "non-numeric branches keep their leading zeros",
			input:  bankAccount(strPtr("001"), strPtr("1A"), strPtr("123456"), nil),
			stored: []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), strPtr("01A"), strPtr("123456"), nil))},
		},
		{
			name:   "same account at another branch is accepted",
			input:  bankAccount(strPtr("001"), strPtr("0002"), strPtr("123456"), nil),
			stored: []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), nil))},
		},
		{
			name:   "same account at another bank is accepted",
			input:  bankAccount(strPtr("237"), strPtr("0001"), strPtr("123456"), nil),
			stored: []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), nil))},
		},
		{
			name:   "a holder whose account was removed does not match its stale token",
			input:  bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), nil),
			stored: []*mmodel.Instrument{storedInstrument(bankAccount(strPtr("001"), strPtr("0001"), nil, nil))},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			holderRepo := holder.NewMockRepository(ctrl)
			instrumentRepo := instrument.NewMockRepository(ctrl)
			holderID := uuid.New()

			holderRepo.EXPECT().Find(gomock.Any(), orgID, holderID, false).Return(&mmodel.Holder{ID: &holderID}, nil)
			instrumentRepo.EXPECT().
				FindAll(gomock.Any(), orgID, uuid.Nil, searchesAccount(*tt.input.Account), false).
				Return(tt.stored, nil)

			if !tt.refused {
				instrumentRepo.EXPECT().Create(gomock.Any(), orgID, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, i *mmodel.Instrument) (*mmodel.Instrument, error) { return i, nil })
			}

			uc := &UseCase{HolderRepo: holderRepo, InstrumentRepo: instrumentRepo, LedgerAccounts: &stubLedgerAccountReader{ledgerExists: true, accountExists: true}}

			_, err := uc.CreateInstrument(context.Background(), orgID, holderID, &mmodel.CreateInstrumentInput{
				LedgerID:       uuid.NewString(),
				AccountID:      uuid.NewString(),
				BankingDetails: tt.input,
			})

			if tt.refused {
				requireBankAccountConflict(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestCreateInstrument_WithoutBankAccountSkipsUniqueness(t *testing.T) {
	const orgID = "0193d0f0-0000-7000-8000-000000000001"

	for name, bd := range map[string]*mmodel.BankingDetails{
		"no banking details": nil,
		"no account":         bankAccount(strPtr("001"), strPtr("0001"), nil, nil),
		"empty account":      bankAccount(strPtr("001"), strPtr("0001"), strPtr(""), nil),
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			holderRepo := holder.NewMockRepository(ctrl)
			instrumentRepo := instrument.NewMockRepository(ctrl) // no FindAll expected
			holderID := uuid.New()

			holderRepo.EXPECT().Find(gomock.Any(), orgID, holderID, false).Return(&mmodel.Holder{ID: &holderID}, nil)
			instrumentRepo.EXPECT().Create(gomock.Any(), orgID, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, i *mmodel.Instrument) (*mmodel.Instrument, error) { return i, nil })

			uc := &UseCase{HolderRepo: holderRepo, InstrumentRepo: instrumentRepo, LedgerAccounts: &stubLedgerAccountReader{ledgerExists: true, accountExists: true}}

			_, err := uc.CreateInstrument(context.Background(), orgID, holderID, &mmodel.CreateInstrumentInput{
				LedgerID:       uuid.NewString(),
				AccountID:      uuid.NewString(),
				BankingDetails: bd,
			})
			require.NoError(t, err)
		})
	}
}

func TestUpdateInstrumentByID_BankAccountUniqueness(t *testing.T) {
	const orgID = "0193d0f0-0000-7000-8000-000000000001"

	selfID := uuid.New()
	self := &mmodel.Instrument{ID: &selfID, BankingDetails: bankAccount(strPtr("001"), strPtr("0002"), strPtr("123456"), strPtr("CACC"))}
	twinAt0001 := storedInstrument(bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), strPtr("CACC")))

	tests := []struct {
		name           string
		patch          *mmodel.BankingDetails
		fieldsToRemove []string
		holders        []*mmodel.Instrument
		refused        bool
	}{
		{
			name:    "moving to a branch that already holds the account is refused",
			patch:   &mmodel.BankingDetails{Branch: strPtr("0001")},
			holders: []*mmodel.Instrument{self, twinAt0001},
			refused: true,
		},
		{
			name:           "removing the branch while another branch holds the account is refused",
			fieldsToRemove: []string{"bankingDetails.branch"},
			holders:        []*mmodel.Instrument{self, twinAt0001},
			refused:        true,
		},
		{
			name:    "re-saving its own key is accepted",
			patch:   &mmodel.BankingDetails{Branch: strPtr("0002"), Type: strPtr("PG")},
			holders: []*mmodel.Instrument{self},
		},
		{
			name:    "moving to a free branch is accepted",
			patch:   &mmodel.BankingDetails{Branch: strPtr("0003")},
			holders: []*mmodel.Instrument{self, twinAt0001},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			instrumentRepo := instrument.NewMockRepository(ctrl)
			holderID := uuid.New()

			instrumentRepo.EXPECT().Find(gomock.Any(), orgID, holderID, selfID, false).Return(self, nil)
			instrumentRepo.EXPECT().
				FindAll(gomock.Any(), orgID, uuid.Nil, searchesAccount("123456"), false).
				Return(tt.holders, nil)

			if !tt.refused {
				instrumentRepo.EXPECT().Update(gomock.Any(), orgID, holderID, selfID, gomock.Any(), tt.fieldsToRemove).Return(self, nil)
			}

			uc := &UseCase{InstrumentRepo: instrumentRepo}

			_, err := uc.UpdateInstrumentByID(context.Background(), orgID, holderID, selfID,
				&mmodel.UpdateInstrumentInput{BankingDetails: tt.patch}, tt.fieldsToRemove)

			if tt.refused {
				requireBankAccountConflict(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestUpdateInstrumentByID_PatchOutsideBankAccountKeyReadsNothing(t *testing.T) {
	const orgID = "0193d0f0-0000-7000-8000-000000000001"

	for name, tc := range map[string]struct {
		patch          *mmodel.BankingDetails
		fieldsToRemove []string
	}{
		"metadata only":            {},
		"type only":                {patch: &mmodel.BankingDetails{Type: strPtr("PG")}},
		"removing the account":     {fieldsToRemove: []string{"bankingDetails.account"}},
		"removing banking details": {fieldsToRemove: []string{"bankingDetails"}},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			instrumentRepo := instrument.NewMockRepository(ctrl) // no Find, no FindAll expected
			holderID, id := uuid.New(), uuid.New()

			instrumentRepo.EXPECT().Update(gomock.Any(), orgID, holderID, id, gomock.Any(), tc.fieldsToRemove).
				Return(&mmodel.Instrument{ID: &id}, nil)

			uc := &UseCase{InstrumentRepo: instrumentRepo}

			_, err := uc.UpdateInstrumentByID(context.Background(), orgID, holderID, id,
				&mmodel.UpdateInstrumentInput{Metadata: map[string]any{"k": "v"}, BankingDetails: tc.patch}, tc.fieldsToRemove)
			require.NoError(t, err)
		})
	}
}
