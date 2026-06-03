// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestNewUpdateLimitCommand(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	cmd, err := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)

	require.NoError(t, err)
	assert.NotNil(t, cmd)
	assert.Equal(t, mockRepo, cmd.repo)
}

func TestNewUpdateLimitCommand_NilRepository(t *testing.T) {
	cmd, err := NewUpdateLimitCommand(nil, testutil.NewDefaultMockClock(), nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilLimitRepository)
	assert.Nil(t, cmd)
}

func TestNewUpdateLimitCommand_NilClock(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	cmd, err := NewUpdateLimitCommand(mockRepo, nil, nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilClock)
	assert.Nil(t, cmd)
}

// expectTxSuccess wires the BeginTx → UpdateWithTx → RecordLimitEventWithTx →
// Commit chain for a successful update path. Shared by happy-path table cases.
func expectTxSuccess(
	t *testing.T,
	mockRepo *MockLimitRepository,
	auditWriter *MockAuditWriter,
	txBeginner *pgdbMocks.MockTxBeginner,
	mockTx *pgdbMocks.MockTx,
	limitID uuid.UUID,
) {
	t.Helper()
	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(),
			gomock.AssignableToTypeOf(mockTx),
			model.AuditEventLimitUpdated,
			model.AuditActionUpdate,
			limitID,
			gomock.Any(),
			gomock.Any(),
			"Limit updated via API",
		).Return(nil),
		mockTx.EXPECT().Commit().Return(nil),
	)
}

