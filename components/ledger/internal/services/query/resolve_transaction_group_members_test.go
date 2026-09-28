// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	postgres "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// indexedEvidenceRepositoryFake holds one indexed execution per transaction.
// Raw values are the transaction ID so the codec stub can tell them apart.
type indexedEvidenceRepositoryFake struct {
	indexed     map[uuid.UUID]uuid.UUID
	indexErr    error
	evidenceErr error
}

func (fake *indexedEvidenceRepositoryFake) GetEngineTransactionIndex(_ context.Context, _, _, transactionID uuid.UUID) ([]byte, error) {
	if fake.indexErr != nil {
		return nil, fake.indexErr
	}

	if _, ok := fake.indexed[transactionID]; !ok {
		return nil, redis.ErrEngineWriteBehindNotFound
	}

	return []byte(transactionID.String()), nil
}

func (fake *indexedEvidenceRepositoryFake) GetEngineTransactionEvidence(_ context.Context, _, _, transactionID, executionID uuid.UUID) ([]byte, []byte, error) {
	if fake.evidenceErr != nil {
		return nil, nil, fake.evidenceErr
	}

	if fake.indexed[transactionID] != executionID {
		return nil, nil, redis.ErrEngineWriteBehindNotFound
	}

	return []byte(transactionID.String()), []byte(executionID.String()), nil
}

func (fake *indexedEvidenceRepositoryFake) GetEngineMaterializedTransaction(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*redis.EngineMaterializedTransaction, error) {
	return nil, redis.ErrEngineWriteBehindNotFound
}

func (fake *indexedEvidenceRepositoryFake) MaterializeEngineTransaction(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, []byte, time.Duration) (bool, error) {
	return true, nil
}

// manifestCodecStub answers the index with the fake's execution and the
// manifest configured for the addressed transaction.
type manifestCodecStub struct {
	repository  *indexedEvidenceRepositoryFake
	manifests   map[uuid.UUID][]EngineExecutionMember
	manifestErr error
}

func (codec manifestCodecStub) DecodeEngineTransactionIndex(_ context.Context, raw []byte, _, _, transactionID uuid.UUID) (uuid.UUID, bool, error) {
	if string(raw) != transactionID.String() {
		return uuid.Nil, false, errors.New("index identity mismatch")
	}

	return codec.repository.indexed[transactionID], true, nil
}

func (codec manifestCodecStub) BuildEngineTransactionLookup(_ context.Context, _, _, _ []byte, organizationID, ledgerID, transactionID uuid.UUID) (*postgres.Transaction, error) {
	return &postgres.Transaction{
		ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		Status: postgres.Status{Code: constant.PENDING},
	}, nil
}

func (codec manifestCodecStub) DecodeEngineTransactionExecutionMembers(_ context.Context, _, _, _ []byte, _, _, transactionID uuid.UUID) ([]EngineExecutionMember, bool, error) {
	if codec.manifestErr != nil {
		return nil, false, codec.manifestErr
	}

	members, ok := codec.manifests[transactionID]

	return members, ok, nil
}

type groupMembersFixture struct {
	organizationID      uuid.UUID
	ledgerA, ledgerB    uuid.UUID
	groupID             uuid.UUID
	origin, destination uuid.UUID
	repository          *indexedEvidenceRepositoryFake
	codec               manifestCodecStub
	transactionRepo     *postgres.MockRepository
	uc                  *UseCase
}

func newGroupMembersFixture(t *testing.T) *groupMembersFixture {
	t.Helper()

	fixture := &groupMembersFixture{
		organizationID: uuid.New(), ledgerA: uuid.New(), ledgerB: uuid.New(),
		groupID: uuid.New(), origin: uuid.New(), destination: uuid.New(),
	}
	execution := uuid.New()
	fixture.repository = &indexedEvidenceRepositoryFake{indexed: map[uuid.UUID]uuid.UUID{
		fixture.origin: execution, fixture.destination: execution,
	}}
	fixture.codec = manifestCodecStub{
		repository: fixture.repository,
		manifests: map[uuid.UUID][]EngineExecutionMember{fixture.origin: {
			{TransactionID: fixture.origin, OrganizationID: fixture.organizationID, LedgerID: fixture.ledgerA},
			{TransactionID: fixture.destination, OrganizationID: fixture.organizationID, LedgerID: fixture.ledgerB},
		}},
	}
	fixture.transactionRepo = postgres.NewMockRepository(gomock.NewController(t))
	fixture.uc = &UseCase{
		EngineWriteBehindRepo:  fixture.repository,
		EngineWriteBehindCodec: fixture.codec,
		TransactionRepo:        fixture.transactionRepo,
	}

	return fixture
}

