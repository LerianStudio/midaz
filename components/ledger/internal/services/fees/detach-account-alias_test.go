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
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

const (
	detachAlias      = "@dead"
	detachOtherAlias = "@alive"
)

var (
	detachOrgID    = uuid.MustParse("0190a000-0000-7000-8000-000000000001")
	detachLedgerID = uuid.MustParse("0190a000-0000-7000-8000-000000000002")
	detachPackA    = uuid.MustParse("0190a000-0000-7000-8000-0000000000a1")
	detachPackB    = uuid.MustParse("0190a000-0000-7000-8000-0000000000b1")
	detachReadAt   = time.Date(2026, time.March, 1, 10, 0, 0, 0, time.UTC)
	detachFreshAt  = time.Date(2026, time.March, 1, 10, 0, 5, 0, time.UTC)
)

type detachHarness struct {
	uc       *UseCase
	packRepo *pack.MockRepository
	cache    *fakePackageCache
	emitter  *pkgStreaming.MockEmitter
}

func newDetachHarness(t *testing.T) *detachHarness {
	t.Helper()

	ctrl := gomock.NewController(t)

	h := &detachHarness{
		packRepo: pack.NewMockRepository(ctrl),
		cache:    newFakePackageCache(),
		emitter:  pkgStreaming.NewMockEmitter(),
	}

	h.uc = &UseCase{
		packageRepo:  h.packRepo,
		resolver:     feeshared.NewMockMidazResolver(ctrl),
		PackageCache: h.cache,
		Streaming:    h.emitter,
	}

	return h
}

func detachFee(creditAccount string) model.Fee {
	return model.Fee{FeeLabel: "fee", CreditAccount: creditAccount}
}

func detachPack(id uuid.UUID, enable bool, fees map[string]model.Fee, waived *[]string) *pack.Package {
	return &pack.Package{
		ID:             id,
		LedgerID:       detachLedgerID,
		Fees:           fees,
		WaivedAccounts: waived,
		Enable:         &enable,
		CreatedAt:      detachReadAt,
		UpdatedAt:      detachReadAt,
	}
}

// expectPackUpdate captures the write the detach sends for id under updatedAt.
func (h *detachHarness) expectPackUpdate(id uuid.UUID, updatedAt time.Time, captured *bson.M) *gomock.Call {
	return h.packRepo.EXPECT().
		Update(gomock.Any(), id, detachOrgID, detachLedgerID, updatedAt, gomock.Any()).
		DoAndReturn(func(_ context.Context, id, _, _ uuid.UUID, _ time.Time, fields *bson.M) (*pack.Package, error) {
			*captured = *fields

			return detachPack(id, false, nil, nil), nil
		})
}

func packConflict() error {
	return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", feeconstant.PackageCollection)
}

func setOf(t *testing.T, fields bson.M) bson.M {
	t.Helper()

	set, ok := fields["$set"].(bson.M)
	require.True(t, ok, "write must carry $set")
	assert.IsType(t, time.Time{}, set["updated_at"])

	return set
}

func countEvents(m *pkgStreaming.MockEmitter, key string) int {
	n := 0

	for _, e := range m.Events() {
		if e.DefinitionKey == key {
			n++
		}
	}

	return n
}

