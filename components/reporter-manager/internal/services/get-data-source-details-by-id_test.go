// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace/noop"

	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/redis"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/postgres"
)

func TestUseCase_GetBaseCollectionName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Collection with organization suffix",
			input:    "holders_org123",
			expected: "holders",
		},
		{
			name:     "Collection without underscore",
			input:    "accounts",
			expected: "accounts",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Collection with multiple underscores keeps all but last segment",
			input:    "related_parties_org456",
			expected: "related_parties",
		},
		{
			name:     "Single underscore prefix",
			input:    "holders_",
			expected: "holders",
		},
		{
			name:     "Underscore only",
			input:    "_",
			expected: "",
		},
	}

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := uc.getBaseCollectionName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUseCase_ShouldIncludeFieldForPluginCRM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		fieldName      string
		collectionName string
		expected       bool
	}{
		{
			name:           "Search field is always included",
			fieldName:      "search.document",
			collectionName: "holders",
			expected:       true,
		},
		{
			name:           "Search parent object is included",
			fieldName:      "search",
			collectionName: "aliases",
			expected:       true,
		},
		{
			name:           "Top-level encrypted field document is excluded",
			fieldName:      "document",
			collectionName: "holders",
			expected:       false,
		},
		{
			name:           "Top-level encrypted field name is excluded",
			fieldName:      "name",
			collectionName: "holders",
			expected:       false,
		},
		{
			name:           "Nested encrypted field contact.primary_email is excluded",
			fieldName:      "contact.primary_email",
			collectionName: "holders",
			expected:       false,
		},
		{
			name:           "Nested encrypted field banking_details.account is excluded",
			fieldName:      "banking_details.account",
			collectionName: "aliases",
			expected:       false,
		},
		{
			name:           "Nested encrypted field legal_person.representative.name is excluded",
			fieldName:      "legal_person.representative.name",
			collectionName: "holders",
			expected:       false,
		},
		{
			name:           "Non-encrypted field in holders collection is included",
			fieldName:      "external_id",
			collectionName: "holders",
			expected:       true,
		},
		{
			name:           "Non-encrypted field in aliases collection is included",
			fieldName:      "account_id",
			collectionName: "aliases",
			expected:       true,
		},
		{
			name:           "Non-encrypted field in unknown collection is included",
			fieldName:      "status",
			collectionName: "other_collection",
			expected:       true,
		},
		{
			name:           "Empty field name for holders collection",
			fieldName:      "",
			collectionName: "holders",
			expected:       true,
		},
		{
			name:           "Empty collection name with regular field",
			fieldName:      "some_field",
			collectionName: "",
			expected:       true,
		},
		{
			name:           "Encrypted field excluded regardless of collection",
			fieldName:      "document",
			collectionName: "other_collection",
			expected:       false,
		},
	}

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := uc.shouldIncludeFieldForPluginCRM(tt.fieldName, tt.collectionName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUseCase_GetFieldsForPluginCRM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		schema   mongodb.CollectionSchema
		expected []string
	}{
		{
			name: "Holders collection returns expanded fields",
			schema: mongodb.CollectionSchema{
				CollectionName: "holders_org123",
				Fields: []mongodb.FieldInformation{
					{Name: "field1", DataType: "string"},
				},
			},
			expected: []string{
				"_id",
				"external_id",
				"type",
				"addresses",
				"created_at",
				"updated_at",
				"deleted_at",
				"metadata",
				"search.document",
				"natural_person.favorite_name",
				"natural_person.social_name",
				"natural_person.gender",
				"natural_person.birth_date",
				"natural_person.civil_status",
				"natural_person.nationality",
				"natural_person.status",
				"legal_person.trade_name",
				"legal_person.activity",
				"legal_person.type",
				"legal_person.founding_date",
				"legal_person.size",
				"legal_person.status",
				"legal_person.representative.role",
			},
		},
		{
			name: "Aliases collection returns expanded fields",
			schema: mongodb.CollectionSchema{
				CollectionName: "aliases_org456",
				Fields: []mongodb.FieldInformation{
					{Name: "field1", DataType: "string"},
				},
			},
			expected: []string{
				"_id",
				"account_id",
				"holder_id",
				"ledger_id",
				"type",
				"created_at",
				"updated_at",
				"deleted_at",
				"metadata",
				"search.document",
				"search.banking_details_account",
				"search.banking_details_iban",
				"search.regulatory_fields_participant_document",
				"search.related_party_documents",
				"banking_details.branch",
				"banking_details.type",
				"banking_details.opening_date",
				"banking_details.closing_date",
				"banking_details.country_code",
				"banking_details.bank_id",
				"regulatory_fields",
				"related_parties",
				"related_parties._id",
				"related_parties.role",
				"related_parties.start_date",
				"related_parties.end_date",
			},
		},
		{
			name: "Unknown collection falls back to field filtering",
			schema: mongodb.CollectionSchema{
				CollectionName: "custom_org789",
				Fields: []mongodb.FieldInformation{
					{Name: "status", DataType: "string"},
					{Name: "document", DataType: "string"},
					{Name: "search.document", DataType: "string"},
					{Name: "contact.primary_email", DataType: "string"},
				},
			},
			expected: []string{"status", "search.document"},
		},
		{
			name: "Unknown collection with all fields excluded",
			schema: mongodb.CollectionSchema{
				CollectionName: "sensitive_org000",
				Fields: []mongodb.FieldInformation{
					{Name: "document", DataType: "string"},
					{Name: "name", DataType: "string"},
				},
			},
			expected: []string{},
		},
		{
			name: "Unknown collection with empty fields list",
			schema: mongodb.CollectionSchema{
				CollectionName: "empty_org111",
				Fields:         []mongodb.FieldInformation{},
			},
			expected: []string{},
		},
		{
			name: "Unknown collection with all fields included",
			schema: mongodb.CollectionSchema{
				CollectionName: "plain_org222",
				Fields: []mongodb.FieldInformation{
					{Name: "_id", DataType: "objectId"},
					{Name: "status", DataType: "string"},
					{Name: "created_at", DataType: "date"},
				},
			},
			expected: []string{"_id", "status", "created_at"},
		},
	}

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := uc.getFieldsForPluginCRM(tt.schema)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUseCase_GetExpandedFieldsForPluginCRM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		collectionName string
		expectNil      bool
		expectLen      int
		expectContains []string
	}{
		{
			name:           "Holders collection returns expanded fields",
			collectionName: "holders",
			expectNil:      false,
			expectLen:      23,
			expectContains: []string{
				"_id",
				"external_id",
				"type",
				"search.document",
				"natural_person.favorite_name",
				"legal_person.representative.role",
			},
		},
		{
			name:           "Aliases collection returns expanded fields",
			collectionName: "aliases",
			expectNil:      false,
			expectLen:      26,
			expectContains: []string{
				"_id",
				"account_id",
				"holder_id",
				"search.banking_details_account",
				"banking_details.branch",
				"related_parties.role",
			},
		},
		{
			name:           "Unknown collection returns nil",
			collectionName: "unknown",
			expectNil:      true,
		},
		{
			name:           "Empty collection name returns nil",
			collectionName: "",
			expectNil:      true,
		},
	}

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := uc.getExpandedFieldsForPluginCRM(tt.collectionName)

			if tt.expectNil {
				assert.Nil(t, result)
				return
			}

			assert.NotNil(t, result)
			assert.Len(t, result, tt.expectLen)

			for _, expected := range tt.expectContains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestUseCase_GetDataSourceDetailsByID(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because it modifies global state (immutable registry)
	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"mongo_ds", "pg_ds"})

	t.Cleanup(func() {
		pkg.ResetRegisteredDataSourceIDsForTesting()
	})

	mongoSchema := []mongodb.CollectionSchema{
		{
			CollectionName: "collection1",
			Fields: []mongodb.FieldInformation{
				{Name: "field1", DataType: "string"},
				{Name: "field2", DataType: "int"},
			},
		},
	}
	postgresSchema := []postgres.TableSchema{
		{
			SchemaName: "public",
			TableName:  "table1",
			Columns: []postgres.ColumnInformation{
				{Name: "col1", DataType: "string"},
				{Name: "col2", DataType: "int"},
			},
		},
	}

	cacheKey := constant.DataSourceDetailsKeyPrefix + ":mongo_ds"
	cacheKeyPG := constant.DataSourceDetailsKeyPrefix + ":pg_ds"

	mongoResult := &model.DataSourceDetails{
		Id:           "mongo_ds",
		ExternalName: "mongo_db",
		Type:         pkg.MongoDBType,
		Tables: []model.TableDetails{{
			Name:   "collection1",
			Fields: []string{"field1", "field2"},
		}},
	}
	pgResult := &model.DataSourceDetails{
		Id:           "pg_ds",
		ExternalName: "pg_db",
		Type:         pkg.PostgreSQLType,
		Tables: []model.TableDetails{{
			Name:   "public.table1",
			Fields: []string{"col1", "col2"},
		}},
	}
	mongoResultJSON, _ := json.Marshal(mongoResult)
	pgResultJSON, _ := json.Marshal(pgResult)

	tests := []struct {
		name         string
		setupSvc     func(ctrl *gomock.Controller) *UseCase
		dataSourceID string
		expectErr    bool
		errContains  string
		expectResult *model.DataSourceDetails
	}{
		{
			name:         "Cache hit - MongoDB",
			dataSourceID: "mongo_ds",
			setupSvc: func(ctrl *gomock.Controller) *UseCase {
				mockMongoRepo := mongodb.NewMockRepository(ctrl)
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKey).Return(string(mongoResultJSON), nil)
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"mongo_ds": {
							DatabaseType:      pkg.MongoDBType,
							MongoDBRepository: mockMongoRepo,
							MongoDBName:       "mongo_db",
							Initialized:       true,
						},
					}),
					RedisRepo: mockRedisRepo,
				}
			},
			expectErr:    false,
			expectResult: mongoResult,
		},
		{
			name:         "Cache miss - MongoDB, sets cache",
			dataSourceID: "mongo_ds",
			setupSvc: func(ctrl *gomock.Controller) *UseCase {
				mockMongoRepo := mongodb.NewMockRepository(ctrl)
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKey).Return("", nil)
				mockMongoRepo.EXPECT().GetDatabaseSchema(gomock.Any()).Return(mongoSchema, nil)
				mockRedisRepo.EXPECT().Set(gomock.Any(), cacheKey, string(mongoResultJSON), time.Second*time.Duration(constant.RedisTTL)).Return(nil)
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"mongo_ds": {
							DatabaseType:      pkg.MongoDBType,
							MongoDBRepository: mockMongoRepo,
							MongoDBName:       "mongo_db",
							Initialized:       true,
						},
					}),
					RedisRepo: mockRedisRepo,
				}
			},
			expectErr:    false,
			expectResult: mongoResult,
		},
		{
			name:         "Cache error - MongoDB, acts as miss",
			dataSourceID: "mongo_ds",
			setupSvc: func(ctrl *gomock.Controller) *UseCase {
				mockMongoRepo := mongodb.NewMockRepository(ctrl)
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKey).Return("", errors.New("redis error"))
				mockMongoRepo.EXPECT().GetDatabaseSchema(gomock.Any()).Return(mongoSchema, nil)
				mockRedisRepo.EXPECT().Set(gomock.Any(), cacheKey, string(mongoResultJSON), time.Second*time.Duration(constant.RedisTTL)).Return(nil)
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"mongo_ds": {
							DatabaseType:      pkg.MongoDBType,
							MongoDBRepository: mockMongoRepo,
							MongoDBName:       "mongo_db",
							Initialized:       true,
						},
					}),
					RedisRepo: mockRedisRepo,
				}
			},
			expectErr:    false,
			expectResult: mongoResult,
		},
		{
			name:         "Cache hit - PostgreSQL",
			dataSourceID: "pg_ds",
			setupSvc: func(ctrl *gomock.Controller) *UseCase {
				mockPostgresRepo := postgres.NewMockRepository(ctrl)
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKeyPG).Return(string(pgResultJSON), nil)
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"pg_ds": {
							DatabaseType:       pkg.PostgreSQLType,
							PostgresRepository: mockPostgresRepo,
							DatabaseConfig:     &postgres.Connection{Connected: true, DBName: "pg_db"},
							MongoDBName:        "pg_db",
							Initialized:        true,
						},
					}),
					RedisRepo: mockRedisRepo,
				}
			},
			expectErr:    false,
			expectResult: pgResult,
		},
		{
			name:         "Cache miss - PostgreSQL, sets cache",
			dataSourceID: "pg_ds",
			setupSvc: func(ctrl *gomock.Controller) *UseCase {
				mockPostgresRepo := postgres.NewMockRepository(ctrl)
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKeyPG).Return("", nil)
				mockPostgresRepo.EXPECT().GetDatabaseSchema(gomock.Any(), gomock.Any()).Return(postgresSchema, nil)
				mockRedisRepo.EXPECT().Set(gomock.Any(), cacheKeyPG, string(pgResultJSON), time.Second*time.Duration(constant.RedisTTL)).Return(nil)
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"pg_ds": {
							DatabaseType:       pkg.PostgreSQLType,
							PostgresRepository: mockPostgresRepo,
							DatabaseConfig:     &postgres.Connection{Connected: true, DBName: "pg_db"},
							MongoDBName:        "pg_db",
							Initialized:        true,
						},
					}),
					RedisRepo: mockRedisRepo,
				}
			},
			expectErr:    false,
			expectResult: pgResult,
		},
		{
			name:         "Error - Data source not found",
			dataSourceID: "not_found",
			setupSvc: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockRedisRepo.EXPECT().Get(gomock.Any(), constant.DataSourceDetailsKeyPrefix+":not_found").Return("", nil)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}), RedisRepo: mockRedisRepo}
			},
			expectErr:    true,
			errContains:  constant.ErrMissingDataSource.Error(),
			expectResult: nil,
		},
		{
			name:         "Error - MongoDB repo returns error",
			dataSourceID: "mongo_ds",
			setupSvc: func(ctrl *gomock.Controller) *UseCase {
				mockMongoRepo := mongodb.NewMockRepository(ctrl)
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKey).Return("", nil)
				mockMongoRepo.EXPECT().GetDatabaseSchema(gomock.Any()).Return(nil, errors.New("db error"))
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"mongo_ds": {
							DatabaseType:      pkg.MongoDBType,
							MongoDBRepository: mockMongoRepo,
							MongoDBName:       "mongo_db",
							Initialized:       true,
						},
					}),
					RedisRepo: mockRedisRepo,
				}
			},
			expectErr:    true,
			errContains:  constant.ErrMissingDataSource.Error(),
			expectResult: nil,
		},
		{
			name:         "Error - PostgreSQL repo returns error",
			dataSourceID: "pg_ds",
			setupSvc: func(ctrl *gomock.Controller) *UseCase {
				mockPostgresRepo := postgres.NewMockRepository(ctrl)
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKeyPG).Return("", nil)
				mockPostgresRepo.EXPECT().GetDatabaseSchema(gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))
				return &UseCase{
					Logger: log.NewNop(),
					Tracer: noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"pg_ds": {
							DatabaseType:       pkg.PostgreSQLType,
							PostgresRepository: mockPostgresRepo,
							DatabaseConfig:     &postgres.Connection{Connected: true, DBName: "pg_db"},
							MongoDBName:        "pg_db",
							Initialized:        true,
						},
					}),
					RedisRepo: mockRedisRepo,
				}
			},
			expectErr:    true,
			errContains:  constant.ErrMissingDataSource.Error(),
			expectResult: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := tt.setupSvc(ctrl)

			ctx := context.Background()
			result, err := svc.GetDataSourceDetailsByID(ctx, tt.dataSourceID)
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectResult, result)
			}
		})
	}
}