func (fixture *groupMembersFixture) resolve() ([]*postgres.Transaction, error) {
	return fixture.uc.ResolveTransactionGroupMembers(
		context.Background(), fixture.organizationID, fixture.ledgerA, fixture.origin, fixture.groupID,
	)
}

func memberIDs(members []*postgres.Transaction) []string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.ID)
	}

	return ids
}

var onPrimary = gomock.Cond(func(ctx context.Context) bool { return readrouting.IsPrimaryRead(ctx) })

func TestResolveTransactionGroupMembers_ResolvesTheManifestBeforeProjection(t *testing.T) {
	fixture := newGroupMembersFixture(t)

	members, err := fixture.resolve()
	require.NoError(t, err)
	assert.Equal(t, []string{fixture.origin.String(), fixture.destination.String()}, memberIDs(members),
		"members come from the addressed execution, in execution order, without a group listing")
	assert.Equal(t, fixture.ledgerB.String(), members[1].LedgerID)
}

func TestResolveTransactionGroupMembers_ResolvesAPersistedMemberFromThePrimary(t *testing.T) {
	fixture := newGroupMembersFixture(t)
	delete(fixture.repository.indexed, fixture.destination)
	persisted := &postgres.Transaction{
		ID: fixture.destination.String(), OrganizationID: fixture.organizationID.String(), LedgerID: fixture.ledgerB.String(),
		Status: postgres.Status{Code: constant.APPROVED},
	}
	fixture.transactionRepo.EXPECT().
		FindWithOperations(onPrimary, fixture.organizationID, fixture.ledgerB, fixture.destination).
		Return(persisted, nil)

	members, err := fixture.resolve()
	require.NoError(t, err)
	require.Len(t, members, 2)
	assert.Same(t, persisted, members[1])
}

func TestResolveTransactionGroupMembers_ReadsTheGroupFromThePrimaryWithoutAManifest(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*groupMembersFixture)
	}{
		{name: "the addressed transaction has no index", setup: func(fixture *groupMembersFixture) {
			delete(fixture.repository.indexed, fixture.origin)
		}},
		{name: "its execution recorded no manifest", setup: func(fixture *groupMembersFixture) {
			delete(fixture.codec.manifests, fixture.origin)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGroupMembersFixture(t)
			test.setup(fixture)
			rows := []*postgres.Transaction{{ID: fixture.origin.String()}, {ID: fixture.destination.String()}}
			fixture.transactionRepo.EXPECT().FindByGroupID(onPrimary, fixture.groupID).Return(rows, nil)

			members, err := fixture.resolve()
			require.NoError(t, err)
			assert.Equal(t, rows, members)
		})
	}
}

func TestResolveTransactionGroupMembers_MissingManifestMemberIsIncomplete(t *testing.T) {
	fixture := newGroupMembersFixture(t)
	delete(fixture.repository.indexed, fixture.destination)
	fixture.transactionRepo.EXPECT().
		FindWithOperations(onPrimary, fixture.organizationID, fixture.ledgerB, fixture.destination).
		Return(&postgres.Transaction{}, nil)
	fixture.transactionRepo.EXPECT().
		Find(onPrimary, fixture.organizationID, fixture.ledgerB, fixture.destination).
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransaction))

	members, err := fixture.resolve()
	require.Nil(t, members)

	var business pkg.UnprocessableOperationError
	require.Truef(t, errors.As(err, &business), "expected a business error, got %v", err)
	assert.Equal(t, constant.ErrCrossLedgerGroupIncomplete.Error(), business.Code)
}

func TestResolveTransactionGroupMembers_FailsClosedOnUnreadableEvidence(t *testing.T) {
	transportErr := errors.New("valkey unavailable")

	for _, test := range []struct {
		name  string
		setup func(*groupMembersFixture)
		want  error
	}{
		{name: "index transport error", setup: func(fixture *groupMembersFixture) {
			fixture.repository.indexErr = transportErr
		}, want: transportErr},
		{name: "evidence missing behind the index", setup: func(fixture *groupMembersFixture) {
			fixture.repository.evidenceErr = redis.ErrEngineWriteBehindNotFound
		}, want: redis.ErrEngineWriteBehindNotFound},
		{name: "corrupt manifest", setup: func(fixture *groupMembersFixture) {
			fixture.codec.manifestErr = transportErr
			fixture.uc.EngineWriteBehindCodec = fixture.codec
		}, want: transportErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGroupMembersFixture(t)
			test.setup(fixture)

			members, err := fixture.resolve()
			require.Nil(t, members)
			require.ErrorIs(t, err, test.want, "no primary read may stand in for evidence that exists but cannot be read")
		})
	}
}

var _ redis.EngineWriteBehindRepository = (*indexedEvidenceRepositoryFake)(nil)
