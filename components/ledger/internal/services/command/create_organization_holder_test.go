// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// stubHolderProvisioner is a hand-rolled HolderProvisioner stub that records the
// arguments of the self-holder provisioning call.
type stubHolderProvisioner struct {
	err      error
	calls    int
	gotOrgID string
	gotID    uuid.UUID
	gotInput *mmodel.CreateHolderInput
}

func (s *stubHolderProvisioner) CreateHolderWithID(_ context.Context, organizationID string, id uuid.UUID, chi *mmodel.CreateHolderInput) (*mmodel.Holder, error) {
	s.calls++
	s.gotOrgID = organizationID
	s.gotID = id
	s.gotInput = chi

	if s.err != nil {
		return nil, s.err
	}

	return &mmodel.Holder{ID: &id, Type: chi.Type, Name: &chi.Name, Document: &chi.Document}, nil
}

func newOrgUseCaseForHolder(ctrl *gomock.Controller, prov *stubHolderProvisioner, orgID string) *UseCase {
	mockRepo := organization.NewMockRepository(ctrl)

	mockRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Organization) (*mmodel.Organization, error) {
			out := *in
			out.ID = orgID

			return &out, nil
		})

	return &UseCase{
		OrganizationRepo:  mockRepo,
		HolderProvisioner: prov,
	}
}

// TestCreateOrganizationProvisionsSelfHolder asserts the eager self-holder is
// provisioned with the deterministic UUIDv5 derived from the org ID, a LEGAL_PERSON
// type, and the org's legal name/document.
func TestCreateOrganizationProvisionsSelfHolder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	prov := &stubHolderProvisioner{}
	uc := newOrgUseCaseForHolder(ctrl, prov, orgID.String())

	org, err := uc.CreateOrganization(context.Background(), &mmodel.CreateOrganizationInput{
		LegalName:     "Acme Ltd",
		LegalDocument: "123456789",
		Status:        mmodel.Status{Code: "ACTIVE"},
	})

	require.NoError(t, err)
	require.NotNil(t, org)
	require.Equal(t, 1, prov.calls)

	assert.Equal(t, orgID.String(), prov.gotOrgID)
	assert.Equal(t, deriveSelfHolderID(orgID), prov.gotID)
	require.NotNil(t, prov.gotInput)
	require.NotNil(t, prov.gotInput.Type)
	assert.Equal(t, "LEGAL_PERSON", *prov.gotInput.Type)
	assert.Equal(t, "Acme Ltd", prov.gotInput.Name)
	assert.Equal(t, "123456789", prov.gotInput.Document)
}

// TestCreateOrganizationSelfHolderNonFatal proves a provisioning failure does not
// fail the organization create (non-fatal post-commit hook, R34).
func TestCreateOrganizationSelfHolderNonFatal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	prov := &stubHolderProvisioner{err: errors.New("mongo unavailable")}
	uc := newOrgUseCaseForHolder(ctrl, prov, orgID.String())

	org, err := uc.CreateOrganization(context.Background(), &mmodel.CreateOrganizationInput{
		LegalName:     "Acme Ltd",
		LegalDocument: "123456789",
		Status:        mmodel.Status{Code: "ACTIVE"},
	})

	require.NoError(t, err, "self-holder provisioning failure must not fail org create")
	require.NotNil(t, org)
	assert.Equal(t, 1, prov.calls)
}

// TestCreateOrganizationNilProvisioner proves a nil provisioner is a no-op and the
// org create still succeeds (the backfill runner is the repair path).
func TestCreateOrganizationNilProvisioner(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := organization.NewMockRepository(ctrl)
	mockRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Organization) (*mmodel.Organization, error) {
			out := *in
			out.ID = uuid.New().String()

			return &out, nil
		})

	// HolderProvisioner left unset (nil interface) to exercise the no-op guard.
	uc := &UseCase{OrganizationRepo: mockRepo}

	org, err := uc.CreateOrganization(context.Background(), &mmodel.CreateOrganizationInput{
		LegalName:     "Acme Ltd",
		LegalDocument: "123456789",
		Status:        mmodel.Status{Code: "ACTIVE"},
	})

	require.NoError(t, err)
	require.NotNil(t, org)
}
