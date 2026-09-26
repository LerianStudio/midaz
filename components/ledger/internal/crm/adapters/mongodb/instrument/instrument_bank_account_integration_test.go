//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package instrument

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

// These tests write through the repository directly, skipping the service pre-check, so what
// refuses a twin here is the unique index alone: the path two concurrent creates race on.

func bankAccountInstrument(t *testing.T, bankID, branch, account, accountType string) *mmodel.Instrument {
	t.Helper()

	i := mongotestutil.CreateTestInstrumentSimple(t, uuid.New(), "account-"+uuid.NewString()[:8], "12345678901")
	i.BankingDetails = &mmodel.BankingDetails{BankID: &bankID, Branch: &branch, Account: &account, Type: &accountType}

	return i
}

func requireBankAccountConflict(t *testing.T, err error) {
	t.Helper()

	var conflict pkg.EntityConflictError

	require.True(t, errors.As(err, &conflict), "want a 409 conflict, got %v", err)
	assert.Equal(t, cn.ErrBankAccountAlreadyRegistered.Error(), conflict.Code)
}

func TestIntegration_InstrumentRepo_Create_BankAccountTwinRefusedByIndex(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	organizationID := "org-bankacct-" + uuid.NewString()[:8]
	repo := createRepository(t, container, organizationID)
	ctx := context.Background()

	_, err := repo.Create(ctx, organizationID, bankAccountInstrument(t, "001", "0001", "123456", "TRAN"))
	require.NoError(t, err)

	_, err = repo.Create(ctx, organizationID, bankAccountInstrument(t, "001", "0001", "123456", "PG"))
	requireBankAccountConflict(t, err)

	_, err = repo.Create(ctx, organizationID, bankAccountInstrument(t, "001", "0002", "123456", "TRAN"))
	require.NoError(t, err, "the same account at another branch is another account")

	_, err = repo.Create(ctx, organizationID, bankAccountInstrument(t, "237", "0001", "123456", "TRAN"))
	require.NoError(t, err, "the same account at another bank is another account")

	for n := range 2 {
		branchless := bankAccountInstrument(t, "001", "", "999999", "PG")
		branchless.BankingDetails.Branch = nil

		_, err = repo.Create(ctx, organizationID, branchless)
		if n == 0 {
			require.NoError(t, err)
			continue
		}

		requireBankAccountConflict(t, err)
	}
}

func TestIntegration_InstrumentRepo_Update_BankAccountTwinRefusedByIndex(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	organizationID := "org-bankupd-" + uuid.NewString()[:8]
	repo := createRepository(t, container, organizationID)
	ctx := context.Background()

	_, err := repo.Create(ctx, organizationID, bankAccountInstrument(t, "001", "0001", "123456", "CACC"))
	require.NoError(t, err)

	moving := bankAccountInstrument(t, "001", "0002", "123456", "CACC")
	_, err = repo.Create(ctx, organizationID, moving)
	require.NoError(t, err)

	_, err = repo.Update(ctx, organizationID, *moving.HolderID, *moving.ID,
		&mmodel.Instrument{BankingDetails: &mmodel.BankingDetails{Branch: strPtr("0001")}}, nil)
	requireBankAccountConflict(t, err)
}

