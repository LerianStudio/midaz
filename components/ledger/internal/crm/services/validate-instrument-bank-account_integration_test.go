//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

// rotatingEncryptor stands in for a search keyset mid-rotation: a write mints its token under the
// active key, a search asks for every enabled key.
type rotatingEncryptor struct {
	active  string
	enabled []string
}

func (e *rotatingEncryptor) EncryptField(_ context.Context, _ encryption.FieldContext, plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (e *rotatingEncryptor) DecryptField(_ context.Context, _ encryption.FieldContext, ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "enc:"), nil
}

func (e *rotatingEncryptor) GenerateSearchToken(_ context.Context, _ encryption.SearchTokenContext, value string) (string, uint32, error) {
	return e.active + ":" + value, 0, nil
}

func (e *rotatingEncryptor) GenerateSearchTokenCandidates(_ context.Context, _ encryption.SearchTokenContext, value string) ([]string, error) {
	tokens := make([]string, 0, len(e.enabled))
	for _, key := range e.enabled {
		tokens = append(tokens, key+":"+value)
	}

	return tokens, nil
}

func TestIntegration_CreateInstrument_BankAccountUniquenessAcrossRotationAndDeletion(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	conn := mongotestutil.CreateConnection(t, container.URI, container.DBName)
	fe := &rotatingEncryptor{active: "k1", enabled: []string{"k1"}}

	holderRepo, err := holder.NewMongoDBRepository(conn, fe)
	require.NoError(t, err)

	instrumentRepo, err := instrument.NewMongoDBRepository(conn, fe)
	require.NoError(t, err)

	uc := &UseCase{HolderRepo: holderRepo, InstrumentRepo: instrumentRepo, LedgerAccounts: &stubLedgerAccountReader{ledgerExists: true, accountExists: true}}
	ctx := context.Background()
	organizationID := uuid.NewString()

	owner, err := holderRepo.Create(ctx, organizationID, mongotestutil.CreateTestHolderSimple(t, "Owner", "12345678901"))
	require.NoError(t, err)

	register := func() (*mmodel.Instrument, error) {
		return uc.CreateInstrument(ctx, organizationID, *owner.ID, &mmodel.CreateInstrumentInput{
			LedgerID:       uuid.NewString(),
			AccountID:      uuid.NewString(),
			BankingDetails: bankAccount(strPtr("001"), strPtr("0001"), strPtr("123456"), strPtr("CACC")),
		})
	}

	first, err := register()
	require.NoError(t, err)

	// The keyset rotates: the first row's token was minted under k1, a new one would be under k2, so the
	// unique index cannot see the pair and only the pre-check's every-key search refuses it.
	fe.active, fe.enabled = "k2", []string{"k2", "k1"}

	_, err = register()
	requireBankAccountConflict(t, err)

	require.NoError(t, uc.DeleteInstrumentByID(ctx, organizationID, *owner.ID, *first.ID, false))

	_, err = register()
	require.NoError(t, err, "a soft-deleted instrument no longer holds its bank account")
}