func TestDetachAccountAlias_FeePacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pack        *pack.Package
		want        model.FeeAliasDetachResult
		wantUnset   bson.M
		wantEnable  any
		wantWaived  any
		wantWritten bool
	}{
		{
			name: "removes only the fee crediting the alias and keeps enable",
			pack: detachPack(detachPackA, true, map[string]model.Fee{
				"adm": detachFee(detachAlias), "cust": detachFee(detachOtherAlias),
			}, nil),
			want:        model.FeeAliasDetachResult{PackagesUpdated: 1},
			wantUnset:   bson.M{"fees.adm": ""},
			wantWritten: true,
		},
		{
			name:        "disables a package left without fees",
			pack:        detachPack(detachPackA, true, map[string]model.Fee{"adm": detachFee(detachAlias)}, nil),
			want:        model.FeeAliasDetachResult{PackagesUpdated: 1, PackagesDisabled: 1},
			wantUnset:   bson.M{"fees.adm": ""},
			wantEnable:  false,
			wantWritten: true,
		},
		{
			name: "filters waivedAccounts without touching fees",
			pack: detachPack(detachPackA, true, map[string]model.Fee{"cust": detachFee(detachOtherAlias)},
				&[]string{detachOtherAlias, detachAlias}),
			want:        model.FeeAliasDetachResult{PackagesUpdated: 1},
			wantWaived:  []string{detachOtherAlias},
			wantWritten: true,
		},
		{
			name:        "cleans a disabled package and keeps it disabled",
			pack:        detachPack(detachPackA, false, map[string]model.Fee{"adm": detachFee(detachAlias)}, nil),
			want:        model.FeeAliasDetachResult{PackagesUpdated: 1},
			wantUnset:   bson.M{"fees.adm": ""},
			wantEnable:  false,
			wantWritten: true,
		},
		{
			name: "leaves a package that does not reference the alias untouched",
			pack: detachPack(detachPackA, true, map[string]model.Fee{"cust": detachFee(detachOtherAlias)},
				&[]string{detachOtherAlias}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newDetachHarness(t)

			h.packRepo.EXPECT().
				FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
				Return([]*pack.Package{tt.pack}, nil)

			var captured bson.M
			if tt.wantWritten {
				h.expectPackUpdate(detachPackA, detachReadAt, &captured)
			}

			got, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			if !tt.wantWritten {
				assert.Zero(t, h.cache.delCalls)
				assert.Empty(t, h.emitter.Events())

				return
			}

			set := setOf(t, captured)
			assert.Equal(t, tt.wantEnable, set["enable"])
			assert.Equal(t, tt.wantWaived, set["waived_accounts"])

			if tt.wantUnset == nil {
				assert.NotContains(t, captured, "$unset")
			} else {
				assert.Equal(t, tt.wantUnset, captured["$unset"])
			}

			assert.Equal(t, 1, h.cache.delCalls)
			assert.Equal(t, 1, countEvents(h.emitter, "fee_packages.updated"))
		})
	}
}

func TestDetachAccountAlias_FeePacksEventPerPackageAndOneInvalidation(t *testing.T) {
	t.Parallel()

	h := newDetachHarness(t)

	h.packRepo.EXPECT().
		FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
		Return([]*pack.Package{
			detachPack(detachPackA, true, map[string]model.Fee{"adm": detachFee(detachAlias)}, nil),
			detachPack(detachPackB, true, map[string]model.Fee{"cust": detachFee(detachOtherAlias)}, &[]string{detachAlias}),
		}, nil)

	var capturedA, capturedB bson.M

	h.expectPackUpdate(detachPackA, detachReadAt, &capturedA)
	h.expectPackUpdate(detachPackB, detachReadAt, &capturedB)

	got, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
	require.NoError(t, err)
	assert.Equal(t, model.FeeAliasDetachResult{PackagesUpdated: 2, PackagesDisabled: 1}, got)
	assert.Equal(t, 2, countEvents(h.emitter, "fee_packages.updated"))
	pkgStreaming.AssertEventEmitted(t, h.emitter, "fee_packages", "updated")
	assert.Equal(t, 1, h.cache.delCalls)
}