func TestUseCase_GetDataSourceDetailsByID_DefaultType(t *testing.T) {
	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"unknown_ds"})
	t.Cleanup(func() { pkg.ResetRegisteredDataSourceIDsForTesting() })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	cacheKey := constant.DataSourceDetailsKeyPrefix + ":unknown_ds"

	mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKey).Return("", nil)

	svc := &UseCase{
		Logger: log.NewNop(),
		Tracer: noop.NewTracerProvider().Tracer("test"),
		ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
			"unknown_ds": {
				DatabaseType: "unsupported_type",
				Initialized:  true,
			},
		}),
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	result, err := svc.GetDataSourceDetailsByID(ctx, "unknown_ds")

	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrMissingDataSource.Error())
	assert.Nil(t, result)
}

func TestUseCase_GetDataSourceDetailsByID_CacheSetError(t *testing.T) {
	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"mongo_ds"})
	t.Cleanup(func() { pkg.ResetRegisteredDataSourceIDsForTesting() })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockMongoRepo := mongodb.NewMockRepository(ctrl)

	cacheKey := constant.DataSourceDetailsKeyPrefix + ":mongo_ds"

	mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKey).Return("", nil)
	mockMongoRepo.EXPECT().GetDatabaseSchema(gomock.Any()).Return([]mongodb.CollectionSchema{
		{
			CollectionName: "collection1",
			Fields: []mongodb.FieldInformation{
				{Name: "field1", DataType: "string"},
			},
		},
	}, nil)
	mockRedisRepo.EXPECT().Set(gomock.Any(), cacheKey, gomock.Any(), gomock.Any()).Return(errors.New("redis write error"))

	svc := &UseCase{
		Logger: log.NewNop(),
		Tracer: noop.NewTracerProvider().Tracer("test"),
		ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
			"mongo_ds": {
				DatabaseType:      pkg.MongoDBType,
				MongoDBRepository: mockMongoRepo,
				MongoDBName:       "mongo_db",
				Initialized:       true,
			},
		}),
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	result, err := svc.GetDataSourceDetailsByID(ctx, "mongo_ds")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis write error")
	assert.Nil(t, result)
}