func TestUpdateLimitCommand_Execute(t *testing.T) {
	limitID := testutil.MustDeterministicUUID(1)
	now := testutil.FixedTime().UTC()

	// newExistingLimit creates a fresh limit instance per test to avoid mutation side effects.
	newExistingLimit := func() *model.Limit {
		return &model.Limit{
			ID:        limitID,
			Name:      "Original Limit",
			LimitType: model.LimitTypeDaily,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "USD",
			Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2))}},
			Status:    model.LimitStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	tests := []struct {
		name        string
		limitID     uuid.UUID
		input       *UpdateLimitInput
		setupMock   func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx)
		expectError bool
		errorIs     error
		validate    func(*testing.T, *model.Limit)
	}{
		{
			name:    "Success - update name only",
			limitID: limitID,
			input: &UpdateLimitInput{
				Name: testutil.StringPtr("Updated Limit Name"),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
				expectTxSuccess(t, m, aw, tb, tx, limitID)
			},
			expectError: false,
			validate: func(t *testing.T, limit *model.Limit) {
				assert.Equal(t, "Updated Limit Name", limit.Name)
			},
		},
		{
			name:    "Success - update maxAmount only",
			limitID: limitID,
			input: &UpdateLimitInput{
				MaxAmount: testutil.Ptr(decimal.RequireFromString("2000")),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
				expectTxSuccess(t, m, aw, tb, tx, limitID)
			},
			expectError: false,
			validate: func(t *testing.T, limit *model.Limit) {
				assert.Equal(t, "2000", limit.MaxAmount.String())
			},
		},
		{
			name:    "Success - update description",
			limitID: limitID,
			input: &UpdateLimitInput{
				Description: testutil.StringPtr("Updated description"),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
				expectTxSuccess(t, m, aw, tb, tx, limitID)
			},
			expectError: false,
			validate: func(t *testing.T, limit *model.Limit) {
				require.NotNil(t, limit.Description)
				assert.Equal(t, "Updated description", *limit.Description)
			},
		},
		{
			name:    "Success - update scopes",
			limitID: limitID,
			input: &UpdateLimitInput{
				Scopes: &[]model.Scope{{PortfolioID: testutil.UUIDPtr(testutil.MustDeterministicUUID(10))}},
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
				expectTxSuccess(t, m, aw, tb, tx, limitID)
			},
			expectError: false,
			validate: func(t *testing.T, limit *model.Limit) {
				assert.Len(t, limit.Scopes, 1)
				assert.NotNil(t, limit.Scopes[0].PortfolioID)
			},
		},
		{
			name:    "Success - update multiple fields",
			limitID: limitID,
			input: &UpdateLimitInput{
				Name:        testutil.StringPtr("Multi-Update Limit"),
				MaxAmount:   testutil.Ptr(decimal.RequireFromString("3000")),
				Description: testutil.StringPtr("New description"),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
				expectTxSuccess(t, m, aw, tb, tx, limitID)
			},
			expectError: false,
			validate: func(t *testing.T, limit *model.Limit) {
				assert.Equal(t, "Multi-Update Limit", limit.Name)
				assert.Equal(t, "3000", limit.MaxAmount.String())
			},
		},
		{
			name:    "Success - no changes (all nil)",
			limitID: limitID,
			input:   &UpdateLimitInput{},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
				// No tx / no audit / no UpdateWithTx expected when no fields are modified.
				// Pin the contract explicitly so the no-op invariant is asserted, not assumed.
				tb.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
				m.EXPECT().UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				aw.EXPECT().RecordLimitEventWithTx(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(),
				).Times(0)
			},
			expectError: false,
		},
		{
			name:    "Failure - limit not found",
			limitID: testutil.MustDeterministicUUID(20),
			input: &UpdateLimitInput{
				Name: testutil.StringPtr("New Name"),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), gomock.Any()).Return(nil, constant.ErrLimitNotFound)
			},
			expectError: true,
			errorIs:     constant.ErrLimitNotFound,
		},
		{
			name:    "Failure - empty name",
			limitID: limitID,
			input: &UpdateLimitInput{
				Name: testutil.StringPtr(""),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
			},
			expectError: true,
		},
		{
			name:    "Failure - zero maxAmount",
			limitID: limitID,
			input: &UpdateLimitInput{
				MaxAmount: testutil.Ptr(decimal.RequireFromString("0")),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
			},
			expectError: true,
		},
		{
			name:    "Failure - negative maxAmount",
			limitID: limitID,
			input: &UpdateLimitInput{
				MaxAmount: testutil.Ptr(decimal.RequireFromString("-1")),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
			},
			expectError: true,
		},
		{
			name:    "Failure - empty scopes array",
			limitID: limitID,
			input: &UpdateLimitInput{
				Scopes: &[]model.Scope{},
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
			},
			expectError: true,
		},
		{
			name:    "Failure - invalid scope in array",
			limitID: limitID,
			input: &UpdateLimitInput{
				Scopes: &[]model.Scope{{}},
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
			},
			expectError: true,
		},
		{
			name:    "Failure - repository GetByID error",
			limitID: limitID,
			input: &UpdateLimitInput{
				Name: testutil.StringPtr("New Name"),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
		{
			name:    "Failure - repository Update error (inside tx)",
			limitID: limitID,
			input: &UpdateLimitInput{
				Name: testutil.StringPtr("New Name"),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
				gomock.InOrder(
					tb.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil),
					m.EXPECT().UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(tx), gomock.Any()).Return(errors.New("database error")),
					tx.EXPECT().Rollback().Return(nil),
				)
				tx.EXPECT().Commit().Times(0)
			},
			expectError: true,
		},
		{
			name:    "Failure - update deleted limit",
			limitID: limitID,
			input: &UpdateLimitInput{
				Name: testutil.StringPtr("New Name"),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				deletedLimit := &model.Limit{
					ID:        limitID,
					Name:      "Deleted Limit",
					LimitType: model.LimitTypeDaily,
					MaxAmount: decimal.RequireFromString("1000"),
					Currency:  "USD",
					Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(30))}},
					Status:    model.LimitStatusDeleted,
					CreatedAt: now,
					UpdatedAt: now,
				}
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(deletedLimit, nil)
			},
			expectError: true,
			errorIs:     constant.ErrLimitAlreadyDeleted,
		},
		{
			name:    "Failure - description with XSS content",
			limitID: limitID,
			input: &UpdateLimitInput{
				Description: testutil.StringPtr("<script>alert('xss')</script>"),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(newExistingLimit(), nil)
			},
			expectError: true,
			errorIs:     constant.ErrLimitDescriptionInvalidChars,
		},
		{
			name:    "Failure - nil input",
			limitID: limitID,
			input:   nil,
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
			},
			expectError: true,
			errorIs:     constant.ErrLimitNilInput,
		},
		{
			name:    "Failure - nil UUID",
			limitID: uuid.Nil,
			input: &UpdateLimitInput{
				Name: testutil.StringPtr("New Name"),
			},
			setupMock: func(t *testing.T, m *MockLimitRepository, aw *MockAuditWriter, tb *pgdbMocks.MockTxBeginner, tx *pgdbMocks.MockTx) {
			},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := NewMockLimitRepository(ctrl)
			auditWriter := NewMockAuditWriter(ctrl)
			txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
			mockTx := pgdbMocks.NewMockTx(ctrl)

			tc.setupMock(t, mockRepo, auditWriter, txBeginner, mockTx)

			cmd, cmdErr := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
			require.NoError(t, cmdErr)
			result, err := cmd.Execute(context.Background(), tc.limitID, tc.input)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorIs != nil {
					assert.ErrorIs(t, err, tc.errorIs)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tc.validate != nil {
					tc.validate(t, result)
				}
			}
		})
	}
}

// TestUpdateLimitCommand_Execute_BeginTxError verifies BeginTx error handling.
func TestUpdateLimitCommand_Execute_BeginTxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(200)
	now := testutil.FixedTime().UTC()

	existing := &model.Limit{
		ID:        limitID,
		Name:      "Original",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(201))}},
		Status:    model.LimitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	beginErr := errors.New("connection refused")

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(existing, nil)
	txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, beginErr)

	mockRepo.EXPECT().UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().RecordLimitEventWithTx(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Times(0)

	cmd, cmdErr := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID, &UpdateLimitInput{Name: testutil.StringPtr("New Name")})

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr)
	assert.Nil(t, result)
}