func TestIntegration_InstrumentRepo_ClearingTheAccountReleasesIt(t *testing.T) {
	for name, clear := range map[string]struct {
		patch          *mmodel.Instrument
		fieldsToRemove []string
	}{
		"account removed": {patch: &mmodel.Instrument{}, fieldsToRemove: []string{"bankingDetails.account"}},
		"account emptied": {patch: &mmodel.Instrument{BankingDetails: &mmodel.BankingDetails{Account: strPtr("")}}},
	} {
		t.Run(name, func(t *testing.T) {
			container := mongotestutil.SetupReusableContainer(t)
			organizationID := "org-bankclr-" + uuid.NewString()[:8]
			repo := createRepository(t, container, organizationID)
			ctx := context.Background()

			first := bankAccountInstrument(t, "001", "0001", "123456", "CACC")
			_, err := repo.Create(ctx, organizationID, first)
			require.NoError(t, err)

			_, err = repo.Update(ctx, organizationID, *first.HolderID, *first.ID, clear.patch, clear.fieldsToRemove)
			require.NoError(t, err)

			_, err = findRawInstrument(t, container, organizationID, *first.ID).LookupErr("search", "banking_details_account")
			require.Error(t, err, "the account token must leave with the account")

			_, err = repo.Create(ctx, organizationID, bankAccountInstrument(t, "001", "0001", "123456", "CACC"))
			require.NoError(t, err, "an account no instrument holds any more is free to register")
		})
	}
}

// recordingLogger captures log lines; ContextWithLogger needs only log.Universal.
type recordingLogger struct {
	mu      sync.Mutex
	entries []recordedEntry
}

type recordedEntry struct {
	level  int
	msg    string
	fields map[string]any
}

func (r *recordingLogger) Log(_ context.Context, level int, msg string, fields ...any) {
	entry := recordedEntry{level: level, msg: msg, fields: map[string]any{}}

	for _, f := range fields {
		if field, ok := f.(libLog.Field); ok {
			entry.fields[field.Key] = field.Value
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = append(r.entries, entry)
}

func TestIntegration_InstrumentRepo_BankAccountIndexBuildFailureDoesNotBlockCreate(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	organizationID := "org-bankdup-" + uuid.NewString()[:8]
	repo := createRepository(t, container, organizationID)
	collName := strings.ToLower("aliases_" + organizationID)

	// Two live rows already share one bank account, as an organization written before this rule holds them.
	twin := bson.M{
		"banking_details": bson.M{"bank_id": "001", "branch": "0001", "account": "ciphertext"},
		"search":          bson.M{"banking_details_account": "token-123456"},
		"deleted_at":      nil,
	}
	for range 2 {
		doc := bson.M{"_id": uuid.New(), "holder_id": uuid.New(), "account_id": uuid.NewString(), "ledger_id": uuid.NewString()}
		for k, v := range twin {
			doc[k] = v
		}

		_, err := container.Database.Collection(collName).InsertOne(context.Background(), doc)
		require.NoError(t, err)
	}

	logger := &recordingLogger{}
	ctx := libObservability.ContextWithLogger(context.Background(), logger)

	_, err := repo.Create(ctx, organizationID, bankAccountInstrument(t, "001", "0001", "654321", "CACC"))
	require.NoError(t, err, "a failed uniqueness index build must not block the organization's creates")

	_, err = repo.Create(ctx, organizationID, bankAccountInstrument(t, "001", "0001", "777777", "CACC"))
	require.NoError(t, err)

	var warned []recordedEntry
	for _, e := range logger.entries {
		if e.level == int(libLog.LevelWarn) && e.fields["index"] == bankAccountIndexName {
			warned = append(warned, e)
		}
	}

	require.Len(t, warned, 1, "the failed build is attempted and reported once per process, as a WARN naming the index")
	assert.Equal(t, organizationID, warned[0].fields["organization_id"])

	buildErr, ok := warned[0].fields["error"].(error)
	require.True(t, ok)
	assert.ErrorContains(t, buildErr, "E11000", "the build is refused by the existing twins")

	cursor, err := container.Database.Collection(collName).Indexes().List(context.Background())
	require.NoError(t, err)

	var indexes []bson.M
	require.NoError(t, cursor.All(context.Background(), &indexes))

	names := make([]any, 0, len(indexes))
	for _, idx := range indexes {
		names = append(names, idx["name"])
	}

	assert.NotContains(t, names, bankAccountIndexName)
	assert.Len(t, indexes, len(indexModels())+1, "the collection's other indexes are still built")
}

func strPtr(s string) *string { return &s }
