// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"slices"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const windowTestCollection = "Portfolio"

// sortedIDs returns n random ids in ascending string order, the order the metadata store answers.
func sortedIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = uuid.NewString()
	}

	slices.Sort(out)

	return out
}

// windowHarness runs listMetadataWindow over a fake entity store holding the live ids.
type windowHarness struct {
	metadataRepo *mongodb.MockRepository
	live         map[string]bool
	findAllCalls []http.QueryHeader
}

func newWindowHarness(t *testing.T, live []string) *windowHarness {
	t.Helper()

	h := &windowHarness{
		metadataRepo: mongodb.NewMockRepository(gomock.NewController(t)),
		live:         make(map[string]bool, len(live)),
	}

	for _, id := range live {
		h.live[id] = true
	}

	h.metadataRepo.EXPECT().FindByEntityIDs(gomock.Any(), windowTestCollection, gomock.Any()).
		Return([]*mongodb.Metadata{}, nil).AnyTimes()

	return h
}

// expectBatch expects one FindEntityIDs read after `after` answering ids.
func (h *windowHarness) expectBatch(after string, batchSize int, ids []string) *gomock.Call {
	return h.metadataRepo.EXPECT().
		FindEntityIDs(gomock.Any(), windowTestCollection, gomock.Any(), after, batchSize).
		Return(ids, nil)
}

func (h *windowHarness) list(ctx context.Context, filter http.QueryHeader, batchSize int) ([]string, error) {
	_, span := noop.NewTracerProvider().Tracer("test").Start(ctx, "test")
	defer span.End()

	uc := &UseCase{OnboardingMetadataRepo: h.metadataRepo}

	rows, err := listMetadataWindow(ctx, span, libLog.NewNop(), uc, filter, metadataListWindow[*mmodel.Portfolio]{
		collection: windowTestCollection,
		batchSize:  batchSize,
		findAll: func(_ context.Context, qh http.QueryHeader) ([]*mmodel.Portfolio, error) {
			h.findAllCalls = append(h.findAllCalls, qh)

			// Answer in reverse so the window has to restore the batch order.
			out := []*mmodel.Portfolio{}

			for i := len(qh.EntityIDs) - 1; i >= 0; i-- {
				if h.live[qh.EntityIDs[i].String()] {
					out = append(out, &mmodel.Portfolio{ID: qh.EntityIDs[i].String()})
				}
			}

			return out, nil
		},
		entityID: func(p *mmodel.Portfolio) string { return p.ID },
		attach:   func(p *mmodel.Portfolio, data map[string]any) { p.Metadata = data },
	})
	if rows == nil {
		return nil, err
	}

	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}

	return ids, err
}

func pageFilter(limit, page int) http.QueryHeader {
	return http.QueryHeader{UseMetadata: true, Limit: limit, Page: page, SortOrder: "asc"}
}

func TestListMetadataWindow_FirstPageReadsOneBatch(t *testing.T) {
	ids := sortedIDs(7)
	h := newWindowHarness(t, ids)

	h.expectBatch("", 3, ids[:3])

	got, err := h.list(context.Background(), pageFilter(2, 1), 3)

	require.NoError(t, err)
	assert.Equal(t, ids[:2], got)
	require.Len(t, h.findAllCalls, 1)
	assert.Equal(t, 1, h.findAllCalls[0].Page)
	assert.Equal(t, 3, h.findAllCalls[0].Limit, "the entity store reads the whole batch on one page")
}

func TestListMetadataWindow_PageCrossesBatchBoundary(t *testing.T) {
	ids := sortedIDs(7)
	h := newWindowHarness(t, ids)

	gomock.InOrder(
		h.expectBatch("", 3, ids[:3]),
		h.expectBatch(ids[2], 3, ids[3:6]),
	)

	got, err := h.list(context.Background(), pageFilter(2, 2), 3)

	require.NoError(t, err)
	assert.Equal(t, ids[2:4], got)
}

func TestListMetadataWindow_LastPageEndsOnShortBatch(t *testing.T) {
	ids := sortedIDs(7)
	h := newWindowHarness(t, ids)

	gomock.InOrder(
		h.expectBatch("", 3, ids[:3]),
		h.expectBatch(ids[2], 3, ids[3:6]),
		h.expectBatch(ids[5], 3, ids[6:]),
	)

	got, err := h.list(context.Background(), pageFilter(2, 4), 3)

	require.NoError(t, err)
	assert.Equal(t, ids[6:], got)
}

