// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountregistration

import (
	"database/sql"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedTime is shared across the tests so we don't rely on time.Now() — per Midaz
// CLAUDE.md rule "do not use time.Now() in tests".
var fixedTime = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

// TestAccountRegistrationPostgreSQLModel_ToEntity_AllFieldsPopulated covers the
// fully-populated row → entity conversion. Every nullable column carries Valid=true so
// the ToEntity branches that pull the inner value all execute.
func TestAccountRegistrationPostgreSQLModel_ToEntity_AllFieldsPopulated(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()
	accountID := uuid.New()
	aliasID := uuid.New()

	completedAt := fixedTime.Add(2 * time.Hour)
	nextRetryAt := fixedTime.Add(time.Hour)
	claimedAt := fixedTime.Add(30 * time.Minute)
	lastRecoveredAt := fixedTime.Add(45 * time.Minute)

	model := &AccountRegistrationPostgreSQLModel{
		ID:              id,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		HolderID:        holderID,
		IdempotencyKey:  "idem-key-1",
		RequestHash:     "hash-abc",
		AccountID:       uuid.NullUUID{UUID: accountID, Valid: true},
		CRMAliasID:      uuid.NullUUID{UUID: aliasID, Valid: true},
		Status:          string(mmodel.AccountRegistrationCompleted),
		FailureCode:     sql.NullString{String: "CRM_TRANSIENT", Valid: true},
		FailureMessage:  sql.NullString{String: "downstream timeout", Valid: true},
		RetryCount:      3,
		NextRetryAt:     sql.NullTime{Time: nextRetryAt, Valid: true},
		ClaimedBy:       sql.NullString{String: "worker-7", Valid: true},
		ClaimedAt:       sql.NullTime{Time: claimedAt, Valid: true},
		LastRecoveredAt: sql.NullTime{Time: lastRecoveredAt, Valid: true},
		CreatedAt:       fixedTime,
		UpdatedAt:       fixedTime.Add(time.Minute),
		CompletedAt:     sql.NullTime{Time: completedAt, Valid: true},
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, id, entity.ID)
	assert.Equal(t, organizationID, entity.OrganizationID)
	assert.Equal(t, ledgerID, entity.LedgerID)
	assert.Equal(t, holderID, entity.HolderID)
	assert.Equal(t, "idem-key-1", entity.IdempotencyKey)
	assert.Equal(t, "hash-abc", entity.RequestHash)
	assert.Equal(t, mmodel.AccountRegistrationCompleted, entity.Status)
	assert.Equal(t, 3, entity.RetryCount)
	assert.Equal(t, fixedTime, entity.CreatedAt)
	assert.Equal(t, fixedTime.Add(time.Minute), entity.UpdatedAt)

	require.NotNil(t, entity.AccountID)
	assert.Equal(t, accountID, *entity.AccountID)

	require.NotNil(t, entity.CRMAliasID)
	assert.Equal(t, aliasID, *entity.CRMAliasID)

	require.NotNil(t, entity.FailureCode)
	assert.Equal(t, "CRM_TRANSIENT", *entity.FailureCode)

	require.NotNil(t, entity.FailureMessage)
	assert.Equal(t, "downstream timeout", *entity.FailureMessage)

	require.NotNil(t, entity.NextRetryAt)
	assert.Equal(t, nextRetryAt, *entity.NextRetryAt)

	require.NotNil(t, entity.ClaimedBy)
	assert.Equal(t, "worker-7", *entity.ClaimedBy)

	require.NotNil(t, entity.ClaimedAt)
	assert.Equal(t, claimedAt, *entity.ClaimedAt)

	require.NotNil(t, entity.LastRecoveredAt)
	assert.Equal(t, lastRecoveredAt, *entity.LastRecoveredAt)

	require.NotNil(t, entity.CompletedAt)
	assert.Equal(t, completedAt, *entity.CompletedAt)
}

// TestAccountRegistrationPostgreSQLModel_ToEntity_AllNullablesNull covers the inverse:
// every nullable column is Valid=false, so the entity's pointer fields must remain nil.
// This is the regression guard against the zero-value trap (rule cited in the file's
// header comment).
func TestAccountRegistrationPostgreSQLModel_ToEntity_AllNullablesNull(t *testing.T) {
	t.Parallel()

	model := &AccountRegistrationPostgreSQLModel{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		HolderID:       uuid.New(),
		IdempotencyKey: "idem-key-2",
		RequestHash:    "hash-xyz",
		Status:         string(mmodel.AccountRegistrationReceived),
		RetryCount:     0,
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
		// All sql.Null* fields default to Valid=false.
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, mmodel.AccountRegistrationReceived, entity.Status)
	assert.Nil(t, entity.AccountID, "AccountID must remain nil when AccountID.Valid=false")
	assert.Nil(t, entity.CRMAliasID, "CRMAliasID must remain nil when CRMAliasID.Valid=false")
	assert.Nil(t, entity.FailureCode)
	assert.Nil(t, entity.FailureMessage)
	assert.Nil(t, entity.NextRetryAt)
	assert.Nil(t, entity.ClaimedBy)
	assert.Nil(t, entity.ClaimedAt)
	assert.Nil(t, entity.LastRecoveredAt)
	assert.Nil(t, entity.CompletedAt)
}

// TestAccountRegistrationPostgreSQLModel_FromEntity_AllFieldsPopulated mirrors the
// AllFieldsPopulated ToEntity test in reverse: every optional pointer is non-nil so all
// nullable assignments fire.
func TestAccountRegistrationPostgreSQLModel_FromEntity_AllFieldsPopulated(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	holderID := uuid.New()
	accountID := uuid.New()
	aliasID := uuid.New()

	failureCode := "ACTIVATE_FAILED"
	failureMessage := "ledger update failed"
	claimedBy := "worker-3"
	completedAt := fixedTime.Add(time.Hour)
	nextRetryAt := fixedTime.Add(2 * time.Hour)
	claimedAt := fixedTime.Add(15 * time.Minute)
	lastRecoveredAt := fixedTime.Add(20 * time.Minute)

	entity := &mmodel.AccountRegistration{
		ID:              id,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		HolderID:        holderID,
		IdempotencyKey:  "idem-key-3",
		RequestHash:     "hash-def",
		AccountID:       &accountID,
		CRMAliasID:      &aliasID,
		Status:          mmodel.AccountRegistrationFailedRetryable,
		FailureCode:     &failureCode,
		FailureMessage:  &failureMessage,
		RetryCount:      2,
		NextRetryAt:     &nextRetryAt,
		ClaimedBy:       &claimedBy,
		ClaimedAt:       &claimedAt,
		LastRecoveredAt: &lastRecoveredAt,
		CreatedAt:       fixedTime,
		UpdatedAt:       fixedTime.Add(time.Minute),
		CompletedAt:     &completedAt,
	}

	model := &AccountRegistrationPostgreSQLModel{}
	model.FromEntity(entity)

	assert.Equal(t, id, model.ID)
	assert.Equal(t, organizationID, model.OrganizationID)
	assert.Equal(t, ledgerID, model.LedgerID)
	assert.Equal(t, holderID, model.HolderID)
	assert.Equal(t, "idem-key-3", model.IdempotencyKey)
	assert.Equal(t, "hash-def", model.RequestHash)
	assert.Equal(t, string(mmodel.AccountRegistrationFailedRetryable), model.Status)
	assert.Equal(t, 2, model.RetryCount)
	assert.Equal(t, fixedTime, model.CreatedAt)
	assert.Equal(t, fixedTime.Add(time.Minute), model.UpdatedAt)

	require.True(t, model.AccountID.Valid)
	assert.Equal(t, accountID, model.AccountID.UUID)

	require.True(t, model.CRMAliasID.Valid)
	assert.Equal(t, aliasID, model.CRMAliasID.UUID)

	require.True(t, model.FailureCode.Valid)
	assert.Equal(t, failureCode, model.FailureCode.String)

	require.True(t, model.FailureMessage.Valid)
	assert.Equal(t, failureMessage, model.FailureMessage.String)

	require.True(t, model.NextRetryAt.Valid)
	assert.Equal(t, nextRetryAt, model.NextRetryAt.Time)

	require.True(t, model.ClaimedBy.Valid)
	assert.Equal(t, claimedBy, model.ClaimedBy.String)

	require.True(t, model.ClaimedAt.Valid)
	assert.Equal(t, claimedAt, model.ClaimedAt.Time)

	require.True(t, model.LastRecoveredAt.Valid)
	assert.Equal(t, lastRecoveredAt, model.LastRecoveredAt.Time)

	require.True(t, model.CompletedAt.Valid)
	assert.Equal(t, completedAt, model.CompletedAt.Time)
}

// TestAccountRegistrationPostgreSQLModel_FromEntity_AllNilOptionals verifies that nil
// pointer fields produce sql.Null* values with Valid=false. This is the round-trip
// counterpart to AllNullablesNull above.
func TestAccountRegistrationPostgreSQLModel_FromEntity_AllNilOptionals(t *testing.T) {
	t.Parallel()

	entity := &mmodel.AccountRegistration{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		HolderID:       uuid.New(),
		IdempotencyKey: "idem-key-4",
		RequestHash:    "hash-empty",
		Status:         mmodel.AccountRegistrationReceived,
		RetryCount:     0,
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
		// All optional pointer fields are nil.
	}

	model := &AccountRegistrationPostgreSQLModel{}
	model.FromEntity(entity)

	assert.False(t, model.AccountID.Valid, "AccountID must be Valid=false when entity.AccountID is nil")
	assert.False(t, model.CRMAliasID.Valid, "CRMAliasID must be Valid=false when entity.CRMAliasID is nil")
	assert.False(t, model.FailureCode.Valid)
	assert.False(t, model.FailureMessage.Valid)
	assert.False(t, model.NextRetryAt.Valid)
	assert.False(t, model.ClaimedBy.Valid)
	assert.False(t, model.ClaimedAt.Valid)
	assert.False(t, model.LastRecoveredAt.Valid)
	assert.False(t, model.CompletedAt.Valid)
}

// TestAccountRegistrationPostgreSQLModel_RoundTrip asserts ToEntity ∘ FromEntity is the
// identity for every field. This guards against subtle drift where a new field is added
// to one side but not the other — the test fails immediately with a useful diff.
func TestAccountRegistrationPostgreSQLModel_RoundTrip(t *testing.T) {
	t.Parallel()

	accountID := uuid.New()
	aliasID := uuid.New()
	failureCode := "CRM_CONFLICT"
	failureMessage := "alias already exists"
	claimedBy := "worker-roundtrip"
	completedAt := fixedTime.Add(3 * time.Hour)
	nextRetryAt := fixedTime.Add(time.Hour)
	claimedAt := fixedTime.Add(10 * time.Minute)
	lastRecoveredAt := fixedTime.Add(25 * time.Minute)

	original := &mmodel.AccountRegistration{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		LedgerID:        uuid.New(),
		HolderID:        uuid.New(),
		IdempotencyKey:  "idem-roundtrip",
		RequestHash:     "hash-roundtrip",
		AccountID:       &accountID,
		CRMAliasID:      &aliasID,
		Status:          mmodel.AccountRegistrationCompleted,
		FailureCode:     &failureCode,
		FailureMessage:  &failureMessage,
		RetryCount:      1,
		NextRetryAt:     &nextRetryAt,
		ClaimedBy:       &claimedBy,
		ClaimedAt:       &claimedAt,
		LastRecoveredAt: &lastRecoveredAt,
		CreatedAt:       fixedTime,
		UpdatedAt:       fixedTime.Add(time.Minute),
		CompletedAt:     &completedAt,
	}

	model := &AccountRegistrationPostgreSQLModel{}
	model.FromEntity(original)

	round := model.ToEntity()

	assert.Equal(t, original, round, "round-trip must preserve every field exactly")
}
