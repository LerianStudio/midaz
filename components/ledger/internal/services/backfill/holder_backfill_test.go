// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package backfill

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	libHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProvisioner records provisioning calls and optionally fails.
type fakeProvisioner struct {
	calls   []provisionCall
	failErr error
}

type provisionCall struct {
	organizationID string
	id             uuid.UUID
}

func (f *fakeProvisioner) CreateHolderWithID(_ context.Context, organizationID string, id uuid.UUID, _ *mmodel.CreateHolderInput) (*mmodel.Holder, error) {
	f.calls = append(f.calls, provisionCall{organizationID: organizationID, id: id})
	if f.failErr != nil {
		return nil, f.failErr
	}

	return &mmodel.Holder{ID: &id}, nil
}

// fakeOrgLister returns a fixed set of organizations on the first page only.
type fakeOrgLister struct {
	orgs  []*mmodel.Organization
	calls int
}

func (f *fakeOrgLister) FindAll(_ context.Context, _ libHTTP.QueryHeader) ([]*mmodel.Organization, error) {
	f.calls++
	if f.calls > 1 {
		return nil, nil
	}

	return f.orgs, nil
}

func TestBuildMaterialiseQuery_Invariants(t *testing.T) {
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	selfHolderID := command.DeriveSelfHolderID(orgID)

	query, args, err := buildMaterialiseQuery(orgID, selfHolderID)
	require.NoError(t, err)

	lower := strings.ToLower(query)

	assert.Contains(t, lower, "update account set holder_id", "must set holder_id on account")
	assert.Contains(t, lower, "holder_id is null", "must only touch rows still NULL (idempotency)")
	assert.Contains(t, lower, "deleted_at is null", "must skip soft-deleted rows")
	assert.Contains(t, lower, "lower(type) <>", "must exempt @external accounts by type")
	assert.Contains(t, query, "$1", "must use Dollar placeholders for PostgreSQL")

	// The bound args carry the self-holder id (first SET), the org id, and the
	// 'external' literal. uuid.UUID serialises via its driver.Valuer, so the org
	// id is bound as its canonical string form.
	require.Len(t, args, 3, "set value + org id + external literal")
	assert.Equal(t, selfHolderID, args[0], "self-holder id is the SET value")
	assert.Contains(t, args, "external", "external type literal is bound")
	assert.Contains(t, args, orgID.String(), "organization id is bound as its string form")
}

func TestListAllOrgsFilter_WidensDateBounds(t *testing.T) {
	filter := listAllOrgsFilter(3)

	assert.Equal(t, orgPageSize, filter.Limit)
	assert.Equal(t, 3, filter.Page)
	assert.Equal(t, "asc", filter.SortOrder)
	// A zero EndDate would exclude every row from FindAll's created_at filter.
	assert.True(t, filter.EndDate.After(time.Now()), "end date must be in the future to include all orgs")
	assert.True(t, filter.StartDate.Before(time.Now()), "start date must precede all orgs")
}

func TestRunTenant_ProvisioningFailureAbortsBeforePG(t *testing.T) {
	orgID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	provErr := errors.New("mongo down")

	prov := &fakeProvisioner{failErr: provErr}
	orgs := &fakeOrgLister{orgs: []*mmodel.Organization{{ID: orgID.String(), LegalName: "Acme", LegalDocument: "doc"}}}

	b := NewHolderBackfiller(orgs, prov)

	_, err := b.RunTenant(context.Background())

	// The Mongo provisioning error must surface unchanged. If the runner had
	// proceeded to the PG step, it would have failed instead with the
	// "onboarding postgres connection missing" error (no DB in ctx) — proving
	// the Mongo-first ordering aborts before any PG write.
	require.Error(t, err)
	assert.ErrorIs(t, err, provErr)
	assert.Len(t, prov.calls, 1, "self-holder provisioning attempted exactly once")
}

func TestRunTenant_DerivesDeterministicSelfHolderPerOrg(t *testing.T) {
	orgA := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	prov := &fakeProvisioner{}
	orgs := &fakeOrgLister{orgs: []*mmodel.Organization{{ID: orgA.String(), LegalName: "Acme", LegalDocument: "doc"}}}

	b := NewHolderBackfiller(orgs, prov)

	// The PG materialisation will fail (no DB in ctx), but the Mongo provisioning
	// runs first, so we can assert it used the deterministic derived ID.
	_, _ = b.RunTenant(context.Background())

	require.Len(t, prov.calls, 1)
	assert.Equal(t, orgA.String(), prov.calls[0].organizationID)
	assert.Equal(t, command.DeriveSelfHolderID(orgA), prov.calls[0].id,
		"backfill must derive the SAME self-holder ID as the create/org-provision paths")
}
