// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// blockExceptionFixture wires a UseCase over the two collaborators the command
// touches: the account repo (the read-only alias assertion) and the transaction
// Redis repo (the only write). Nothing else is populated, which is itself an
// assertion: a command that started mutating account or balance state would fail
// here with a nil repo rather than pass quietly.
type blockExceptionFixture struct {
	uc      *UseCase
	account *account.MockRepository
	redis   *txRedis.MockRedisRepository
}

func newBlockExceptionFixture(t *testing.T) *blockExceptionFixture {
	t.Helper()

	ctrl := gomock.NewController(t)

	accountRepo := account.NewMockRepository(ctrl)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	return &blockExceptionFixture{
		uc:      &UseCase{AccountRepo: accountRepo, TransactionRedisRepo: redisRepo},
		account: accountRepo,
		redis:   redisRepo,
	}
}

// accountWithAlias builds the minimal account row the alias assertion reads.
func accountWithAlias(alias string) *mmodel.Account {
	return &mmodel.Account{ID: uuid.NewString(), Alias: &alias}
}

// TestCreateAccountBlockExceptions_HappyPath drives a multi-item batch end to end
// and locks the four things a caller depends on: one identifier per item in
// REQUEST order, the alias and amount echoed verbatim, expiresAt derived from the
// applied TTL (default and custom), and the canonical amount reaching the cache.
func TestCreateAccountBlockExceptions_HappyPath(t *testing.T) {
	f := newBlockExceptionFixture(t)

	orgID, ledgerID := uuid.New(), uuid.New()
	customTTL := 900

	input := &mmodel.CreateAccountBlockExceptionsInput{
		Exceptions: []mmodel.CreateAccountBlockExceptionInput{
			{AccountAlias: "@fraud_account", Amount: "150.00"},
			{AccountAlias: "@other_account", Amount: "0.01", TTL: &customTTL},
		},
	}

	f.account.EXPECT().
		ListAccountsByAlias(gomock.Any(), orgID, ledgerID, []string{"@fraud_account", "@other_account"}).
		Return([]*mmodel.Account{accountWithAlias("@fraud_account"), accountWithAlias("@other_account")}, nil)

	var written []txRedis.AccountBlockException

	f.redis.EXPECT().
		CreateAccountBlockExceptions(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, exceptions []txRedis.AccountBlockException) error {
			written = exceptions

			return nil
		})

	before := time.Now().UTC()
	out, err := f.uc.CreateAccountBlockExceptions(context.Background(), orgID, ledgerID, input)
	after := time.Now().UTC()

	require.NoError(t, err)
	require.NotNil(t, out)
	require.Len(t, out.Exceptions, 2, "one identifier per requested exception")
	require.Len(t, written, 2, "the whole batch reaches the cache in one call")

	assert.Equal(t, "@fraud_account", out.Exceptions[0].AccountAlias, "items keep request order")
	assert.Equal(t, "@other_account", out.Exceptions[1].AccountAlias)

	assert.Equal(t, "150.00", out.Exceptions[0].Amount, "the response echoes the requested amount verbatim")
	assert.Equal(t, "150", written[0].Amount, "the cache stores the canonical decimal the Lua script compares")
	assert.Equal(t, "0.01", written[1].Amount, "a canonical amount is stored unchanged")

	assert.NotEqual(t, out.Exceptions[0].AccountBlockExceptionID, out.Exceptions[1].AccountBlockExceptionID,
		"each exception is a distinct single-use identifier")

	for i, got := range out.Exceptions {
		_, err := uuid.Parse(got.AccountBlockExceptionID)
		require.NoErrorf(t, err, "exception %d must carry a UUID identifier", i)
		assert.Equal(t, got.AccountBlockExceptionID, written[i].ID.String(),
			"the returned identifier must be the one written to the cache")
	}

	assert.Equal(t, mmodel.AccountBlockExceptionDefaultTTLSeconds*int(time.Second), int(written[0].TTL),
		"an item without ttl gets the default")
	assert.Equal(t, customTTL*int(time.Second), int(written[1].TTL), "an item with ttl gets it verbatim")

	assertExpiresWithin(t, out.Exceptions[0].ExpiresAt, before, after, written[0].TTL)
	assertExpiresWithin(t, out.Exceptions[1].ExpiresAt, before, after, written[1].TTL)
}

// assertExpiresWithin checks expiresAt was derived from the applied TTL over an
// instant captured inside the call, without reading the clock again.
func assertExpiresWithin(t *testing.T, expiresAt, before, after time.Time, ttl time.Duration) {
	t.Helper()

	assert.False(t, expiresAt.Before(before.Add(ttl)), "expiresAt must be at least now+ttl")
	assert.False(t, expiresAt.After(after.Add(ttl)), "expiresAt must be at most now+ttl")
}

