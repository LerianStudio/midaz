// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accountexception"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

func ptrString(s string) *string { return &s }

// extractCode returns the catalog code carried by a typed midaz business error,
// failing the test when err is not one of the known business error shapes.
func extractCode(t *testing.T, err error) string {
	t.Helper()

	var (
		notFound      pkg.EntityNotFoundError
		validation    pkg.ValidationError
		conflict      pkg.EntityConflictError
		unprocessable pkg.UnprocessableOperationError
	)

	switch {
	case errors.As(err, &notFound):
		return notFound.Code
	case errors.As(err, &validation):
		return validation.Code
	case errors.As(err, &conflict):
		return conflict.Code
	case errors.As(err, &unprocessable):
		return unprocessable.Code
	}

	t.Fatalf("error is not a known midaz business error: %v", err)

	return ""
}

// TestCreateAccountExceptionSuccess covers the happy path: an existing, non-external
// account with a valid window persists and returns the created exception.
func TestCreateAccountExceptionSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	input := &mmodel.CreateAccountExceptionInput{
		OperationalTypeCodes: []string{"PIX_IN", "TED_OUT"},
		BalanceKey:           ptrString("asset-freeze"),
		Context:              "Judicial order 12345/2026",
	}

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "deposit"}, nil)

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, e *mmodel.AccountException) (*mmodel.AccountException, error) {
			assert.NotEmpty(t, e.ID)
			assert.Equal(t, organizationID.String(), e.OrganizationID)
			assert.Equal(t, ledgerID.String(), e.LedgerID)
			assert.Equal(t, accountID.String(), e.AccountID)
			assert.Equal(t, input.OperationalTypeCodes, e.OperationalTypeCodes)
			assert.Equal(t, input.Context, e.Context)
			assert.False(t, e.CreatedAt.IsZero())
			return e, nil
		})

	uc := &UseCase{AccountRepo: mockAccountRepo, AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.CreateAccountException(context.Background(), organizationID, ledgerID, accountID, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, accountID.String(), result.AccountID)
}

// TestCreateAccountExceptionAccountNotFound: a nil account is a 404 (ErrAccountIDNotFound).
func TestCreateAccountExceptionAccountNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
		Return(nil, nil)

	uc := &UseCase{AccountRepo: mockAccountRepo, AccountExceptionRepo: accountexception.NewMockRepository(ctrl)}

	result, err := uc.CreateAccountException(context.Background(), organizationID, ledgerID, accountID,
		&mmodel.CreateAccountExceptionInput{OperationalTypeCodes: []string{"PIX_IN"}, Context: "ctx"})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, constant.ErrAccountIDNotFound.Error(), extractCode(t, err))
}

// TestCreateAccountExceptionFindError: a technical failure loading the account is propagated.
func TestCreateAccountExceptionFindError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
		Return(nil, errors.New("connection reset"))

	uc := &UseCase{AccountRepo: mockAccountRepo, AccountExceptionRepo: accountexception.NewMockRepository(ctrl)}

	result, err := uc.CreateAccountException(context.Background(), organizationID, ledgerID, accountID,
		&mmodel.CreateAccountExceptionInput{OperationalTypeCodes: []string{"PIX_IN"}, Context: "ctx"})

	require.Error(t, err)
	assert.Nil(t, result)
}

// TestCreateAccountExceptionExternalAccount: exception on an external account is 0074.
func TestCreateAccountExceptionExternalAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "external"}, nil)

	uc := &UseCase{AccountRepo: mockAccountRepo, AccountExceptionRepo: accountexception.NewMockRepository(ctrl)}

	result, err := uc.CreateAccountException(context.Background(), organizationID, ledgerID, accountID,
		&mmodel.CreateAccountExceptionInput{OperationalTypeCodes: []string{"PIX_IN"}, Context: "ctx"})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, constant.ErrForbiddenExternalAccountManipulation.Error(), extractCode(t, err))
}

// TestCreateAccountExceptionInvalidWindow: expiresAt not after effectiveAt is 0505.
func TestCreateAccountExceptionInvalidWindow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	effectiveAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "deposit"}, nil)

	uc := &UseCase{AccountRepo: mockAccountRepo, AccountExceptionRepo: accountexception.NewMockRepository(ctrl)}

	result, err := uc.CreateAccountException(context.Background(), organizationID, ledgerID, accountID,
		&mmodel.CreateAccountExceptionInput{
			OperationalTypeCodes: []string{"PIX_IN"},
			Context:              "ctx",
			EffectiveAt:          &effectiveAt,
			ExpiresAt:            &expiresAt,
		})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, constant.ErrInvalidAccountExceptionValidityWindow.Error(), extractCode(t, err))
}

// TestCreateAccountExceptionRepoError: a repository failure is propagated.
func TestCreateAccountExceptionRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "deposit"}, nil)

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, errors.New("boom"))

	uc := &UseCase{AccountRepo: mockAccountRepo, AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.CreateAccountException(context.Background(), organizationID, ledgerID, accountID,
		&mmodel.CreateAccountExceptionInput{OperationalTypeCodes: []string{"PIX_IN"}, Context: "ctx"})

	require.Error(t, err)
	assert.Nil(t, result)
}

// TestCreateAccountException_EmitsCreatedEvent: a successful create publishes exactly
// one account_exception.created event carrying the persisted identity.
func TestCreateAccountException_EmitsCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "deposit"}, nil)

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, e *mmodel.AccountException) (*mmodel.AccountException, error) {
			return e, nil
		})

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := &UseCase{AccountRepo: mockAccountRepo, AccountExceptionRepo: mockExceptionRepo, Streaming: mockEmitter}

	result, err := uc.CreateAccountException(context.Background(), organizationID, ledgerID, accountID,
		&mmodel.CreateAccountExceptionInput{OperationalTypeCodes: []string{"PIX_IN"}, Context: "ctx"})

	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "account_exception", "created")
	assert.Equal(t, "account_exception.created", emitted[0].DefinitionKey)
	assert.Equal(t, result.ID, emitted[0].Subject)
}
