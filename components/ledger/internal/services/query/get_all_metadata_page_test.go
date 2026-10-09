// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// expectFindAll expects one entity-store read answering rows, in exactly that order, for a filter
// satisfying filterMatcher.
type expectFindAll func(rows []uuid.UUID, filterMatcher gomock.Matcher) *gomock.Call

// pageMetadataCase drives one metadata-filtered listing through the shared window contract.
type pageMetadataCase struct {
	name       string
	collection string
	// offset is true for the page/limit listings; the account type listing pages by cursor.
	offset bool
	// build wires the entity repository and returns the use case plus a function expecting one
	// FindAll per call.
	build func(ctrl *gomock.Controller) (*UseCase, expectFindAll)
	// list runs the use case with filter and returns the ids in result order plus the metadata it
	// attached, keyed by entity ID.
	list func(uc *UseCase, filter http.QueryHeader) ([]string, map[string]map[string]any, error)
}

func listedPage[T any](items []T, entry func(T) (string, map[string]any)) ([]string, map[string]map[string]any) {
	if items == nil {
		return nil, nil
	}

	ids := make([]string, 0, len(items))
	out := make(map[string]map[string]any, len(items))

	for _, item := range items {
		id, data := entry(item)
		ids = append(ids, id)
		out[id] = data
	}

	return ids, out
}