func TestUseCase_GetDataSourceDetailsByID_NilRedisRepo(t *testing.T) {
	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"mongo_ds"})
	t.Cleanup(func() { pkg.ResetRegisteredDataSourceIDsForTesting() })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMongoRepo := mongodb.NewMockRepository(ctrl)

	mockMongoRepo.EXPECT().GetDatabaseSchema(gomock.Any()).Return([]mongodb.CollectionSchema{
		{
			CollectionName: "collection1",
			Fields: []mongodb.FieldInformation{
				{Name: "field1", DataType: "string"},
			},
		},
	}, nil)

	svc := &UseCase{
		Logger: log.NewNop(),
		Tracer: noop.NewTracerProvider().Tracer("test"),
		ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
			"mongo_ds": {
				DatabaseType:      pkg.MongoDBType,
				MongoDBRepository: mockMongoRepo,
				MongoDBName:       "mongo_db",
				Initialized:       true,
			},
		}),
		RedisRepo: nil,
	}

	ctx := context.Background()
	result, err := svc.GetDataSourceDetailsByID(ctx, "mongo_ds")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "mongo_ds", result.Id)
}

func TestUseCase_GetDisplayNameForCollection(t *testing.T) {
	t.Parallel()

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	tests := []struct {
		name           string
		collectionName string
		dataSourceID   string
		expected       string
	}{
		{
			name:           "plugin_crm strips org suffix",
			collectionName: "holders_org123",
			dataSourceID:   "plugin_crm",
			expected:       "holders",
		},
		{
			name:           "non-plugin_crm returns full name",
			collectionName: "holders_org123",
			dataSourceID:   "midaz_organization",
			expected:       "holders_org123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := uc.getDisplayNameForCollection(tt.collectionName, tt.dataSourceID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUseCase_GetFieldsForCollection_NonPluginCRM(t *testing.T) {
	t.Parallel()

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	collection := mongodb.CollectionSchema{
		CollectionName: "users",
		Fields: []mongodb.FieldInformation{
			{Name: "id", DataType: "string"},
			{Name: "name", DataType: "string"},
			{Name: "email", DataType: "string"},
		},
	}

	fields := uc.getFieldsForCollection(collection, "midaz_organization")

	assert.Equal(t, []string{"id", "name", "email"}, fields)
}

func TestUseCase_GetDataSourceDetailsFromCache_UnmarshalError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().Get(gomock.Any(), "test-key").Return("{invalid-json", nil)

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	result, ok := uc.getDataSourceDetailsFromCache(ctx, "test-key")

	assert.False(t, ok)
	assert.Nil(t, result)
}

func TestUseCase_SetDataSourceDetailsToCache_NilDetails(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: nil,
	}

	ctx := context.Background()
	err := uc.setDataSourceDetailsToCache(ctx, "test-key", nil)

	require.NoError(t, err)
}

func TestUseCase_GetDataSourceDetailsByID_UnregisteredDatasource(t *testing.T) {
	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"registered_ds"})
	t.Cleanup(func() { pkg.ResetRegisteredDataSourceIDsForTesting() })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	cacheKey := constant.DataSourceDetailsKeyPrefix + ":unregistered_ds"
	mockRedisRepo.EXPECT().Get(gomock.Any(), cacheKey).Return("", nil)

	svc := &UseCase{
		Logger: log.NewNop(),
		Tracer: noop.NewTracerProvider().Tracer("test"),
		ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
			"unregistered_ds": {
				DatabaseType: pkg.MongoDBType,
				MongoDBName:  "some_db",
				Initialized:  true,
			},
		}),
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	result, err := svc.GetDataSourceDetailsByID(ctx, "unregistered_ds")

	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrMissingDataSource.Error())
	assert.Nil(t, result)
}