// TestUpdateLimitCommand_Execute_AuditError_Rollback verifies audit failure triggers rollback.
func TestUpdateLimitCommand_Execute_AuditError_Rollback(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(210)
	now := testutil.FixedTime().UTC()

	existing := &model.Limit{
		ID:        limitID,
		Name:      "Original",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(211))}},
		Status:    model.LimitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	auditErr := errors.New("audit insert failed")

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(existing, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(),
			gomock.AssignableToTypeOf(mockTx),
			model.AuditEventLimitUpdated,
			model.AuditActionUpdate,
			limitID,
			gomock.Any(),
			gomock.Any(),
			"Limit updated via API",
		).Return(auditErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)
	mockTx.EXPECT().Commit().Times(0)

	cmd, cmdErr := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID, &UpdateLimitInput{Name: testutil.StringPtr("Updated")})

	require.Error(t, err)
	assert.ErrorIs(t, err, auditErr)
	assert.Nil(t, result)
}

// TestUpdateLimitCommand_Execute_CommitError verifies Commit error handling.
func TestUpdateLimitCommand_Execute_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	ctx := context.Background()
	limitID := testutil.MustDeterministicUUID(220)
	now := testutil.FixedTime().UTC()

	existing := &model.Limit{
		ID:        limitID,
		Name:      "Original",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(221))}},
		Status:    model.LimitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	commitErr := errors.New("commit failed")

	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(existing, nil)

	gomock.InOrder(
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil),
		mockRepo.EXPECT().UpdateWithTx(gomock.Any(), gomock.AssignableToTypeOf(mockTx), gomock.Any()).Return(nil),
		auditWriter.EXPECT().RecordLimitEventWithTx(
			gomock.Any(),
			gomock.AssignableToTypeOf(mockTx),
			model.AuditEventLimitUpdated,
			model.AuditActionUpdate,
			limitID,
			gomock.Any(),
			gomock.Any(),
			"Limit updated via API",
		).Return(nil),
		mockTx.EXPECT().Commit().Return(commitErr),
		mockTx.EXPECT().Rollback().Return(nil),
	)

	cmd, cmdErr := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	result, err := cmd.Execute(ctx, limitID, &UpdateLimitInput{Name: testutil.StringPtr("Updated")})

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr)
	assert.Nil(t, result)
}

// TestUpdateLimitCommand_Execute_ContextCancellation_PreFetch verifies that when
// the caller's context is already cancelled before Execute is invoked, the
// pre-fetch ctx.Err() guard short-circuits the operation: no GetByID, no
// transaction, no audit event, and context.Canceled is surfaced to the caller.
func TestUpdateLimitCommand_Execute_ContextCancellation_PreFetch(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	// Explicitly assert repository/tx/audit methods are NOT called when context is
	// cancelled before the pre-fetch guard.
	mockRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().RecordLimitEventWithTx(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Times(0)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately, before Execute runs.

	cmd, cmdErr := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	newName := "updated-limit-name"
	result, err := cmd.Execute(ctx, testutil.MustDeterministicUUID(300), &UpdateLimitInput{Name: &newName})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}

// TestUpdateLimitCommand_Execute_ContextCancellation_PreTx verifies that when
// the caller's context is cancelled AFTER GetByID returns (i.e. between fetch
// and tx), the post-validation/pre-tx ctx.Err() guard fires: the transaction
// is never begun, UpdateWithTx is never called, no audit event is recorded,
// and context.Canceled is surfaced to the caller.
func TestUpdateLimitCommand_Execute_ContextCancellation_PreTx(t *testing.T) {
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(310)
	now := testutil.FixedTime().UTC()

	existing := &model.Limit{
		ID:        limitID,
		Name:      "Original",
		LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(311))}},
		Status:    model.LimitStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockRepo := NewMockLimitRepository(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	ctx, cancel := context.WithCancel(t.Context())

	// Cancel the context inline as GetByID returns, so the second ctx.Err()
	// guard (post-validation, pre-executeInTx) fires. This simulates cancellation
	// arriving mid-flight between fetch and tx begin.
	mockRepo.EXPECT().
		GetByID(gomock.Any(), limitID).
		DoAndReturn(func(_ context.Context, _ uuid.UUID) (*model.Limit, error) {
			cancel()
			return existing, nil
		})

	// Tx must never begin, and no WithTx / audit calls are expected.
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().UpdateWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().RecordLimitEventWithTx(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Times(0)

	cmd, cmdErr := NewUpdateLimitCommand(mockRepo, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, cmdErr)

	// hasChanges must evaluate true so the pre-tx guard path is actually reached.
	newName := "updated-limit-name"
	result, err := cmd.Execute(ctx, limitID, &UpdateLimitInput{Name: &newName})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}