// TestCreateAccountBlockExceptions_SharedInstantAcrossBatch locks that every
// expiresAt in a uniform-TTL batch is derived from ONE captured instant, so a
// large batch reports a single window instead of drifting item by item.
func TestCreateAccountBlockExceptions_SharedInstantAcrossBatch(t *testing.T) {
	f := newBlockExceptionFixture(t)

	orgID, ledgerID := uuid.New(), uuid.New()

	items := make([]mmodel.CreateAccountBlockExceptionInput, 0, 3)
	accounts := make([]*mmodel.Account, 0, 3)
	aliases := make([]string, 0, 3)

	for i := range 3 {
		alias := fmt.Sprintf("@acc_%d", i)
		items = append(items, mmodel.CreateAccountBlockExceptionInput{AccountAlias: alias, Amount: "1"})
		accounts = append(accounts, accountWithAlias(alias))
		aliases = append(aliases, alias)
	}

	f.account.EXPECT().ListAccountsByAlias(gomock.Any(), orgID, ledgerID, aliases).Return(accounts, nil)
	f.redis.EXPECT().CreateAccountBlockExceptions(gomock.Any(), orgID, ledgerID, gomock.Any()).Return(nil)

	out, err := f.uc.CreateAccountBlockExceptions(context.Background(), orgID, ledgerID,
		&mmodel.CreateAccountBlockExceptionsInput{Exceptions: items})
	require.NoError(t, err)

	for _, got := range out.Exceptions[1:] {
		assert.Equal(t, out.Exceptions[0].ExpiresAt, got.ExpiresAt,
			"one captured instant must drive every expiresAt in a uniform-TTL batch")
	}
}

// TestCreateAccountBlockExceptions_DeduplicatesAliasLookup proves the alias
// assertion is ONE batched read over the distinct aliases, not one query per
// item: two items on the same alias must produce a single-element lookup and
// still mint two independent identifiers.
func TestCreateAccountBlockExceptions_DeduplicatesAliasLookup(t *testing.T) {
	f := newBlockExceptionFixture(t)

	orgID, ledgerID := uuid.New(), uuid.New()

	f.account.EXPECT().
		ListAccountsByAlias(gomock.Any(), orgID, ledgerID, []string{"@same"}).
		Return([]*mmodel.Account{accountWithAlias("@same")}, nil).
		Times(1)

	f.redis.EXPECT().CreateAccountBlockExceptions(gomock.Any(), orgID, ledgerID, gomock.Any()).Return(nil)

	out, err := f.uc.CreateAccountBlockExceptions(context.Background(), orgID, ledgerID,
		&mmodel.CreateAccountBlockExceptionsInput{
			Exceptions: []mmodel.CreateAccountBlockExceptionInput{
				{AccountAlias: "@same", Amount: "10"},
				{AccountAlias: "@same", Amount: "20"},
			},
		})
	require.NoError(t, err)
	require.Len(t, out.Exceptions, 2)
	assert.NotEqual(t, out.Exceptions[0].AccountBlockExceptionID, out.Exceptions[1].AccountBlockExceptionID)
}