func pageMetadataCases() []pageMetadataCase {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	return []pageMetadataCase{
		{
			name:       "organizations",
			collection: constant.EntityOrganization,
			offset:     true,
			build: func(ctrl *gomock.Controller) (*UseCase, expectFindAll) {
				repo := organization.NewMockRepository(ctrl)

				return &UseCase{OrganizationRepo: repo}, func(rows []uuid.UUID, filterMatcher gomock.Matcher) *gomock.Call {
					out := make([]*mmodel.Organization, len(rows))

					for i, id := range rows {
						out[i] = &mmodel.Organization{ID: id.String()}
					}

					return repo.EXPECT().FindAll(gomock.Any(), filterMatcher).Return(out, nil)
				}
			},
			list: func(uc *UseCase, filter http.QueryHeader) ([]string, map[string]map[string]any, error) {
				rows, err := uc.GetAllMetadataOrganizations(ctx, filter)
				ids, meta := listedPage(rows, func(r *mmodel.Organization) (string, map[string]any) { return r.ID, r.Metadata })

				return ids, meta, err
			},
		},
		{
			name:       "ledgers",
			collection: constant.EntityLedger,
			offset:     true,
			build: func(ctrl *gomock.Controller) (*UseCase, expectFindAll) {
				repo := ledger.NewMockRepository(ctrl)

				return &UseCase{LedgerRepo: repo}, func(rows []uuid.UUID, filterMatcher gomock.Matcher) *gomock.Call {
					out := make([]*mmodel.Ledger, len(rows))

					for i, id := range rows {
						out[i] = &mmodel.Ledger{ID: id.String()}
					}

					return repo.EXPECT().FindAll(gomock.Any(), organizationID, filterMatcher).Return(out, nil)
				}
			},
			list: func(uc *UseCase, filter http.QueryHeader) ([]string, map[string]map[string]any, error) {
				rows, err := uc.GetAllMetadataLedgers(ctx, organizationID, filter)
				ids, meta := listedPage(rows, func(r *mmodel.Ledger) (string, map[string]any) { return r.ID, r.Metadata })

				return ids, meta, err
			},
		},
		{
			name:       "portfolios",
			collection: constant.EntityPortfolio,
			offset:     true,
			build: func(ctrl *gomock.Controller) (*UseCase, expectFindAll) {
				repo := portfolio.NewMockRepository(ctrl)

				return &UseCase{PortfolioRepo: repo}, func(rows []uuid.UUID, filterMatcher gomock.Matcher) *gomock.Call {
					out := make([]*mmodel.Portfolio, len(rows))

					for i, id := range rows {
						out[i] = &mmodel.Portfolio{ID: id.String()}
					}

					return repo.EXPECT().FindAll(gomock.Any(), organizationID, ledgerID, filterMatcher).Return(out, nil)
				}
			},
			list: func(uc *UseCase, filter http.QueryHeader) ([]string, map[string]map[string]any, error) {
				rows, err := uc.GetAllMetadataPortfolios(ctx, organizationID, ledgerID, filter)
				ids, meta := listedPage(rows, func(r *mmodel.Portfolio) (string, map[string]any) { return r.ID, r.Metadata })

				return ids, meta, err
			},
		},
		{
			name:       "accounts",
			collection: constant.EntityAccount,
			offset:     true,
			build: func(ctrl *gomock.Controller) (*UseCase, expectFindAll) {
				repo := account.NewMockRepository(ctrl)

				return &UseCase{AccountRepo: repo}, func(rows []uuid.UUID, filterMatcher gomock.Matcher) *gomock.Call {
					out := make([]*mmodel.Account, len(rows))

					for i, id := range rows {
						out[i] = &mmodel.Account{ID: id.String()}
					}

					return repo.EXPECT().
						FindAll(gomock.Any(), organizationID, ledgerID, gomock.Nil(), gomock.Nil(), filterMatcher, mmodel.HolderOnV2).
						Return(out, nil)
				}
			},
			list: func(uc *UseCase, filter http.QueryHeader) ([]string, map[string]map[string]any, error) {
				rows, err := uc.GetAllMetadataAccounts(ctx, organizationID, ledgerID, nil, nil, filter, mmodel.HolderOnV2)
				ids, meta := listedPage(rows, func(r *mmodel.Account) (string, map[string]any) { return r.ID, r.Metadata })

				return ids, meta, err
			},
		},
		{
			name:       "assets",
			collection: constant.EntityAsset,
			offset:     true,
			build: func(ctrl *gomock.Controller) (*UseCase, expectFindAll) {
				repo := asset.NewMockRepository(ctrl)

				return &UseCase{AssetRepo: repo}, func(rows []uuid.UUID, filterMatcher gomock.Matcher) *gomock.Call {
					out := make([]*mmodel.Asset, len(rows))

					for i, id := range rows {
						out[i] = &mmodel.Asset{ID: id.String()}
					}

					return repo.EXPECT().FindAll(gomock.Any(), organizationID, ledgerID, filterMatcher).Return(out, nil)
				}
			},
			list: func(uc *UseCase, filter http.QueryHeader) ([]string, map[string]map[string]any, error) {
				rows, err := uc.GetAllMetadataAssets(ctx, organizationID, ledgerID, filter)
				ids, meta := listedPage(rows, func(r *mmodel.Asset) (string, map[string]any) { return r.ID, r.Metadata })

				return ids, meta, err
			},
		},
		{
			name:       "segments",
			collection: constant.EntitySegment,
			offset:     true,
			build: func(ctrl *gomock.Controller) (*UseCase, expectFindAll) {
				repo := segment.NewMockRepository(ctrl)

				return &UseCase{SegmentRepo: repo}, func(rows []uuid.UUID, filterMatcher gomock.Matcher) *gomock.Call {
					out := make([]*mmodel.Segment, len(rows))

					for i, id := range rows {
						out[i] = &mmodel.Segment{ID: id.String()}
					}

					return repo.EXPECT().FindAll(gomock.Any(), organizationID, ledgerID, filterMatcher).Return(out, nil)
				}
			},
			list: func(uc *UseCase, filter http.QueryHeader) ([]string, map[string]map[string]any, error) {
				rows, err := uc.GetAllMetadataSegments(ctx, organizationID, ledgerID, filter)
				ids, meta := listedPage(rows, func(r *mmodel.Segment) (string, map[string]any) { return r.ID, r.Metadata })

				return ids, meta, err
			},
		},
		{
			name:       "account types",
			collection: constant.EntityAccountType,
			build: func(ctrl *gomock.Controller) (*UseCase, expectFindAll) {
				repo := accounttype.NewMockRepository(ctrl)

				return &UseCase{AccountTypeRepo: repo}, func(rows []uuid.UUID, filterMatcher gomock.Matcher) *gomock.Call {
					out := make([]*mmodel.AccountType, len(rows))

					for i, id := range rows {
						out[i] = &mmodel.AccountType{ID: id}
					}

					return repo.EXPECT().
						FindAll(gomock.Any(), organizationID, ledgerID, filterMatcher).
						Return(out, libHTTP.CursorPagination{}, nil)
				}
			},
			list: func(uc *UseCase, filter http.QueryHeader) ([]string, map[string]map[string]any, error) {
				rows, _, err := uc.GetAllMetadataAccountType(ctx, organizationID, ledgerID, filter)
				ids, meta := listedPage(rows, func(r *mmodel.AccountType) (string, map[string]any) { return r.ID.String(), r.Metadata })

				return ids, meta, err
			},
		},
	}
}

func offsetPageMetadataCases() []pageMetadataCase {
	return slices.DeleteFunc(pageMetadataCases(), func(tc pageMetadataCase) bool { return !tc.offset })
}

func idStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}

	return out
}

