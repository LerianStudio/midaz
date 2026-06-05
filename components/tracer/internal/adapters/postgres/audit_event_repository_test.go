// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// setupAuditEventRepositoryMockDB creates a gomock controller, mock DBConnection, and sqlmock for testing.
// Returns the repository, sqlmock for query expectations, and a cleanup function.
func setupAuditEventRepositoryMockDB(t *testing.T) (*AuditEventRepository, sqlmock.Sqlmock, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(db, nil).AnyTimes()

	repo := NewAuditEventRepositoryWithConnection(mockConn)

	cleanup := func() {
		sqlMock.ExpectClose()
		err := db.Close()
		require.NoError(t, err)
	}

	return repo, sqlMock, cleanup
}

// createTestAuditEvent creates a test audit event with default values.
func createTestAuditEvent(t *testing.T) *model.AuditEvent {
	t.Helper()

	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	ruleID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")

	return &model.AuditEvent{
		ID:           1,
		Hash:         "abc123hash",
		PreviousHash: "xyz789prev",
		EventID:      uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
		EventType:    model.AuditEventTransactionValidated,
		CreatedAt:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		Action:       model.AuditActionValidate,
		Result:       model.AuditResultAllow,
		ResourceID:   "txn-12345",
		ResourceType: model.ResourceTypeTransaction,
		Actor: model.Actor{
			ActorType: model.ActorTypeSystem,
			ID:        "system-001",
			Name:      "Validation Engine",
			Role:      "validator",
			IPAddress: "10.0.0.1",
		},
		Context: map[string]any{
			"request": map[string]any{
				"account": map[string]any{
					"id":          accountID.String(),
					"segmentId":   "seg-001",
					"portfolioId": "port-001",
				},
				"transactionType": "PIX",
				"amount":          100,
			},
			"response": map[string]any{
				"decision":         "ALLOW",
				"reason":           "Transaction approved",
				"matchedRuleIds":   []string{ruleID.String()},
				"evaluatedRuleIds": []string{ruleID.String()},
				"processingTimeMs": 45,
			},
		},
		Metadata: map[string]any{
			"correlationId": "corr-123",
			"ticketId":      "ticket-456",
		},
	}
}

// auditEventColumns returns the column names for audit event queries.
func auditEventColumns() []string {
	return []string{
		"id", "hash", "previous_hash",
		"event_id", "event_type", "created_at", "action", "result",
		"resource_id", "resource_type",
		"actor_type", "actor_id", "actor_name", "actor_role", "actor_ip_address",
		"context", "metadata",
	}
}

// auditEventRow creates a sqlmock row from an audit event.
func auditEventRow(t *testing.T, event *model.AuditEvent) *sqlmock.Rows {
	t.Helper()

	contextJSON, err := json.Marshal(event.Context)
	require.NoError(t, err, "failed to marshal context")

	metadataJSON, err := json.Marshal(event.Metadata)
	require.NoError(t, err, "failed to marshal metadata")

	var previousHash any
	if event.PreviousHash != "" {
		previousHash = event.PreviousHash
	}

	var actorRole any
	if event.Actor.Role != "" {
		actorRole = event.Actor.Role
	}

	return sqlmock.NewRows(auditEventColumns()).
		AddRow(
			event.ID,
			event.Hash,
			previousHash,
			event.EventID,
			string(event.EventType),
			event.CreatedAt,
			string(event.Action),
			string(event.Result),
			event.ResourceID,
			string(event.ResourceType),
			string(event.Actor.ActorType),
			event.Actor.ID,
			event.Actor.Name,
			actorRole,
			event.Actor.IPAddress,
			contextJSON,
			metadataJSON,
		)
}

