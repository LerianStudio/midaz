// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountprotection

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sinkMarkerStore records the seed admissions one sink test took and released.
// Seed admissions are what a sink carries: the cache-miss load hands them over to
// the execution that admits its seeds.
type sinkMarkerStore struct {
	mu       sync.Mutex
	owned    map[uuid.UUID]string
	released []uuid.UUID
}

func newSinkMarkerStore() *sinkMarkerStore {
	return &sinkMarkerStore{owned: map[uuid.UUID]string{}}
}

func (s *sinkMarkerStore) GetAccountClosingMarker(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (string, bool, error) {
	return "", false, nil
}

func (s *sinkMarkerStore) GetAccountClosedMarker(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (s *sinkMarkerStore) SetAccountClosedMarker(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}

func (s *sinkMarkerStore) AcquireAccountAdminOwnership(_ context.Context, _, _, accountID uuid.UUID, token string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, owned := s.owned[accountID]; owned {
		return false, nil
	}

	s.owned[accountID] = token

	return true, nil
}

func (s *sinkMarkerStore) ReleaseAccountAdminOwnership(_ context.Context, _, _, accountID uuid.UUID, token string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.owned[accountID] != token {
		return false, nil
	}

	delete(s.owned, accountID)
	s.released = append(s.released, accountID)

	return true, nil
}

func (s *sinkMarkerStore) AdmitAccountSeed(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return s.AcquireAccountAdminOwnership(ctx, organizationID, ledgerID, accountID, token)
}

func (s *sinkMarkerStore) ReleaseAccountSeed(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return s.ReleaseAccountAdminOwnership(ctx, organizationID, ledgerID, accountID, token)
}

func (s *sinkMarkerStore) ownedAccounts() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.owned)
}

func sinkScope() (uuid.UUID, uuid.UUID, uuid.UUID) {
	return uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		uuid.MustParse("33333333-3333-4333-8333-333333333333")
}

func TestSinkAnswersTheTokenOfTheAccountItOwns(t *testing.T) {
	t.Parallel()

	organizationID, ledgerID, accountID := sinkScope()
	store := newSinkMarkerStore()

	ctx, sink := ContextWithSink(context.Background())

	admission, err := NewSeedAdmissionGuard(nil, store).AcquireSeedAdmission(ctx, organizationID, ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)
	require.True(t, AdoptAdmission(ctx, admission))

	assert.Equal(t, admission.Token(), sink.TokenFor(organizationID, ledgerID, accountID))
	assert.Empty(t, sink.TokenFor(organizationID, ledgerID, uuid.MustParse("44444444-4444-4444-8444-444444444444")),
		"an account outside the admission carries no token")
	assert.Empty(t, sink.TokenFor(uuid.MustParse("55555555-5555-4555-8555-555555555555"), ledgerID, accountID),
		"the token never answers for another organization")
	assert.Empty(t, sink.TokenFor(organizationID, uuid.MustParse("66666666-6666-4666-8666-666666666666"), accountID),
		"the token never answers for another ledger")
}

func TestSinkWithoutAContextOwnsNothing(t *testing.T) {
	t.Parallel()

	organizationID, ledgerID, accountID := sinkScope()

	assert.Nil(t, SinkFromContext(context.Background()))
	assert.Empty(t, SinkFromContext(context.Background()).TokenFor(organizationID, ledgerID, accountID))
	assert.False(t, AdoptAdmission(context.Background(), &Admission{accountIDs: []uuid.UUID{accountID}}))
}

func TestSinkReleasesEveryAdmissionItCollected(t *testing.T) {
	t.Parallel()

	organizationID, ledgerID, accountID := sinkScope()
	store := newSinkMarkerStore()

	ctx, sink := ContextWithSink(context.Background())

	admission, err := NewSeedAdmissionGuard(nil, store).AcquireSeedAdmission(ctx, organizationID, ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)
	require.True(t, AdoptAdmission(ctx, admission))
	require.Equal(t, 1, store.ownedAccounts())

	sink.Release(ctx)

	assert.Equal(t, 0, store.ownedAccounts())
	assert.Equal(t, []uuid.UUID{accountID}, store.released)
}

func TestSinkKeepsAnIndeterminateAdmissionForReconciliation(t *testing.T) {
	t.Parallel()

	organizationID, ledgerID, accountID := sinkScope()
	store := newSinkMarkerStore()

	ctx, sink := ContextWithSink(context.Background())

	admission, err := NewSeedAdmissionGuard(nil, store).AcquireSeedAdmission(ctx, organizationID, ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)
	require.True(t, AdoptAdmission(ctx, admission))

	sink.MarkIndeterminate()
	sink.Release(ctx)

	assert.Equal(t, 1, store.ownedAccounts(), "an unresolved outcome keeps its protection")
	assert.Empty(t, store.released)
}
