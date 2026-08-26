// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// This file locks the ctx-cancellation guard on the two create cores that open a
// resource. Both guards are the FIRST statement of their core, ahead of
// NewTrackingFromContext, matching the fee cores (fee_estimate_core.go /
// fee_package_core.go): a context the caller has already abandoned buys no span, no
// service call and no write.
//
// The guards live in the CORES, not the shells, because each core is shared by the /v1
// and /v2 handlers — one guard covers both contracts, and a future version group
// inherits it.
//
// The assertion that matters in each case is the .Times(0) on the repository: the
// command must not be entered at all. For the organization that transitively covers the
// self-holder provisioning too, which is why provisionSelfHolder needs no guard of its
// own — it is unreachable when the command never runs. (Its own post-commit,
// non-fatal position is why a guard THERE would change nothing: either way no holder is
// written, and the idempotent backfill runner is the repair path.)

// countingHolderProvisioner records whether the self-holder port was reached.
type countingHolderProvisioner struct {
	calls int
}

func (p *countingHolderProvisioner) CreateHolderWithID(_ context.Context, _ string, id uuid.UUID, chi *mmodel.CreateHolderInput) (*mmodel.Holder, error) {
	p.calls++

	return &mmodel.Holder{ID: &id, Type: chi.Type, Name: &chi.Name, Document: &chi.Document}, nil
}

var _ command.HolderProvisioner = (*countingHolderProvisioner)(nil)

// TestCreateOrganizationCore_CanceledContext proves a canceled context returns
// context.Canceled from the core and reaches NEITHER the organization repository NOR the
// self-holder provisioner.
func TestCreateOrganizationCore_CanceledContext(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgRepo := organization.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	orgRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Times(0)
	metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	prov := &countingHolderProvisioner{}

	handler := &OrganizationHandler{Command: &command.UseCase{
		OrganizationRepo:       orgRepo,
		OnboardingMetadataRepo: metadataRepo,
		HolderProvisioner:      prov,
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	org, err := handler.createOrganization(ctx, &mmodel.CreateOrganizationInput{
		LegalName:     "Acme Ltd",
		LegalDocument: "12345678901234",
		Status:        mmodel.Status{Code: "ACTIVE"},
	}, command.HolderOnV2)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, org)
	assert.Zero(t, prov.calls, "the self-holder provisioner must not be reached on a canceled context")
}

// TestCreateAccountCore_CanceledContext proves a canceled context returns
// context.Canceled from the core and reaches no repository. The policy argument is
// irrelevant here — the guard sits ahead of the holder seam — so HolderOnV2 is passed to
// prove the guard wins even on the contract that DOES run the seam.
func TestCreateAccountCore_CanceledContext(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	accountRepo := account.NewMockRepository(ctrl)
	assetRepo := asset.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)
	ledgerRepo := ledger.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	accountRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Times(0)
	accountRepo.EXPECT().FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	balanceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Times(0)
	ledgerRepo.EXPECT().GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	metadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	handler := &AccountHandler{Command: &command.UseCase{
		AccountRepo:            accountRepo,
		AssetRepo:              assetRepo,
		BalanceRepo:            balanceRepo,
		LedgerRepo:             ledgerRepo,
		OnboardingMetadataRepo: metadataRepo,
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	acc, err := handler.createAccount(ctx, uuid.New(), uuid.New(), &mmodel.CreateAccountInput{
		Name:      "Checking",
		Type:      "deposit",
		AssetCode: "USD",
	}, "Bearer test", command.HolderOnV2)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, acc)
}
