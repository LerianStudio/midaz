// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	libPointers "github.com/LerianStudio/lib-commons/v7/commons/pointers"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// fakeHolderByIDReader stubs GetHolderByID for the Exists discrimination test.
type fakeHolderByIDReader struct {
	holder *mmodel.Holder
	err    error
}

func (f fakeHolderByIDReader) GetHolderByID(_ context.Context, _ string, _ uuid.UUID, _ bool) (*mmodel.Holder, error) {
	return f.holder, f.err
}

func TestHolderReaderAdapter_Exists(t *testing.T) {
	id := uuid.New()
	holder := &mmodel.Holder{ID: &id}

	infraErr := errors.New("mongo timeout")

	tests := []struct {
		name       string
		reader     fakeHolderByIDReader
		wantExists bool
		wantErr    bool
		wantErrIs  error
	}{
		{
			name:       "holder found",
			reader:     fakeHolderByIDReader{holder: holder},
			wantExists: true,
		},
		{
			name: "holder-not-found business error maps to (false, nil)",
			reader: fakeHolderByIDReader{err: pkg.EntityNotFoundError{
				Code: constant.ErrHolderNotFound.Error(),
			}},
			wantExists: false,
		},
		{
			name: "different EntityNotFoundError code propagates",
			reader: fakeHolderByIDReader{err: pkg.EntityNotFoundError{
				Code: constant.ErrOrganizationIDNotFound.Error(),
			}},
			wantExists: false,
			wantErr:    true,
		},
		{
			name:       "infrastructure error propagates",
			reader:     fakeHolderByIDReader{err: infraErr},
			wantExists: false,
			wantErr:    true,
			wantErrIs:  infraErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := holderReaderAdapter{service: tt.reader}

			exists, err := adapter.Exists(context.Background(), "org-1", id)

			assert.Equal(t, tt.wantExists, exists)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
		})
	}
}

// ctxCapturingHolderReader records the context GetHolderByID receives, so a test can
// assert which CRM database the holder read resolves.
type ctxCapturingHolderReader struct {
	called bool
	ctx    context.Context
}

func (r *ctxCapturingHolderReader) GetHolderByID(ctx context.Context, _ string, id uuid.UUID, _ bool) (*mmodel.Holder, error) {
	r.called = true
	r.ctx = ctx

	return &mmodel.Holder{ID: &id}, nil
}

// fakeCRMTenantDatabase stubs the CRM Mongo manager's per-tenant database resolution.
type fakeCRMTenantDatabase struct {
	db       *mongo.Database
	err      error
	tenantID string
}

func (f *fakeCRMTenantDatabase) GetDatabaseForTenant(_ context.Context, tenantID string) (*mongo.Database, error) {
	f.tenantID = tenantID

	return f.db, f.err
}

// TestHolderReaderAdapter_ExistsMultiTenant pins the CRM database resolution the holder
// existence check performs in multi-tenant mode. The check runs inside account create, whose
// route middleware injects only the module-keyed onboarding and transaction stores, so the
// CRM holder repo would find no Mongo on the generic key and fail the request with a 500.
func TestHolderReaderAdapter_ExistsMultiTenant(t *testing.T) {
	id := uuid.New()
	crmDB := (&mongo.Client{}).Database("crm_tenant_a")
	otherDB := (&mongo.Client{}).Database("fees_tenant_a")

	t.Run("resolves the tenant CRM database onto the generic key for the holder read", func(t *testing.T) {
		reader := &ctxCapturingHolderReader{}
		resolver := &fakeCRMTenantDatabase{db: crmDB}
		adapter := holderReaderAdapter{service: reader, crmTenantDB: resolver}

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

		exists, err := adapter.Exists(ctx, "org-1", id)
		require.NoError(t, err)
		assert.True(t, exists)

		assert.Equal(t, "tenant-a", resolver.tenantID, "the CRM database must be resolved for the caller's tenant")
		require.True(t, reader.called)
		assert.Same(t, crmDB, tmcore.GetMBContext(reader.ctx),
			"the holder read must see the tenant CRM database on the generic key the CRM repos read")
		assert.Nil(t, tmcore.GetMBContext(ctx), "the resolution must not leak onto the caller's context")
	})

	t.Run("replaces a generic Mongo already on the context", func(t *testing.T) {
		reader := &ctxCapturingHolderReader{}
		adapter := holderReaderAdapter{service: reader, crmTenantDB: &fakeCRMTenantDatabase{db: crmDB}}

		ctx := tmcore.ContextWithMB(tmcore.ContextWithTenantID(context.Background(), "tenant-a"), otherDB)

		_, err := adapter.Exists(ctx, "org-1", id)
		require.NoError(t, err)
		assert.Same(t, crmDB, tmcore.GetMBContext(reader.ctx),
			"a generic Mongo bound by another route must not answer the holder read")
	})

	t.Run("missing tenant id fails without reading", func(t *testing.T) {
		reader := &ctxCapturingHolderReader{}
		adapter := holderReaderAdapter{service: reader, crmTenantDB: &fakeCRMTenantDatabase{db: crmDB}}

		exists, err := adapter.Exists(context.Background(), "org-1", id)
		require.Error(t, err)
		assert.False(t, exists)
		assert.False(t, reader.called, "without a tenant the holder read must not fall through to any store")
	})

	t.Run("tenant resolution failure maps to tenant service unavailable", func(t *testing.T) {
		reader := &ctxCapturingHolderReader{}
		adapter := holderReaderAdapter{service: reader, crmTenantDB: &fakeCRMTenantDatabase{err: errors.New("tenant-manager down")}}

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

		exists, err := adapter.Exists(ctx, "org-1", id)
		require.Error(t, err)
		assert.False(t, exists)
		assert.False(t, reader.called)

		var unavailable pkg.ServiceUnavailableError
		require.ErrorAs(t, err, &unavailable)
		assert.Equal(t, constant.ErrTenantServiceUnavailable.Error(), unavailable.Code)
	})
}