func TestListMetadataWindow_FullBatchThenEmptyBatchEnds(t *testing.T) {
	ids := sortedIDs(6)
	h := newWindowHarness(t, ids)

	gomock.InOrder(
		h.expectBatch("", 3, ids[:3]),
		h.expectBatch(ids[2], 3, ids[3:]),
		h.expectBatch(ids[5], 3, []string{}),
	)

	got, err := h.list(context.Background(), pageFilter(10, 1), 3)

	require.NoError(t, err)
	assert.Equal(t, ids, got)
}

func TestListMetadataWindow_GhostIDsTakeNoSlot(t *testing.T) {
	ids := sortedIDs(6)
	// ids[0] and ids[3] have metadata but no live entity row.
	live := []string{ids[1], ids[2], ids[4], ids[5]}

	t.Run("page 1", func(t *testing.T) {
		h := newWindowHarness(t, live)

		gomock.InOrder(
			h.expectBatch("", 3, ids[:3]),
		)

		got, err := h.list(context.Background(), pageFilter(2, 1), 3)

		require.NoError(t, err)
		assert.Equal(t, []string{ids[1], ids[2]}, got)
	})

	t.Run("page 2 offsets over live rows only", func(t *testing.T) {
		h := newWindowHarness(t, live)

		gomock.InOrder(
			h.expectBatch("", 3, ids[:3]),
			h.expectBatch(ids[2], 3, ids[3:]),
		)

		got, err := h.list(context.Background(), pageFilter(2, 2), 3)

		require.NoError(t, err)
		assert.Equal(t, []string{ids[4], ids[5]}, got)
	})
}

func TestListMetadataWindow_NonUUIDIDIsSkipped(t *testing.T) {
	ids := sortedIDs(2)
	h := newWindowHarness(t, ids)

	h.expectBatch("", 3, []string{ids[0], "not-a-uuid"})

	got, err := h.list(context.Background(), pageFilter(2, 1), 3)

	require.NoError(t, err)
	assert.Equal(t, ids[:1], got)
	require.Len(t, h.findAllCalls, 1)
	assert.Len(t, h.findAllCalls[0].EntityIDs, 1)
}

func TestListMetadataWindow_BatchOfOnlyNonUUIDsAdvancesTheCursor(t *testing.T) {
	ids := sortedIDs(1)
	h := newWindowHarness(t, ids)

	gomock.InOrder(
		h.expectBatch("", 2, []string{"bad-a", "bad-b"}),
		h.expectBatch("bad-b", 2, ids),
	)

	got, err := h.list(context.Background(), pageFilter(2, 1), 2)

	require.NoError(t, err)
	assert.Equal(t, ids, got)
	assert.Len(t, h.findAllCalls, 1, "a batch with no UUID never reaches the entity store")
}

func TestListMetadataWindow_CancelledContextStopsBetweenBatches(t *testing.T) {
	ids := sortedIDs(6)
	h := newWindowHarness(t, ids[3:])

	ctx, cancel := context.WithCancel(context.Background())

	h.metadataRepo.EXPECT().
		FindEntityIDs(gomock.Any(), windowTestCollection, gomock.Any(), "", 3).
		DoAndReturn(func(context.Context, string, http.QueryHeader, string, int) ([]string, error) {
			cancel()

			return ids[:3], nil
		})

	got, err := h.list(ctx, pageFilter(2, 1), 3)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, got)
}

func TestListMetadataWindow_NonPositiveLimitIsEmpty(t *testing.T) {
	h := newWindowHarness(t, nil)

	got, err := h.list(context.Background(), pageFilter(0, 1), 3)

	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestListMetadataWindow_DefaultBatchSize(t *testing.T) {
	ids := sortedIDs(1)
	h := newWindowHarness(t, ids)

	h.expectBatch("", metadataListBatchSize, ids)

	got, err := h.list(context.Background(), pageFilter(2, 1), 0)

	require.NoError(t, err)
	assert.Equal(t, ids, got)
}

func TestOrderByEntityIDs(t *testing.T) {
	t.Parallel()

	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	unknownA, unknownB := uuid.NewString(), uuid.NewString()

	rows := []string{unknownA, ids[2].String(), ids[0].String(), unknownB, ids[1].String()}

	orderByEntityIDs(rows, ids, func(s string) string { return s })

	assert.Equal(t, []string{ids[0].String(), ids[1].String(), ids[2].String(), unknownA, unknownB}, rows,
		"rows follow the id order; rows outside it keep their relative order after them")
}