// TestCreateAccountBlockExceptions_RejectionsWriteNothing sweeps every validation
// rejection. In each case NO expectation is set on the Redis repo, so a command
// that wrote anything before finishing validation fails the gomock controller —
// the all-or-nothing guarantee, asserted rather than assumed.
func TestCreateAccountBlockExceptions_RejectionsWriteNothing(t *testing.T) {
	orgID, ledgerID := uuid.New(), uuid.New()

	zeroTTL := 0
	negativeTTL := -1
	overCapTTL := mmodel.AccountBlockExceptionMaxTTLSeconds + 1

	tests := []struct {
		name      string
		input     *mmodel.CreateAccountBlockExceptionsInput
		aliasStub func(f *blockExceptionFixture)
		wantCode  error
	}{
		{
			name:     "nil input",
			input:    nil,
			wantCode: constant.ErrAccountBlockExceptionsRequired,
		},
		{
			name:     "empty batch",
			input:    &mmodel.CreateAccountBlockExceptionsInput{Exceptions: []mmodel.CreateAccountBlockExceptionInput{}},
			wantCode: constant.ErrAccountBlockExceptionsRequired,
		},
		{
			name:     "batch above the limit",
			input:    &mmodel.CreateAccountBlockExceptionsInput{Exceptions: batchOf(mmodel.AccountBlockExceptionMaxBatchSize + 1)},
			wantCode: constant.ErrAccountBlockExceptionsBatchTooLarge,
		},
		{
			name: "unparseable amount",
			input: &mmodel.CreateAccountBlockExceptionsInput{
				Exceptions: []mmodel.CreateAccountBlockExceptionInput{{AccountAlias: "@a", Amount: "abc"}},
			},
			wantCode: constant.ErrAccountBlockExceptionInvalidAmount,
		},
		{
			name: "zero amount",
			input: &mmodel.CreateAccountBlockExceptionsInput{
				Exceptions: []mmodel.CreateAccountBlockExceptionInput{{AccountAlias: "@a", Amount: "0"}},
			},
			wantCode: constant.ErrAccountBlockExceptionInvalidAmount,
		},
		{
			name: "negative amount",
			input: &mmodel.CreateAccountBlockExceptionsInput{
				Exceptions: []mmodel.CreateAccountBlockExceptionInput{{AccountAlias: "@a", Amount: "-1.50"}},
			},
			wantCode: constant.ErrAccountBlockExceptionInvalidAmount,
		},
		{
			name: "zero ttl",
			input: &mmodel.CreateAccountBlockExceptionsInput{
				Exceptions: []mmodel.CreateAccountBlockExceptionInput{{AccountAlias: "@a", Amount: "1", TTL: &zeroTTL}},
			},
			wantCode: constant.ErrAccountBlockExceptionInvalidTTL,
		},
		{
			name: "negative ttl",
			input: &mmodel.CreateAccountBlockExceptionsInput{
				Exceptions: []mmodel.CreateAccountBlockExceptionInput{{AccountAlias: "@a", Amount: "1", TTL: &negativeTTL}},
			},
			wantCode: constant.ErrAccountBlockExceptionInvalidTTL,
		},
		{
			name: "ttl above the cap",
			input: &mmodel.CreateAccountBlockExceptionsInput{
				Exceptions: []mmodel.CreateAccountBlockExceptionInput{{AccountAlias: "@a", Amount: "1", TTL: &overCapTTL}},
			},
			wantCode: constant.ErrAccountBlockExceptionInvalidTTL,
		},
		{
			name: "unknown alias rejects the whole batch",
			input: &mmodel.CreateAccountBlockExceptionsInput{
				Exceptions: []mmodel.CreateAccountBlockExceptionInput{
					{AccountAlias: "@known", Amount: "1"},
					{AccountAlias: "@ghost", Amount: "2"},
				},
			},
			aliasStub: func(f *blockExceptionFixture) {
				f.account.EXPECT().
					ListAccountsByAlias(gomock.Any(), orgID, ledgerID, []string{"@known", "@ghost"}).
					Return([]*mmodel.Account{accountWithAlias("@known")}, nil)
			},
			wantCode: constant.ErrAccountBlockExceptionAliasNotFound,
		},
		{
			name: "second invalid item is reported even when the first is fine",
			input: &mmodel.CreateAccountBlockExceptionsInput{
				Exceptions: []mmodel.CreateAccountBlockExceptionInput{
					{AccountAlias: "@a", Amount: "1"},
					{AccountAlias: "@b", Amount: "nope"},
				},
			},
			wantCode: constant.ErrAccountBlockExceptionInvalidAmount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newBlockExceptionFixture(t)

			if tt.aliasStub != nil {
				tt.aliasStub(f)
			}

			out, err := f.uc.CreateAccountBlockExceptions(context.Background(), orgID, ledgerID, tt.input)

			require.Error(t, err)
			assert.Nil(t, out)
			assert.Equalf(t, tt.wantCode.Error(), businessErrorCode(t, err),
				"rejection must carry code %s", tt.wantCode.Error())
		})
	}
}

// TestCreateAccountBlockExceptions_ErrorMessageNamesOffendingItem locks the
// contract that a rejection is actionable without bisecting the batch: it names
// the offending item's zero-based index, and for an unknown alias also the alias.
func TestCreateAccountBlockExceptions_ErrorMessageNamesOffendingItem(t *testing.T) {
	orgID, ledgerID := uuid.New(), uuid.New()

	t.Run("invalid amount names the index", func(t *testing.T) {
		f := newBlockExceptionFixture(t)

		_, err := f.uc.CreateAccountBlockExceptions(context.Background(), orgID, ledgerID,
			&mmodel.CreateAccountBlockExceptionsInput{
				Exceptions: []mmodel.CreateAccountBlockExceptionInput{
					{AccountAlias: "@a", Amount: "1"},
					{AccountAlias: "@b", Amount: "1"},
					{AccountAlias: "@c", Amount: "-3"},
				},
			})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "index 2", "the message must name the offending item")
	})

	t.Run("unknown alias names the index and the alias", func(t *testing.T) {
		f := newBlockExceptionFixture(t)

		f.account.EXPECT().
			ListAccountsByAlias(gomock.Any(), orgID, ledgerID, []string{"@known", "@ghost"}).
			Return([]*mmodel.Account{accountWithAlias("@known")}, nil)

		_, err := f.uc.CreateAccountBlockExceptions(context.Background(), orgID, ledgerID,
			&mmodel.CreateAccountBlockExceptionsInput{
				Exceptions: []mmodel.CreateAccountBlockExceptionInput{
					{AccountAlias: "@known", Amount: "1"},
					{AccountAlias: "@ghost", Amount: "2"},
				},
			})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "index 1")
		assert.Contains(t, err.Error(), "@ghost")
	})
}

