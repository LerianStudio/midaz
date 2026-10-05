// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	detachBillingA = "0190a000-0000-7000-8000-0000000000c1"
	detachBillingB = "0190a000-0000-7000-8000-0000000000d1"
)

func detachStr(v string) *string { return &v }

func detachBilling(id string, enable bool, mutate func(bp *model.BillingPackage)) *model.BillingPackage {
	bp := &model.BillingPackage{
		ID:             id,
		OrganizationID: detachOrgID.String(),
		LedgerID:       detachLedgerID.String(),
		Type:           model.BillingPackageTypeMaintenance,
		Enable:         &enable,
		CreatedAt:      "2026-03-01T10:00:00Z",
		UpdatedAt:      "2026-03-01T10:00:00Z",
	}

	mutate(bp)

	return bp
}

// expectBillingUpdate captures the write the detach sends for id.
func (h *detachHarness) expectBillingUpdate(id string, captured *bson.M) *gomock.Call {
	return h.billingRepo.EXPECT().
		Update(gomock.Any(), id, detachOrgID.String(), detachLedgerID.String(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id, _, _ string, fields *bson.M) (*model.BillingPackage, error) {
			*captured = *fields

			return detachBilling(id, false, func(*model.BillingPackage) {}), nil
		})
}

func billingSetOf(t *testing.T, fields bson.M) bson.M {
	t.Helper()

	set, ok := fields["$set"].(bson.M)
	require.True(t, ok, "write must carry $set")

	updatedAt, ok := set["updated_at"].(string)
	require.True(t, ok, "billing updated_at is an RFC3339 string")

	_, err := time.Parse(time.RFC3339, updatedAt)
	require.NoError(t, err)

	return set
}

func TestDetachBillingAccountAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		bp          *model.BillingPackage
		want        model.FeeAliasDetachResult
		wantUnset   any
		wantPull    any
		wantEnable  any
		wantWritten bool
	}{
		{
			name: "volume debit leg is unset and disables the package",
			bp: detachBilling(detachBillingA, true, func(bp *model.BillingPackage) {
				bp.Type = model.BillingPackageTypeVolume
				bp.DebitAccountAlias = detachStr(detachAlias)
				bp.CreditAccountAlias = detachStr(detachOtherAlias)
			}),
			want:        model.FeeAliasDetachResult{PackagesUpdated: 1, PackagesDisabled: 1},
			wantUnset:   bson.M{"debit_account_alias": ""},
			wantEnable:  false,
			wantWritten: true,
		},
		{
			name: "volume credit leg is unset and disables the package",
			bp: detachBilling(detachBillingA, true, func(bp *model.BillingPackage) {
				bp.Type = model.BillingPackageTypeVolume
				bp.DebitAccountAlias = detachStr(detachOtherAlias)
				bp.CreditAccountAlias = detachStr(detachAlias)
			}),
			want:        model.FeeAliasDetachResult{PackagesUpdated: 1, PackagesDisabled: 1},
			wantUnset:   bson.M{"credit_account_alias": ""},
			wantEnable:  false,
			wantWritten: true,
		},
		{
			name: "maintenance credit account is unset and disables the package",
			bp: detachBilling(detachBillingA, true, func(bp *model.BillingPackage) {
				bp.MaintenanceCreditAccount = detachStr(detachAlias)
				bp.AccountTarget = &model.AccountTarget{Aliases: []string{detachOtherAlias}}
			}),
			want:        model.FeeAliasDetachResult{PackagesUpdated: 1, PackagesDisabled: 1},
			wantUnset:   bson.M{"maintenance_credit_account": ""},
			wantEnable:  false,
			wantWritten: true,
		},
		{
			name: "target alias is pulled and the package stays enabled while others remain",
			bp: detachBilling(detachBillingA, true, func(bp *model.BillingPackage) {
				bp.MaintenanceCreditAccount = detachStr(detachOtherAlias)
				bp.AccountTarget = &model.AccountTarget{Aliases: []string{detachAlias, "@other"}}
			}),
			want:        model.FeeAliasDetachResult{PackagesUpdated: 1},
			wantPull:    bson.M{"account_target.aliases": detachAlias},
			wantWritten: true,
		},
		{
			name: "last target alias is pulled and disables the package",
			bp: detachBilling(detachBillingA, true, func(bp *model.BillingPackage) {
				bp.MaintenanceCreditAccount = detachStr(detachOtherAlias)
				bp.AccountTarget = &model.AccountTarget{Aliases: []string{detachAlias}}
			}),
			want:        model.FeeAliasDetachResult{PackagesUpdated: 1, PackagesDisabled: 1},
			wantPull:    bson.M{"account_target.aliases": detachAlias},
			wantEnable:  false,
			wantWritten: true,
		},
		{
			name: "disabled package is cleaned and stays disabled",
			bp: detachBilling(detachBillingA, false, func(bp *model.BillingPackage) {
				bp.MaintenanceCreditAccount = detachStr(detachAlias)
				bp.AccountTarget = &model.AccountTarget{Aliases: []string{detachOtherAlias}}
			}),
			want:        model.FeeAliasDetachResult{PackagesUpdated: 1},
			wantUnset:   bson.M{"maintenance_credit_account": ""},
			wantEnable:  false,
			wantWritten: true,
		},
		{
			name: "segment target is not changed",
			bp: detachBilling(detachBillingA, true, func(bp *model.BillingPackage) {
				segmentID := uuid.MustParse("0190a000-0000-7000-8000-0000000000e1")
				bp.MaintenanceCreditAccount = detachStr(detachOtherAlias)
				bp.AccountTarget = &model.AccountTarget{SegmentID: &segmentID}
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newDetachHarness(t)

			h.packRepo.EXPECT().
				FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
				Return(nil, nil)
			h.expectBillingPackages(tt.bp)

			var captured bson.M
			if tt.wantWritten {
				h.expectBillingUpdate(detachBillingA, &captured)
			}

			got, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Zero(t, h.cache.delCalls, "billing does not use the fee package cache")

			if !tt.wantWritten {
				assert.Empty(t, h.emitter.Events())

				return
			}

			set := billingSetOf(t, captured)
			assert.Equal(t, tt.wantEnable, set["enable"])
			assert.Equal(t, tt.wantUnset, captured["$unset"])
			assert.Equal(t, tt.wantPull, captured["$pull"])
			assert.Equal(t, 1, countEvents(h.emitter, "fee_billing_packages.updated"))
		})
	}
}