func TestDetachAccountAlias_FeePackConflict(t *testing.T) {
	t.Parallel()

	stale := func() *pack.Package {
		return detachPack(detachPackA, true, map[string]model.Fee{
			"adm": detachFee(detachAlias), "cust": detachFee(detachOtherAlias),
		}, nil)
	}

	t.Run("re-reads and retries after one conflict", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return([]*pack.Package{stale()}, nil)

		// A concurrent PATCH added a fee: the retry must not disable the package.
		fresh := detachPack(detachPackA, true, map[string]model.Fee{
			"adm": detachFee(detachAlias), "new": detachFee(detachOtherAlias),
		}, nil)
		fresh.UpdatedAt = detachFreshAt

		var captured bson.M

		gomock.InOrder(
			h.packRepo.EXPECT().Update(gomock.Any(), detachPackA, detachOrgID, detachLedgerID, detachReadAt, gomock.Any()).
				Return(nil, packConflict()),
			h.packRepo.EXPECT().FindByID(gomock.Any(), detachPackA, detachOrgID, detachLedgerID).Return(fresh, nil),
			h.expectPackUpdate(detachPackA, detachFreshAt, &captured),
		)

		got, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.NoError(t, err)
		assert.Equal(t, model.FeeAliasDetachResult{PackagesUpdated: 1}, got)
		assert.Equal(t, bson.M{"fees.adm": ""}, captured["$unset"])
		assert.NotContains(t, setOf(t, captured), "enable")
		assert.Equal(t, 1, h.cache.delCalls)
	})

	t.Run("fails after three conflicts", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return([]*pack.Package{stale()}, nil)
		h.packRepo.EXPECT().Update(gomock.Any(), detachPackA, detachOrgID, detachLedgerID, detachReadAt, gomock.Any()).
			Return(nil, packConflict()).Times(3)
		h.packRepo.EXPECT().FindByID(gomock.Any(), detachPackA, detachOrgID, detachLedgerID).
			Return(stale(), nil).Times(2)

		_, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.ErrorIs(t, err, errDetachConflictExhausted)
		assert.False(t, pkg.IsBusinessError(err))
		assert.Zero(t, h.cache.delCalls)
		assert.Empty(t, h.emitter.Events())
	})

	t.Run("skips a package deleted meanwhile", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return([]*pack.Package{stale()}, nil)
		h.packRepo.EXPECT().Update(gomock.Any(), detachPackA, detachOrgID, detachLedgerID, detachReadAt, gomock.Any()).
			Return(nil, packConflict())
		h.packRepo.EXPECT().FindByID(gomock.Any(), detachPackA, detachOrgID, detachLedgerID).
			Return(nil, mongo.ErrNoDocuments)

		got, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.NoError(t, err)
		assert.Zero(t, got)
		assert.Zero(t, h.cache.delCalls)
	})

	t.Run("skips a package that no longer references the alias", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return([]*pack.Package{stale()}, nil)
		h.packRepo.EXPECT().Update(gomock.Any(), detachPackA, detachOrgID, detachLedgerID, detachReadAt, gomock.Any()).
			Return(nil, packConflict())
		h.packRepo.EXPECT().FindByID(gomock.Any(), detachPackA, detachOrgID, detachLedgerID).
			Return(detachPack(detachPackA, true, map[string]model.Fee{"cust": detachFee(detachOtherAlias)}, nil), nil)

		got, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.NoError(t, err)
		assert.Zero(t, got)
	})
}

func TestDetachAccountAlias_FeePackErrorsPropagate(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("mongo unavailable")
	matching := func(id uuid.UUID) *pack.Package {
		return detachPack(id, true, map[string]model.Fee{"adm": detachFee(detachAlias)}, nil)
	}

	t.Run("list", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return(nil, errBoom)

		_, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.ErrorIs(t, err, errBoom)
	})

	t.Run("update", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return([]*pack.Package{matching(detachPackA)}, nil)
		h.packRepo.EXPECT().Update(gomock.Any(), detachPackA, detachOrgID, detachLedgerID, detachReadAt, gomock.Any()).
			Return(nil, errBoom)

		_, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.ErrorIs(t, err, errBoom)
		assert.Zero(t, h.cache.delCalls)
	})

	t.Run("re-read", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return([]*pack.Package{matching(detachPackA)}, nil)
		h.packRepo.EXPECT().Update(gomock.Any(), detachPackA, detachOrgID, detachLedgerID, detachReadAt, gomock.Any()).
			Return(nil, packConflict())
		h.packRepo.EXPECT().FindByID(gomock.Any(), detachPackA, detachOrgID, detachLedgerID).Return(nil, errBoom)

		_, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.ErrorIs(t, err, errBoom)
	})

	t.Run("a later failure still invalidates the cache for packages already written", func(t *testing.T) {
		t.Parallel()

		h := newDetachHarness(t)

		h.packRepo.EXPECT().FindNotDeletedByOrganizationIDAndLedgerID(gomock.Any(), detachOrgID, detachLedgerID).
			Return([]*pack.Package{matching(detachPackA), matching(detachPackB)}, nil)

		var captured bson.M

		h.expectPackUpdate(detachPackA, detachReadAt, &captured)
		h.packRepo.EXPECT().Update(gomock.Any(), detachPackB, detachOrgID, detachLedgerID, detachReadAt, gomock.Any()).
			Return(nil, errBoom)

		_, err := h.uc.DetachAccountAlias(context.Background(), detachOrgID, detachLedgerID, detachAlias)
		require.ErrorIs(t, err, errBoom)
		assert.Equal(t, 1, h.cache.delCalls)
		assert.Equal(t, 1, countEvents(h.emitter, "fee_packages.updated"))
	})
}