// TestCreateAccountBlockExceptions_MaxBatchAccepted pins the boundary: exactly
// AccountBlockExceptionMaxBatchSize items is accepted, so the cap is off-by-one
// safe in the permissive direction too.
func TestCreateAccountBlockExceptions_MaxBatchAccepted(t *testing.T) {
	f := newBlockExceptionFixture(t)

	orgID, ledgerID := uuid.New(), uuid.New()
	items := batchOf(mmodel.AccountBlockExceptionMaxBatchSize)

	accounts := make([]*mmodel.Account, 0, len(items))
	for _, item := range items {
		accounts = append(accounts, accountWithAlias(item.AccountAlias))
	}

	f.account.EXPECT().ListAccountsByAlias(gomock.Any(), orgID, ledgerID, gomock.Any()).Return(accounts, nil)
	f.redis.EXPECT().CreateAccountBlockExceptions(gomock.Any(), orgID, ledgerID, gomock.Any()).Return(nil)

	out, err := f.uc.CreateAccountBlockExceptions(context.Background(), orgID, ledgerID,
		&mmodel.CreateAccountBlockExceptionsInput{Exceptions: items})

	require.NoError(t, err)
	assert.Len(t, out.Exceptions, mmodel.AccountBlockExceptionMaxBatchSize)
}

// TestCreateAccountBlockExceptions_PropagatesCollaboratorFailures asserts a
// failing read or write is surfaced, never swallowed into a partial success: the
// caller must not receive identifiers it cannot use.
func TestCreateAccountBlockExceptions_PropagatesCollaboratorFailures(t *testing.T) {
	orgID, ledgerID := uuid.New(), uuid.New()
	boom := errors.New("boom")

	input := &mmodel.CreateAccountBlockExceptionsInput{
		Exceptions: []mmodel.CreateAccountBlockExceptionInput{{AccountAlias: "@a", Amount: "1"}},
	}

	t.Run("alias lookup failure", func(t *testing.T) {
		f := newBlockExceptionFixture(t)

		f.account.EXPECT().ListAccountsByAlias(gomock.Any(), orgID, ledgerID, []string{"@a"}).Return(nil, boom)

		out, err := f.uc.CreateAccountBlockExceptions(context.Background(), orgID, ledgerID, input)

		require.ErrorIs(t, err, boom)
		assert.Nil(t, out)
	})

	t.Run("cache write failure", func(t *testing.T) {
		f := newBlockExceptionFixture(t)

		f.account.EXPECT().ListAccountsByAlias(gomock.Any(), orgID, ledgerID, []string{"@a"}).
			Return([]*mmodel.Account{accountWithAlias("@a")}, nil)
		f.redis.EXPECT().CreateAccountBlockExceptions(gomock.Any(), orgID, ledgerID, gomock.Any()).Return(boom)

		out, err := f.uc.CreateAccountBlockExceptions(context.Background(), orgID, ledgerID, input)

		require.ErrorIs(t, err, boom)
		assert.Nil(t, out, "a failed write must not report identifiers the caller cannot use")
	})
}

// businessErrorCode extracts the numeric registry code from a business error.
// It reads the TYPED struct rather than matching on Error(), because the typed
// error is what the HTTP layer classifies on and only some of the types
// interpolate the code into their string form.
func businessErrorCode(t *testing.T, err error) string {
	t.Helper()

	var validationErr pkg.ValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Code
	}

	var notFoundErr pkg.EntityNotFoundError
	if errors.As(err, &notFoundErr) {
		return notFoundErr.Code
	}

	t.Fatalf("error is not a classified business error: %T (%v)", err, err)

	return ""
}

// batchOf builds n valid batch items with distinct aliases.
func batchOf(n int) []mmodel.CreateAccountBlockExceptionInput {
	items := make([]mmodel.CreateAccountBlockExceptionInput, 0, n)
	for i := range n {
		items = append(items, mmodel.CreateAccountBlockExceptionInput{
			AccountAlias: fmt.Sprintf("@alias_%d", i),
			Amount:       "1",
		})
	}

	return items
}