func TestAuditEventRepository_Insert_ConnectionError(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused"))

	repo := NewAuditEventRepositoryWithConnection(mockConn)

	ctx := context.Background()
	err := repo.Insert(ctx, createTestAuditEvent(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection")
}

func TestAuditEventRepository_Insert(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		event     *model.AuditEvent
		mockSetup func(mock sqlmock.Sqlmock, event *model.AuditEvent)
		wantErr   bool
		errMsg    string
	}{
		{
			name:  "Success - inserts audit event with all fields",
			event: createTestAuditEvent(t),
			mockSetup: func(mock sqlmock.Sqlmock, event *model.AuditEvent) {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO audit_events`)).
					WithArgs(
						event.EventID,
						string(event.EventType),
						event.CreatedAt,
						string(event.Action),
						string(event.Result),
						event.ResourceID,
						string(event.ResourceType),
						string(event.Actor.ActorType),
						event.Actor.ID,
						event.Actor.Name,
						event.Actor.Role,
						event.Actor.IPAddress,
						sqlmock.AnyArg(),           // contextJSON ($13)
						sqlmock.AnyArg(),           // metadataJSON ($14)
						event.ResourceID,           // $15: WHERE resource_id
						string(event.EventType),    // $16: WHERE event_type
						string(event.ResourceType), // $17: WHERE resource_type
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr: false,
		},
		{
			name: "Success - inserts event with nil actor role",
			event: func() *model.AuditEvent {
				e := createTestAuditEvent(t)
				e.Actor.Role = ""
				return e
			}(),
			mockSetup: func(mock sqlmock.Sqlmock, event *model.AuditEvent) {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO audit_events`)).
					WithArgs(
						event.EventID,
						string(event.EventType),
						event.CreatedAt,
						string(event.Action),
						string(event.Result),
						event.ResourceID,
						string(event.ResourceType),
						string(event.Actor.ActorType),
						event.Actor.ID,
						event.Actor.Name,
						nil, // actor role is nil
						event.Actor.IPAddress,
						sqlmock.AnyArg(),           // contextJSON ($13)
						sqlmock.AnyArg(),           // metadataJSON ($14)
						event.ResourceID,           // $15: WHERE resource_id
						string(event.EventType),    // $16: WHERE event_type
						string(event.ResourceType), // $17: WHERE resource_type
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr: false,
		},
		{
			name: "Success - inserts CRUD event",
			event: func() *model.AuditEvent {
				e := createTestAuditEvent(t)
				e.EventType = model.AuditEventRuleCreated
				e.Action = model.AuditActionCreate
				e.Result = model.AuditResultSuccess
				e.ResourceType = model.ResourceTypeRule
				e.Context = map[string]any{
					"after": map[string]any{
						"name":       "New Rule",
						"expression": "amount > 1000",
					},
					"reason": "Rule created by admin",
				}
				return e
			}(),
			mockSetup: func(mock sqlmock.Sqlmock, event *model.AuditEvent) {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO audit_events`)).
					WithArgs(
						event.EventID,
						string(event.EventType),
						event.CreatedAt,
						string(event.Action),
						string(event.Result),
						event.ResourceID,
						string(event.ResourceType),
						string(event.Actor.ActorType),
						event.Actor.ID,
						event.Actor.Name,
						event.Actor.Role,
						event.Actor.IPAddress,
						sqlmock.AnyArg(),           // contextJSON ($13)
						sqlmock.AnyArg(),           // metadataJSON ($14)
						event.ResourceID,           // $15: WHERE resource_id
						string(event.EventType),    // $16: WHERE event_type
						string(event.ResourceType), // $17: WHERE resource_type
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr: false,
		},
		{
			name:  "Error - nil event",
			event: nil,
			mockSetup: func(mock sqlmock.Sqlmock, event *model.AuditEvent) {
				// No query expected
			},
			wantErr: true,
			errMsg:  "event cannot be nil",
		},
		{
			name:  "Error - database insert fails",
			event: createTestAuditEvent(t),
			mockSetup: func(mock sqlmock.Sqlmock, event *model.AuditEvent) {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO audit_events`)).
					WillReturnError(errors.New("unique constraint violation"))
			},
			wantErr: true,
			errMsg:  "failed to insert audit event",
		},
		// Deduplication Tests
		{
			name:  "Success - duplicate transaction validation silently ignored (dedup: RowsAffected=0)",
			event: createTestAuditEvent(t), // transaction validation event
			mockSetup: func(mock sqlmock.Sqlmock, event *model.AuditEvent) {
				// WHERE NOT EXISTS returns 0 rows affected when duplicate exists
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO audit_events`)).
					WithArgs(
						event.EventID,
						string(event.EventType),
						event.CreatedAt,
						string(event.Action),
						string(event.Result),
						event.ResourceID,
						string(event.ResourceType),
						string(event.Actor.ActorType),
						event.Actor.ID,
						event.Actor.Name,
						event.Actor.Role,
						event.Actor.IPAddress,
						sqlmock.AnyArg(),           // contextJSON ($13)
						sqlmock.AnyArg(),           // metadataJSON ($14)
						event.ResourceID,           // $15: WHERE resource_id
						string(event.EventType),    // $16: WHERE event_type
						string(event.ResourceType), // $17: WHERE resource_type
					).
					WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected = duplicate ignored
			},
			wantErr: false,
		},
		{
			name: "Success - non-transaction resource always inserts (no dedup constraint)",
			event: func() *model.AuditEvent {
				e := createTestAuditEvent(t)
				e.EventType = model.AuditEventLimitCreated
				e.Action = model.AuditActionCreate
				e.Result = model.AuditResultSuccess
				e.ResourceType = model.ResourceTypeLimit // Non-transaction type
				e.ResourceID = "limit-12345"
				return e
			}(),
			mockSetup: func(mock sqlmock.Sqlmock, event *model.AuditEvent) {
				// For non-transaction resources, $17 != 'transaction' so
				// WHERE NOT EXISTS is always true and INSERT proceeds
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO audit_events`)).
					WithArgs(
						event.EventID,
						string(event.EventType),
						event.CreatedAt,
						string(event.Action),
						string(event.Result),
						event.ResourceID,
						string(event.ResourceType),
						string(event.Actor.ActorType),
						event.Actor.ID,
						event.Actor.Name,
						event.Actor.Role,
						event.Actor.IPAddress,
						sqlmock.AnyArg(),           // contextJSON ($13)
						sqlmock.AnyArg(),           // metadataJSON ($14)
						event.ResourceID,           // $15: WHERE resource_id
						string(event.EventType),    // $16: WHERE event_type
						string(event.ResourceType), // $17: 'limit' != 'transaction'
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupAuditEventRepositoryMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock, tt.event)

			ctx := context.Background()
			err := repo.Insert(ctx, tt.event)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestAuditEventRepository_GetByID_ConnectionError(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused"))

	repo := NewAuditEventRepositoryWithConnection(mockConn)

	ctx := context.Background()
	result, err := repo.GetByID(ctx, testutil.MustDeterministicUUID(999))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection")
	assert.Nil(t, result)
}

func TestAuditEventRepository_GetByID(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		eventID   uuid.UUID
		mockSetup func(mock sqlmock.Sqlmock)
		want      *model.AuditEvent
		wantErr   bool
		errType   error
	}{
		{
			name:    "Success - finds audit event",
			eventID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")).
					WillReturnRows(auditEventRow(t, event))
			},
			want:    createTestAuditEvent(t),
			wantErr: false,
		},
		{
			name:    "Error - audit event not found",
			eventID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440099"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440099")).
					WillReturnError(sql.ErrNoRows)
			},
			wantErr: true,
			errType: constant.ErrAuditEventNotFound,
		},
		{
			name:    "Error - database query fails",
			eventID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")).
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupAuditEventRepositoryMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			result, err := repo.GetByID(ctx, tt.eventID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.want.EventID, result.EventID)
				assert.Equal(t, tt.want.EventType, result.EventType)
				assert.Equal(t, tt.want.Action, result.Action)
				assert.Equal(t, tt.want.Result, result.Result)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestAuditEventRepository_List_ConnectionError(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused"))

	repo := NewAuditEventRepositoryWithConnection(mockConn)

	ctx := context.Background()
	filters := &model.AuditEventFilters{Limit: 10}
	result, err := repo.List(ctx, filters)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection")
	assert.Nil(t, result)
}

func TestAuditEventRepository_List(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		filters   *model.AuditEventFilters
		mockSetup func(mock sqlmock.Sqlmock)
		wantLen   int
		wantMore  bool
		wantErr   bool
		errMsg    string
	}{
		{
			name: "Success - finds all events with pagination",
			filters: &model.AuditEventFilters{
				Limit:     10,
				SortBy:    "created_at",
				SortOrder: "DESC",
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(auditEventRow(t, event))
			},
			wantLen:  1,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - filters by event type",
			filters: func() *model.AuditEventFilters {
				eventType := model.AuditEventTransactionValidated
				return &model.AuditEventFilters{
					EventType: &eventType,
					Limit:     10,
				}
			}(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(auditEventRow(t, event))
			},
			wantLen:  1,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - filters by action",
			filters: func() *model.AuditEventFilters {
				action := model.AuditActionValidate
				return &model.AuditEventFilters{
					Action: &action,
					Limit:  10,
				}
			}(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(auditEventRow(t, event))
			},
			wantLen:  1,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - filters by result",
			filters: func() *model.AuditEventFilters {
				result := model.AuditResultAllow
				return &model.AuditEventFilters{
					Result: &result,
					Limit:  10,
				}
			}(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(auditEventRow(t, event))
			},
			wantLen:  1,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - filters by resource type and ID",
			filters: func() *model.AuditEventFilters {
				resourceType := model.ResourceTypeTransaction
				resourceID := "txn-12345"
				return &model.AuditEventFilters{
					ResourceType: &resourceType,
					ResourceID:   &resourceID,
					Limit:        10,
				}
			}(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(auditEventRow(t, event))
			},
			wantLen:  1,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - filters by actor type and ID",
			filters: func() *model.AuditEventFilters {
				actorType := model.ActorTypeSystem
				actorID := "system-001"
				return &model.AuditEventFilters{
					ActorType: &actorType,
					ActorID:   &actorID,
					Limit:     10,
				}
			}(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(auditEventRow(t, event))
			},
			wantLen:  1,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - filters by date range",
			filters: &model.AuditEventFilters{
				StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				EndDate:   time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC),
				Limit:     10,
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(auditEventRow(t, event))
			},
			wantLen:  1,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - filters by JSONB account ID",
			filters: func() *model.AuditEventFilters {
				accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
				return &model.AuditEventFilters{
					AccountID: &accountID,
					Limit:     10,
				}
			}(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(auditEventRow(t, event))
			},
			wantLen:  1,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - filters by JSONB transaction type",
			filters: func() *model.AuditEventFilters {
				txnType := model.TransactionTypePix
				return &model.AuditEventFilters{
					TransactionType: &txnType,
					Limit:           10,
				}
			}(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(auditEventRow(t, event))
			},
			wantLen:  1,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - filters by matched rule ID",
			filters: func() *model.AuditEventFilters {
				ruleID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
				return &model.AuditEventFilters{
					MatchedRuleID: &ruleID,
					Limit:         10,
				}
			}(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				event := createTestAuditEvent(t)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(auditEventRow(t, event))
			},
			wantLen:  1,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - empty results",
			filters: &model.AuditEventFilters{
				Limit: 10,
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(sqlmock.NewRows(auditEventColumns()))
			},
			wantLen:  0,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - has more pages",
			filters: &model.AuditEventFilters{
				Limit: 1,
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				event1 := createTestAuditEvent(t)
				event2 := createTestAuditEvent(t)
				event2.EventID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440099")

				contextJSON, _ := json.Marshal(event1.Context)
				metadataJSON, _ := json.Marshal(event1.Metadata)

				rows := sqlmock.NewRows(auditEventColumns()).
					AddRow(
						event1.ID, event1.Hash, event1.PreviousHash,
						event1.EventID, string(event1.EventType), event1.CreatedAt,
						string(event1.Action), string(event1.Result),
						event1.ResourceID, string(event1.ResourceType),
						string(event1.Actor.ActorType), event1.Actor.ID, event1.Actor.Name,
						event1.Actor.Role, event1.Actor.IPAddress,
						contextJSON, metadataJSON,
					).
					AddRow(
						event2.ID, event2.Hash, event2.PreviousHash,
						event2.EventID, string(event2.EventType), event2.CreatedAt,
						string(event2.Action), string(event2.Result),
						event2.ResourceID, string(event2.ResourceType),
						string(event2.Actor.ActorType), event2.Actor.ID, event2.Actor.Name,
						event2.Actor.Role, event2.Actor.IPAddress,
						contextJSON, metadataJSON,
					)

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(rows)
			},
			wantLen:  1,
			wantMore: true,
			wantErr:  false,
		},
		{
			name:    "Success - nil filters uses defaults",
			filters: nil,
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(sqlmock.NewRows(auditEventColumns()))
			},
			wantLen:  0,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Error - invalid filters",
			filters: &model.AuditEventFilters{
				Limit:     -1,
				SortOrder: "INVALID",
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				// No query expected
			},
			wantErr: true,
			errMsg:  "TRC-0141",
		},
		{
			name: "Success - limit zero uses default",
			filters: &model.AuditEventFilters{
				Limit: 0,
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(sqlmock.NewRows(auditEventColumns()))
			},
			wantLen:  0,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Success - limit at maximum boundary",
			filters: &model.AuditEventFilters{
				Limit: model.MaxAuditEventFilterLimit,
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnRows(sqlmock.NewRows(auditEventColumns()))
			},
			wantLen:  0,
			wantMore: false,
			wantErr:  false,
		},
		{
			name: "Error - limit exceeds maximum boundary",
			filters: &model.AuditEventFilters{
				Limit: model.MaxAuditEventFilterLimit + 1,
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				// No query expected
			},
			wantErr: true,
			errMsg:  "TRC-0141",
		},
		{
			name: "Error - database query fails",
			filters: &model.AuditEventFilters{
				Limit: 10,
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
					WillReturnError(errors.New("query timeout"))
			},
			wantErr: true,
			errMsg:  "failed to list audit events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupAuditEventRepositoryMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			result, err := repo.List(ctx, tt.filters)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Len(t, result.AuditEvents, tt.wantLen)
				assert.Equal(t, tt.wantMore, result.HasMore)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestAuditEventRepository_VerifyHashChain_ConnectionError(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(nil, errors.New("connection refused"))

	repo := NewAuditEventRepositoryWithConnection(mockConn)

	ctx := context.Background()
	result, err := repo.VerifyHashChain(ctx, testutil.MustDeterministicUUID(998))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get database connection")
	assert.Nil(t, result)
}

func TestAuditEventRepository_VerifyHashChain(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		eventID   uuid.UUID
		mockSetup func(mock sqlmock.Sqlmock)
		want      *model.HashChainVerificationResult
		wantErr   bool
		errType   error
	}{
		{
			name:    "Success - valid hash chain",
			eventID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// First query: get internal ID
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM audit_events WHERE event_id = $1`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(100)))

				// Second query: verify hash chain function
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT is_valid, first_invalid_id, total_checked, error_detail FROM verify_audit_hash_chain(1, $1)`)).
					WithArgs(int64(100)).
					WillReturnRows(
						sqlmock.NewRows([]string{"is_valid", "first_invalid_id", "total_checked", "error_detail"}).
							AddRow(true, nil, int64(100), nil),
					)
			},
			want: &model.HashChainVerificationResult{
				IsValid:      true,
				TotalChecked: 100,
				Message:      "Hash chain integrity verified successfully",
			},
			wantErr: false,
		},
		{
			name:    "Success - compromised hash chain",
			eventID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				// First query: get internal ID
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM audit_events WHERE event_id = $1`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(100)))

				// Second query: verify hash chain function returns invalid
				invalidID := int64(50)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT is_valid, first_invalid_id, total_checked, error_detail FROM verify_audit_hash_chain(1, $1)`)).
					WithArgs(int64(100)).
					WillReturnRows(
						sqlmock.NewRows([]string{"is_valid", "first_invalid_id", "total_checked", "error_detail"}).
							AddRow(false, invalidID, int64(100), "Hash mismatch detected"),
					)
			},
			want: &model.HashChainVerificationResult{
				IsValid:        false,
				FirstInvalidID: testutil.Int64Ptr(50),
				TotalChecked:   100,
				Message:        "Hash chain integrity compromised - possible data tampering detected",
			},
			wantErr: false,
		},
		{
			name:    "Error - event not found",
			eventID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440099"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM audit_events WHERE event_id = $1`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440099")).
					WillReturnError(sql.ErrNoRows)
			},
			wantErr: true,
			errType: constant.ErrAuditEventNotFound,
		},
		{
			name:    "Error - failed to get internal ID",
			eventID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM audit_events WHERE event_id = $1`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")).
					WillReturnError(errors.New("database error"))
			},
			wantErr: true,
		},
		{
			name:    "Error - verification function fails",
			eventID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM audit_events WHERE event_id = $1`)).
					WithArgs(uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(100)))

				mock.ExpectQuery(regexp.QuoteMeta(`SELECT is_valid, first_invalid_id, total_checked, error_detail FROM verify_audit_hash_chain(1, $1)`)).
					WithArgs(int64(100)).
					WillReturnError(errors.New("function execution error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupAuditEventRepositoryMockDB(t)
			defer cleanup()

			tt.mockSetup(sqlMock)

			ctx := context.Background()
			result, err := repo.VerifyHashChain(ctx, tt.eventID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.want.IsValid, result.IsValid)
				assert.Equal(t, tt.want.TotalChecked, result.TotalChecked)
				assert.Equal(t, tt.want.Message, result.Message)
				if tt.want.FirstInvalidID != nil {
					require.NotNil(t, result.FirstInvalidID)
					assert.Equal(t, *tt.want.FirstInvalidID, *result.FirstInvalidID)
				} else {
					assert.Nil(t, result.FirstInvalidID)
				}
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestAuditEventRepository_List_SortFields(t *testing.T) {
	testutil.SetupTestTracing(t)

	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		wantErr   bool
	}{
		{
			name:      "Success - sort by created_at DESC",
			sortBy:    "created_at",
			sortOrder: "DESC",
			wantErr:   false,
		},
		{
			name:      "Success - sort by created_at ASC",
			sortBy:    "created_at",
			sortOrder: "ASC",
			wantErr:   false,
		},
		{
			name:      "Success - sort by event_type DESC",
			sortBy:    "event_type",
			sortOrder: "DESC",
			wantErr:   false,
		},
		{
			name:      "Success - sort by event_type ASC",
			sortBy:    "event_type",
			sortOrder: "ASC",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, sqlMock, cleanup := setupAuditEventRepositoryMockDB(t)
			defer cleanup()

			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
				WillReturnRows(sqlmock.NewRows(auditEventColumns()))

			ctx := context.Background()
			filters := &model.AuditEventFilters{
				Limit:     10,
				SortBy:    tt.sortBy,
				SortOrder: tt.sortOrder,
			}

			result, err := repo.List(ctx, filters)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}

			require.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestAuditMapSortField(t *testing.T) {
	r := &AuditEventRepository{}

	tests := []struct {
		name     string
		sortBy   string
		expected string
	}{
		{
			name:     "created_at maps to created_at column",
			sortBy:   "created_at",
			expected: "created_at",
		},
		{
			name:     "event_type maps to event_type column",
			sortBy:   "event_type",
			expected: "event_type",
		},
		{
			name:     "unknown field falls back to empty string",
			sortBy:   "unknown_field",
			expected: "",
		},
		{
			name:     "camelCase createdAt is rejected and falls back to default",
			sortBy:   "createdAt",
			expected: "",
		},
		{
			name:     "camelCase eventType is rejected and falls back to default",
			sortBy:   "eventType",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.mapSortField(tt.sortBy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAuditExtractSortValue(t *testing.T) {
	r := &AuditEventRepository{}

	fixedTime := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	event := &model.AuditEvent{
		CreatedAt: fixedTime,
		EventType: model.AuditEventType("VALIDATION"),
	}

	tests := []struct {
		name     string
		sortBy   string
		expected string
	}{
		{
			name:     "snake_case created_at extracts timestamp",
			sortBy:   "created_at",
			expected: fixedTime.Format(time.RFC3339Nano),
		},
		{
			name:     "snake_case event_type extracts event type",
			sortBy:   "event_type",
			expected: "VALIDATION",
		},
		{
			name:     "camelCase createdAt should fall back to default (empty string)",
			sortBy:   "createdAt",
			expected: "",
		},
		{
			name:     "camelCase eventType should NOT extract event type after migration",
			sortBy:   "eventType",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.extractSortValue(event, tt.sortBy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAuditEventRepository_scanEvent_JSONUnmarshalError(t *testing.T) {
	testutil.SetupTestTracing(t)

	repo, sqlMock, cleanup := setupAuditEventRepositoryMockDB(t)
	defer cleanup()

	// Create invalid JSON to trigger unmarshal error
	invalidJSON := []byte(`{invalid json`)

	rows := sqlmock.NewRows(auditEventColumns()).
		AddRow(
			int64(1),
			"hash123",
			"prevhash",
			testutil.MustDeterministicUUID(997),
			"TRANSACTION_VALIDATED",
			testutil.FixedTime(),
			"VALIDATE",
			"ALLOW",
			"txn-123",
			"transaction",
			"system",
			"sys-001",
			"System",
			"admin",
			"10.0.0.1",
			invalidJSON, // Invalid context JSON
			[]byte(`{}`),
		)

	sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(rows)

	ctx := context.Background()
	result, err := repo.List(ctx, &model.AuditEventFilters{Limit: 10})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan audit event")
	assert.Nil(t, result)

	require.NoError(t, sqlMock.ExpectationsWereMet())
}
