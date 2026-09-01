// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	billing_package "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/billing_package"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

// The by-ID billing-package operations take the ledger the caller is acting
// within. These tests pin all three halves of that contract: a named ledger
// reaches the repository unchanged, uuid.Nil reaches it as the organization-scope
// sentinel and not as an all-zero ledger, and a package the named ledger does not
// own is reported absent — never as a forbidden or otherwise existing resource.

func TestGetBillingPackageByID_PassesCallerLedgerToRepository(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	bpID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(ledgerID.String())).
		Return(&model.BillingPackage{ID: bpID.String(), LedgerID: ledgerID.String()}, nil)

	got, err := svc.GetBillingPackageByID(context.Background(), bpID, orgID, ledgerID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, bpID.String(), got.ID)
}

// TestBillingPackageLedgerScope_NilIsTheEmptySentinel guards the one conversion
// this aggregate needs that the fee-package aggregate does not: the service speaks
// UUIDs and the repository speaks strings, so uuid.Nil must become the empty
// organization-scope sentinel. Rendering it with String() would produce an
// all-zero ledger id, a ledger no package is owned by, and every
// organization-scoped request would answer 404.
func TestBillingPackageLedgerScope_NilIsTheEmptySentinel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, billing_package.AnyLedger, billingPackageLedgerScope(uuid.Nil))
	assert.NotEqual(t, uuid.Nil.String(), billingPackageLedgerScope(uuid.Nil),
		"organization scope must not render as the all-zero ledger id")

	ledgerID := uuid.New()
	assert.Equal(t, ledgerID.String(), billingPackageLedgerScope(ledgerID))
}

func TestGetBillingPackageByID_OrgScope_AsksForTheEmptySentinel(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	bpID := uuid.New()
	orgID := uuid.New()

	mockRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(billing_package.AnyLedger)).
		Return(&model.BillingPackage{ID: bpID.String(), LedgerID: uuid.New().String()}, nil)

	got, err := svc.GetBillingPackageByID(context.Background(), bpID, orgID, uuid.Nil)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestGetBillingPackageByID_CrossLedger_IsNotFound(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	bpID := uuid.New()
	orgID := uuid.New()
	ledgerB := uuid.New()

	// The repository answers a cross-ledger by-ID read exactly as it answers a
	// nonexistent id.
	mockRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(ledgerB.String())).
		Return(nil, mongo.ErrNoDocuments)

	got, err := svc.GetBillingPackageByID(context.Background(), bpID, orgID, ledgerB)
	require.Error(t, err)
	assert.Nil(t, got)

	assertBillingPackageAbsent(t, err, constant.ErrBillingPackageNotFound.Error())
}

func TestUpdateBillingPackage_OwningLedger_PassesLedgerToRepository(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	bpID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockRepo.EXPECT().
		Update(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(ledgerID.String()), gomock.Any()).
		Return(&model.BillingPackage{
			ID:        bpID.String(),
			LedgerID:  ledgerID.String(),
			Label:     "Renamed Through Its Own Ledger",
			UpdatedAt: "2026-01-02T00:00:00Z",
		}, nil)

	got, err := svc.UpdateBillingPackage(context.Background(), bpID, orgID, ledgerID,
		map[string]any{"label": "Renamed Through Its Own Ledger"})
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestUpdateBillingPackage_CrossLedger_IsNotFoundAndDoesNotEmit(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	mockEmitter := pkgStreaming.NewMockEmitter()
	svc.Streaming = mockEmitter

	bpID := uuid.New()
	orgID := uuid.New()
	ledgerB := uuid.New()

	// The ledger clause on the write is the whole enforcement here: no document
	// matches, so the repository reports the same absence a missing id produces.
	mockRepo.EXPECT().
		Update(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(ledgerB.String()), gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "BillingPackage"))

	got, err := svc.UpdateBillingPackage(context.Background(), bpID, orgID, ledgerB,
		map[string]any{"label": "Renamed Through Ledger B"})
	require.Error(t, err)
	assert.Nil(t, got)

	assertBillingPackageAbsent(t, err, constant.ErrEntityNotFound.Error())
	assert.Empty(t, mockEmitter.Events(), "a rejected cross-ledger update must not emit an updated event")
}

func TestDeleteBillingPackage_PassesCallerLedgerToRepository(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	bpID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(ledgerID.String())).
		Return(&model.BillingPackage{ID: bpID.String(), LedgerID: ledgerID.String()}, nil)
	mockRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(ledgerID.String())).
		Return(nil)

	require.NoError(t, svc.DeleteBillingPackage(context.Background(), bpID, orgID, ledgerID))
}

func TestDeleteBillingPackage_CrossLedger_IsNotFoundAndDoesNotEmit(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	mockEmitter := pkgStreaming.NewMockEmitter()
	svc.Streaming = mockEmitter

	bpID := uuid.New()
	orgID := uuid.New()
	ledgerB := uuid.New()

	mockRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(ledgerB.String())).
		Return(nil, mongo.ErrNoDocuments)
	mockRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Eq(bpID.String()), gomock.Eq(orgID.String()), gomock.Eq(ledgerB.String())).
		Return(pkg.ValidateBusinessError(constant.ErrEntityNotFound, "BillingPackage"))

	err := svc.DeleteBillingPackage(context.Background(), bpID, orgID, ledgerB)
	require.Error(t, err)

	assertBillingPackageAbsent(t, err, constant.ErrBillingPackageNotFound.Error())
	assert.Empty(t, mockEmitter.Events(), "a rejected cross-ledger delete must not emit a deleted event")
}

// assertBillingPackageAbsent asserts an error reports the package as absent — the
// same answer a nonexistent id produces — and reveals nothing about another ledger.
func assertBillingPackageAbsent(t *testing.T, err error, wantCode string) {
	t.Helper()

	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound, "a cross-ledger by-ID access must map to not found, never to forbidden")
	assert.Equal(t, wantCode, notFound.Code)

	assert.NotContains(t, err.Error(), "ledger")
	assert.NotContains(t, err.Error(), "Ledger")
}