func entityIDOnly(ids []uuid.UUID) []*mongodb.Metadata {
	out := make([]*mongodb.Metadata, len(ids))
	for i, id := range ids {
		out[i] = &mongodb.Metadata{EntityID: id.String()}
	}

	return out
}

func reversed(ids []uuid.UUID) []uuid.UUID {
	out := slices.Clone(ids)
	slices.Reverse(out)

	return out
}

// sortedUUIDs returns n random ids in ascending string order, the order the metadata store answers.
func sortedUUIDs(n int) []uuid.UUID {
	out := make([]uuid.UUID, n)
	for i := range out {
		out[i] = uuid.New()
	}

	slices.SortFunc(out, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })

	return out
}

// filterWith matches a QueryHeader whose EntityIDs are exactly ids and that satisfies check.
func filterWith(ids []uuid.UUID, check func(http.QueryHeader) bool) gomock.Matcher {
	return gomock.Cond(func(qh http.QueryHeader) bool {
		return slices.Equal(qh.EntityIDs, ids) && check(qh)
	})
}

// batchPage matches the entity read of one batch: exactly ids, on one page sized to them.
func batchPage(ids []uuid.UUID) gomock.Matcher {
	return filterWith(ids, func(qh http.QueryHeader) bool { return qh.Page == 1 && qh.Limit == len(ids) })
}

// expectMatch expects the metadata store read that answers the listing's match: one batch of entity
// ids for the offset listings, the full match set for the account type listing.
func expectMatch(repo *mongodb.MockRepository, tc pageMetadataCase, ids []uuid.UUID) *gomock.Call {
	if tc.offset {
		return repo.EXPECT().
			FindEntityIDs(gomock.Any(), tc.collection, gomock.Any(), "", metadataListBatchSize).
			Return(idStrings(ids), nil)
	}

	return repo.EXPECT().
		FindList(gomock.Any(), tc.collection, gomock.Any()).
		Return(entityIDOnly(ids), nil)
}

// TestGetAllMetadata_LoadsMetadataOnlyForThePage locks the page-metadata contract shared by the
// metadata-filtered onboarding listings: metadata is read only for the entities of the returned page.
func TestGetAllMetadata_LoadsMetadataOnlyForThePage(t *testing.T) {
	for _, tc := range pageMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			page := sortedUUIDs(2)

			uc, expect := tc.build(ctrl)
			findAll := expect(page, gomock.Any())
			uc.OnboardingMetadataRepo = metadataRepo

			gomock.InOrder(
				expectMatch(metadataRepo, tc, page),
				findAll,
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), tc.collection, idStrings(page)).
					Return([]*mongodb.Metadata{
						{EntityID: page[0].String(), Data: map[string]any{"tier": "gold"}},
						{EntityID: page[1].String(), Data: map[string]any{"tier": "silver"}},
					}, nil),
			)

			_, got, err := tc.list(uc, http.QueryHeader{UseMetadata: true, Limit: 2, Page: 1})

			require.NoError(t, err)
			assert.Equal(t, map[string]map[string]any{
				page[0].String(): {"tier": "gold"},
				page[1].String(): {"tier": "silver"},
			}, got)
		})
	}
}

func TestGetAllMetadata_EmptyPageSkipsMetadataLookup(t *testing.T) {
	for _, tc := range pageMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			uc, expect := tc.build(ctrl)
			findAll := expect([]uuid.UUID{}, gomock.Any())
			uc.OnboardingMetadataRepo = metadataRepo

			gomock.InOrder(
				expectMatch(metadataRepo, tc, []uuid.UUID{uuid.New()}),
				findAll,
			)
			metadataRepo.EXPECT().FindByEntityIDs(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			ids, got, err := tc.list(uc, http.QueryHeader{UseMetadata: true, Limit: 2, Page: 1})

			require.NoError(t, err)
			assert.NotNil(t, ids, "an empty page is a non-nil slice")
			assert.Empty(t, got)
		})
	}
}

func TestGetAllMetadata_NoMatchSkipsEntityStore(t *testing.T) {
	for _, tc := range pageMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			uc, _ := tc.build(ctrl)
			uc.OnboardingMetadataRepo = metadataRepo

			expectMatch(metadataRepo, tc, []uuid.UUID{})
			metadataRepo.EXPECT().FindByEntityIDs(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			ids, got, err := tc.list(uc, http.QueryHeader{UseMetadata: true, Limit: 2, Page: 1})

			require.NoError(t, err)
			assert.NotNil(t, ids, "no match is an empty, non-nil page")
			assert.Empty(t, got)
		})
	}
}