func TestDetachBillingAccountAlias_EventPerPackageAndSkipsDeleted(t *testing.T) {
	t.Parallel()

	h := newDetachHarness(t)

	legOnly := func(id string) *model.BillingPackage {
		return detachBilling(id, true, func(bp *model.BillingPackage) {
			bp.MaintenanceCreditAccount = detachStr(detachAlias)
			bp.AccountTarget = &model.AccountTarget{Aliases: []string{detachOtherAlias}}
		})
	}

	h.billingRepo.EXPECT().
		FindNotDeletedByLedger(gomock.Any(), detachOrgID.String(), detachLedgerID.String()).
		Return([]*model.BillingPackage{legOnly(detachBillingA), legOnly(detachBillingB), legOnly(detachBillingA)}, nil)

	var captured bson.M

	h.expectBillingUpdate(detachBillingA, &captured)
	h.expectBillingUpdate(detachBillingB, &captured)
	// The third entry was deleted between the listing and the write.
	h.billingRepo.EXPECT().
		Update(gomock.Any(), detachBillingA, detachOrgID.String(), detachLedgerID.String(), gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "BillingPackage", feeconstant.BillingPackageCollection))

	updated, disabled, err := h.uc.BillingPackages.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
	require.NoError(t, err)
	assert.Equal(t, 2, updated)
	assert.Equal(t, 2, disabled)
	assert.Equal(t, 2, countEvents(h.emitter, "fee_billing_packages.updated"))
}

func TestDetachAccountAlias_SumsFeeAndBillingPackages(t *testing.T) {
	t.Parallel()

	h := newDetachHarness(t)

	h.packRepo.EXPECT().
		FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
		Return([]*pack.Package{detachPack(detachPackA, true, map[string]model.Fee{
			"adm": detachFee(detachAlias), "cust": detachFee(detachOtherAlias),
		}, nil)}, nil)

	var capturedPack, capturedBilling bson.M

	h.expectPackUpdate(detachPackA, detachReadAt, &capturedPack)
	h.expectBillingPackages(detachBilling(detachBillingA, true, func(bp *model.BillingPackage) {
		bp.MaintenanceCreditAccount = detachStr(detachAlias)
		bp.AccountTarget = &model.AccountTarget{Aliases: []string{detachOtherAlias}}
	}))
	h.expectBillingUpdate(detachBillingA, &capturedBilling)

	got, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
	require.NoError(t, err)
	assert.Equal(t, model.FeeAliasDetachResult{PackagesUpdated: 2, PackagesDisabled: 1}, got)
	assert.Equal(t, 1, h.cache.delCalls)
	assert.Equal(t, 1, countEvents(h.emitter, "fee_packages.updated"))
	assert.Equal(t, 1, countEvents(h.emitter, "fee_billing_packages.updated"))
}

func TestDetachBillingAccountAlias_ErrorsPropagate(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("mongo unavailable")

	t.Run("list", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().
			FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return(nil, nil)
		h.billingRepo.EXPECT().
			FindNotDeletedByLedger(gomock.Any(), detachOrgID.String(), detachLedgerID.String()).
			Return(nil, errBoom)

		_, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.ErrorIs(t, err, errBoom)
	})

	t.Run("update", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().
			FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return(nil, nil)
		h.expectBillingPackages(detachBilling(detachBillingA, true, func(bp *model.BillingPackage) {
			bp.MaintenanceCreditAccount = detachStr(detachAlias)
		}))
		h.billingRepo.EXPECT().
			Update(gomock.Any(), detachBillingA, detachOrgID.String(), detachLedgerID.String(), gomock.Any()).
			Return(nil, errBoom)

		_, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.ErrorIs(t, err, errBoom)
		assert.Empty(t, h.emitter.Events())
	})

	t.Run("fee pack failure never reaches billing", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().
			FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return(nil, errBoom)

		_, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.ErrorIs(t, err, errBoom)
	})
}

func TestDetachAccountAlias_NilBillingPackagesFailsClosed(t *testing.T) {
	t.Parallel()

	h := newDetachHarness(t)
	h.uc.BillingPackages = nil

	_, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
	require.ErrorIs(t, err, errNilBillingPackages)
	assert.False(t, pkg.IsBusinessError(err))
}