func TestHolderAccountsReaderAdapter_ListAccountsByHolder(t *testing.T) {
	organizationID := uuid.New()
	holderID := uuid.New()
	ledgerID := uuid.New()

	found := []*mmodel.Account{{ID: "acc1"}}
	repoErr := errors.New("connection refused")

	tests := []struct {
		name string
		// ledgerIDParam is the raw ?ledger_id= value; nil means the parameter was absent.
		ledgerIDParam *string
		expect        func(*account.MockRepository)
		wantAccounts  []*mmodel.Account
		wantErrCode   string
		wantErrText   string
	}{
		{
			name:          "no ledger_id lists across every ledger",
			ledgerIDParam: nil,
			expect: func(repo *account.MockRepository) {
				repo.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), gomock.Any(), mmodel.HolderOnV2).
					Return(found, nil).
					Times(1)
			},
			wantAccounts: found,
		},
		{
			name:          "empty ledger_id counts as absent",
			ledgerIDParam: libPointers.String(""),
			expect: func(repo *account.MockRepository) {
				repo.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), gomock.Any(), mmodel.HolderOnV2).
					Return(found, nil).
					Times(1)
			},
			wantAccounts: found,
		},
		{
			name:          "valid ledger_id narrows the listing",
			ledgerIDParam: libPointers.String(ledgerID.String()),
			expect: func(repo *account.MockRepository) {
				repo.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, matchLedgerIDPtr(ledgerID), gomock.Any(), mmodel.HolderOnV2).
					Return(found, nil).
					Times(1)
			},
			wantAccounts: found,
		},
		{
			name:          "malformed ledger_id is a query parameter validation error",
			ledgerIDParam: libPointers.String("not-a-uuid"),
			expect: func(repo *account.MockRepository) {
				repo.EXPECT().FindAllByHolder(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			wantErrCode: constant.ErrInvalidQueryParameter.Error(),
		},
		{
			name:          "repository error propagates",
			ledgerIDParam: nil,
			expect: func(repo *account.MockRepository) {
				repo.EXPECT().
					FindAllByHolder(gomock.Any(), organizationID, holderID, gomock.Nil(), gomock.Any(), mmodel.HolderOnV2).
					Return(nil, repoErr).
					Times(1)
			},
			wantErrText: repoErr.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			accountRepo := account.NewMockRepository(ctrl)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			tt.expect(accountRepo)

			if len(tt.wantAccounts) > 0 {
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), constant.EntityAccount, gomock.Any()).
					Return(nil, nil).
					Times(1)
			}

			adapter := holderAccountsReaderAdapter{
				query: &query.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo},
			}

			filter := http.QueryHeader{Limit: 10, Page: 1, SortOrder: "asc", LedgerID: tt.ledgerIDParam}

			got, err := adapter.ListAccountsByHolder(context.Background(), organizationID.String(), holderID, filter)

			switch {
			case tt.wantErrCode != "":
				require.Error(t, err)

				var validation pkg.ValidationError
				require.ErrorAs(t, err, &validation)
				assert.Equal(t, tt.wantErrCode, validation.Code)
				assert.Nil(t, got)
			case tt.wantErrText != "":
				require.EqualError(t, err, tt.wantErrText)
				assert.Nil(t, got)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.wantAccounts, got)
			}
		})
	}
}

// TestHolderAccountsReaderAdapter_RejectsMalformedOrganizationID pins the org
// parse guard, which runs before any ledger_id handling.
func TestHolderAccountsReaderAdapter_RejectsMalformedOrganizationID(t *testing.T) {
	adapter := holderAccountsReaderAdapter{query: &query.UseCase{}}

	got, err := adapter.ListAccountsByHolder(context.Background(), "not-a-uuid", uuid.New(), http.QueryHeader{})

	require.Error(t, err)
	assert.Nil(t, got)

	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, constant.ErrOrganizationIDNotFound.Error(), notFound.Code)
}

// matchLedgerIDPtr matches a *uuid.UUID by the value it points at.
func matchLedgerIDPtr(want uuid.UUID) gomock.Matcher {
	return gomock.Cond(func(arg *uuid.UUID) bool {
		return arg != nil && *arg == want
	})
}