func TestGetAllMetadata_PageMetadataFailureIsReturned(t *testing.T) {
	for _, tc := range pageMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			page := []uuid.UUID{uuid.New()}
			storeErr := errors.New("mongo unavailable")

			uc, expect := tc.build(ctrl)
			findAll := expect(page, gomock.Any())
			uc.OnboardingMetadataRepo = metadataRepo

			gomock.InOrder(
				expectMatch(metadataRepo, tc, page),
				findAll,
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), tc.collection, idStrings(page)).
					Return(nil, storeErr),
			)

			ids, got, err := tc.list(uc, http.QueryHeader{UseMetadata: true, Limit: 2, Page: 1})

			require.ErrorIs(t, err, storeErr)
			assert.Nil(t, ids)
			assert.Nil(t, got)
		})
	}
}

func TestGetAllMetadata_MatchFailureIsReturned(t *testing.T) {
	for _, tc := range offsetPageMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataRepo := mongodb.NewMockRepository(ctrl)
			storeErr := errors.New("mongo unavailable")

			uc, _ := tc.build(ctrl)
			uc.OnboardingMetadataRepo = metadataRepo

			metadataRepo.EXPECT().
				FindEntityIDs(gomock.Any(), tc.collection, gomock.Any(), "", metadataListBatchSize).
				Return(nil, storeErr)

			ids, _, err := tc.list(uc, http.QueryHeader{UseMetadata: true, Limit: 2, Page: 1})

			require.ErrorIs(t, err, storeErr)
			assert.Nil(t, ids)
		})
	}
}

func TestGetAllMetadata_EntityStoreFailureIsReturned(t *testing.T) {
	for _, tc := range offsetPageMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataRepo := mongodb.NewMockRepository(ctrl)
			storeErr := errors.New("postgres unavailable")
			page := []uuid.UUID{uuid.New()}

			uc, expect := tc.build(ctrl)
			expect(nil, gomock.Any()).Return(nil, storeErr)
			uc.OnboardingMetadataRepo = metadataRepo

			expectMatch(metadataRepo, tc, page)
			metadataRepo.EXPECT().FindByEntityIDs(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			ids, _, err := tc.list(uc, http.QueryHeader{UseMetadata: true, Limit: 2, Page: 1})

			require.ErrorIs(t, err, storeErr)
			assert.Nil(t, ids)
		})
	}
}

// TestGetAllMetadata_ForwardsEveryRouteFilter locks the batch read: the metadata store receives the
// request's filter unchanged, and the entity store receives every other filter of the route (status,
// created_at range, sort order) with the batch's ids on one page sized to them. The page follows
// the metadata store's order whatever order the entity store answered in.
func TestGetAllMetadata_ForwardsEveryRouteFilter(t *testing.T) {
	for _, tc := range offsetPageMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			status := "ACTIVE"
			ids := reversed(sortedUUIDs(2))
			request := http.QueryHeader{
				UseMetadata: true,
				Metadata:    &bson.M{"metadata.tier": "gold"},
				Limit:       2,
				Page:        1,
				SortOrder:   "desc",
				Status:      &status,
				StartDate:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndDate:     time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
			}

			uc, expect := tc.build(ctrl)
			findAll := expect(reversed(ids), filterWith(ids, func(qh http.QueryHeader) bool {
				return qh.Page == 1 && qh.Limit == len(ids) && qh.SortOrder == "desc" &&
					qh.Status != nil && *qh.Status == status && qh.StartDate.Equal(request.StartDate) &&
					qh.EndDate.Equal(request.EndDate)
			}))
			uc.OnboardingMetadataRepo = metadataRepo

			gomock.InOrder(
				metadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), tc.collection, request, "", metadataListBatchSize).
					Return(idStrings(ids), nil),
				findAll,
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), tc.collection, idStrings(ids)).
					Return(entityIDOnly(ids), nil),
			)

			got, _, err := tc.list(uc, request)

			require.NoError(t, err)
			assert.Equal(t, idStrings(ids), got, "rows follow the metadata store's id order")
		})
	}
}

// TestGetAllMetadata_GhostIDTakesNoSlot locks the offset over live rows: an id the metadata store
// answers but the entity store does not (a soft-deleted entity, or one the route's other filters
// exclude) neither appears on the page nor counts toward the offset.
func TestGetAllMetadata_GhostIDTakesNoSlot(t *testing.T) {
	for _, tc := range offsetPageMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			ids := sortedUUIDs(4)
			live := []uuid.UUID{ids[1], ids[2], ids[3]}

			uc, expect := tc.build(ctrl)
			uc.OnboardingMetadataRepo = metadataRepo

			gomock.InOrder(
				expectMatch(metadataRepo, tc, ids),
				expect(live, batchPage(ids)),
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), tc.collection, idStrings(live[1:2])).
					Return(entityIDOnly(live[1:2]), nil),
			)

			got, _, err := tc.list(uc, http.QueryHeader{UseMetadata: true, Limit: 1, Page: 2})

			require.NoError(t, err)
			assert.Equal(t, idStrings(live[1:2]), got, "the offset counts live rows only")
		})
	}
}

// TestGetAllMetadata_PageCrossesBatchBoundary locks the batched read at its real size: a page whose
// offset runs past the first batch, which holds a ghost id, is completed from the next batch, read
// strictly after the last id of the first.
func TestGetAllMetadata_PageCrossesBatchBoundary(t *testing.T) {
	for _, tc := range offsetPageMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			first := sortedUUIDs(metadataListBatchSize)
			second := []uuid.UUID{uuid.New()}
			firstLive := slices.Delete(slices.Clone(first), 10, 11)
			want := []uuid.UUID{first[metadataListBatchSize-1], second[0]}

			uc, expect := tc.build(ctrl)
			uc.OnboardingMetadataRepo = metadataRepo

			gomock.InOrder(
				metadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), tc.collection, gomock.Any(), "", metadataListBatchSize).
					Return(idStrings(first), nil),
				expect(firstLive, batchPage(first)),
				metadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), tc.collection, gomock.Any(), first[metadataListBatchSize-1].String(), metadataListBatchSize).
					Return(idStrings(second), nil),
				expect(second, batchPage(second)),
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), tc.collection, idStrings(want)).
					Return(entityIDOnly(want), nil),
			)

			// 499 live rows in the first batch: page 250 of size 2 skips 498 and takes the last live
			// row of the first batch plus the first of the second.
			got, _, err := tc.list(uc, http.QueryHeader{UseMetadata: true, Limit: 2, Page: 250})

			require.NoError(t, err)
			assert.Equal(t, idStrings(want), got)
		})
	}
}

// TestGetAllMetadata_NonUUIDIDIsSkipped locks the tolerance to a malformed metadata document: an
// entity id that is not a UUID is skipped and the page is still served.
func TestGetAllMetadata_NonUUIDIDIsSkipped(t *testing.T) {
	for _, tc := range offsetPageMetadataCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataRepo := mongodb.NewMockRepository(ctrl)

			valid := []uuid.UUID{uuid.New()}

			uc, expect := tc.build(ctrl)
			findAll := expect(valid, batchPage(valid))
			uc.OnboardingMetadataRepo = metadataRepo

			gomock.InOrder(
				metadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), tc.collection, gomock.Any(), "", metadataListBatchSize).
					Return([]string{"not-a-uuid", valid[0].String()}, nil),
				findAll,
				metadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), tc.collection, idStrings(valid)).
					Return(entityIDOnly(valid), nil),
			)

			got, _, err := tc.list(uc, http.QueryHeader{UseMetadata: true, Limit: 2, Page: 1})

			require.NoError(t, err)
			assert.Equal(t, idStrings(valid), got)
		})
	}
}

// TestGetAllMetadata_AccountTypeReadsFullMatchSet locks the cursor listing: FindList answers the
// full match set and the cursor page is cut by PostgreSQL with the request's limit and cursor.
func TestGetAllMetadata_AccountTypeReadsFullMatchSet(t *testing.T) {
	tc := pageMetadataCases()[len(pageMetadataCases())-1]
	require.Equal(t, constant.EntityAccountType, tc.collection)

	ctrl := gomock.NewController(t)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	matched := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	page := matched[:2]

	uc, expect := tc.build(ctrl)
	findAll := expect(page, filterWith(matched, func(qh http.QueryHeader) bool {
		return qh.Limit == 2 && qh.Cursor == "next"
	}))
	uc.OnboardingMetadataRepo = metadataRepo

	gomock.InOrder(
		metadataRepo.EXPECT().
			FindList(gomock.Any(), tc.collection, gomock.Cond(func(qh http.QueryHeader) bool {
				return qh.UseMetadata
			})).
			Return(entityIDOnly(matched), nil),
		findAll,
		metadataRepo.EXPECT().
			FindByEntityIDs(gomock.Any(), tc.collection, idStrings(page)).
			Return(entityIDOnly(page), nil),
	)

	ids, _, err := tc.list(uc, http.QueryHeader{UseMetadata: true, Limit: 2, Cursor: "next"})

	require.NoError(t, err)
	assert.Equal(t, idStrings(page), ids)
}
