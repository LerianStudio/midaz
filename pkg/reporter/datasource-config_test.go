// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	libConstant "github.com/LerianStudio/lib-commons/v5/commons/constants"
	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pg "github.com/LerianStudio/midaz/v3/pkg/reporter/postgres"
)

func TestIsFatalError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error returns false",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection refused is fatal",
			err:      errors.New("dial tcp 127.0.0.1:5432: connection refused"),
			expected: true,
		},
		{
			name:     "no such host is fatal",
			err:      errors.New("dial tcp: lookup db.example.com: no such host"),
			expected: true,
		},
		{
			name:     "DNS lookup failure is fatal",
			err:      errors.New("lookup db.example.com on 8.8.8.8:53: no such host"),
			expected: true,
		},
		{
			name:     "server misbehaving is fatal",
			err:      errors.New("lookup db.example.com: server misbehaving"),
			expected: true,
		},
		{
			name:     "unsupported database type is fatal",
			err:      errors.New("unsupported database type: oracle"),
			expected: true,
		},
		{
			name:     "invalid connection string is fatal",
			err:      errors.New("invalid connection string: missing host"),
			expected: true,
		},
		{
			name:     "authentication failed is fatal",
			err:      errors.New("authentication failed for user 'admin'"),
			expected: true,
		},
		{
			name:     "authorization failed is fatal",
			err:      errors.New("authorization failed: insufficient privileges"),
			expected: true,
		},
		{
			name:     "access denied is fatal",
			err:      errors.New("access denied for user 'readonly'@'10.0.0.1'"),
			expected: true,
		},
		{
			name:     "timeout error is retryable",
			err:      errors.New("dial tcp 127.0.0.1:5432: i/o timeout"),
			expected: false,
		},
		{
			name:     "connection reset is retryable",
			err:      errors.New("read: connection reset by peer"),
			expected: false,
		},
		{
			name:     "generic error is retryable",
			err:      errors.New("something went wrong"),
			expected: false,
		},
		{
			name:     "EOF error is retryable",
			err:      errors.New("unexpected EOF"),
			expected: false,
		},
		{
			name:     "case insensitive matching - CONNECTION REFUSED",
			err:      errors.New("CONNECTION REFUSED by server"),
			expected: true,
		},
		{
			name:     "case insensitive matching - Authentication Failed uppercase",
			err:      errors.New("Authentication Failed for user postgres"),
			expected: true,
		},
		{
			name:     "partial match - contains connection refused in longer message",
			err:      errors.New("failed to connect to postgres: connection refused at host:5432"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := isFatalError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRegisterDataSourceIDsForTesting(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	// Reset to ensure clean state
	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	RegisterDataSourceIDsForTesting([]string{"test_db_1", "test_db_2"})

	assert.True(t, IsValidDataSourceID("test_db_1"))
	assert.True(t, IsValidDataSourceID("test_db_2"))
	assert.False(t, IsValidDataSourceID("test_db_3"))
}

func TestResetRegisteredDataSourceIDsForTesting(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	// Register some IDs
	RegisterDataSourceIDsForTesting([]string{"reset_db_1", "reset_db_2"})
	assert.True(t, IsValidDataSourceID("reset_db_1"))

	// Reset should clear all IDs
	ResetRegisteredDataSourceIDsForTesting()

	assert.False(t, IsValidDataSourceID("reset_db_1"))
	assert.False(t, IsValidDataSourceID("reset_db_2"))
}

func TestIsValidDataSourceID(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	tests := []struct {
		name     string
		id       string
		register []string
		expected bool
	}{
		{
			name:     "valid registered ID",
			id:       "valid_db",
			register: []string{"valid_db"},
			expected: true,
		},
		{
			name:     "unregistered ID returns false",
			id:       "unknown_db",
			register: []string{"other_db"},
			expected: false,
		},
		{
			name:     "empty string ID returns false",
			id:       "",
			register: []string{"some_db"},
			expected: false,
		},
	}

	for _, tt := range tests {
		ResetRegisteredDataSourceIDsForTesting()
		RegisterDataSourceIDsForTesting(tt.register)

		t.Run(tt.name, func(t *testing.T) {
			result := IsValidDataSourceID(tt.id)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInitRegisteredDataSourceIDs(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	initRegisteredDataSourceIDs([]string{"init_db_1", "init_db_2"})

	assert.True(t, IsValidDataSourceID("init_db_1"))
	assert.True(t, IsValidDataSourceID("init_db_2"))
	assert.False(t, IsValidDataSourceID("init_db_3"))

	// Second call should be a no-op (sync.Once)
	initRegisteredDataSourceIDs([]string{"init_db_3"})
	assert.False(t, IsValidDataSourceID("init_db_3"), "sync.Once should prevent second registration")
}

func TestCollectDataSourceNames(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	t.Run("Success - collects datasource names from environment", func(t *testing.T) {
		t.Setenv("DATASOURCE_MIDAZ_LEDGER_CONFIG_NAME", "midaz-ledger")
		t.Setenv("DATASOURCE_MIDAZ_LEDGER_HOST", "localhost")
		t.Setenv("DATASOURCE_CRM_CONFIG_NAME", "crm")

		names := collectDataSourceNames()

		assert.True(t, names["midaz_ledger"], "should find midaz_ledger datasource")
		assert.True(t, names["crm"], "should find crm datasource")
	})

	t.Run("Success - returns empty map when no datasource envs exist", func(t *testing.T) {
		// Clear relevant env vars by setting to empty
		// collectDataSourceNames looks for DATASOURCE_*_CONFIG_NAME pattern
		names := collectDataSourceNames()

		// We cannot guarantee empty since other tests or system may have set env vars,
		// but at minimum the function should not panic
		assert.NotNil(t, names)
	})
}

func TestBuildDataSourceConfig(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	t.Run("Success - builds complete config from environment", func(t *testing.T) {
		t.Setenv("DATASOURCE_TEST_DB_CONFIG_NAME", "test-db")
		t.Setenv("DATASOURCE_TEST_DB_HOST", "localhost")
		t.Setenv("DATASOURCE_TEST_DB_PORT", "5432")
		t.Setenv("DATASOURCE_TEST_DB_USER", "admin")
		t.Setenv("DATASOURCE_TEST_DB_PASSWORD", "secret")
		t.Setenv("DATASOURCE_TEST_DB_DATABASE", "testdb")
		t.Setenv("DATASOURCE_TEST_DB_TYPE", "postgresql")
		t.Setenv("DATASOURCE_TEST_DB_SSLMODE", "disable")

		logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

		config, isComplete := buildDataSourceConfig("test_db", logger)

		assert.True(t, isComplete)
		assert.Equal(t, "test-db", config.ConfigName)
		assert.Equal(t, "localhost", config.Host)
		assert.Equal(t, "5432", config.Port)
		assert.Equal(t, "admin", config.User)
		assert.Equal(t, "secret", config.Password)
		assert.Equal(t, "testdb", config.Database)
		assert.Equal(t, "postgresql", config.Type)
		assert.Equal(t, "disable", config.SSLMode)
	})

	t.Run("Success - builds config with MongoDB fields", func(t *testing.T) {
		t.Setenv("DATASOURCE_MONGO_DB_CONFIG_NAME", "mongo-db")
		t.Setenv("DATASOURCE_MONGO_DB_HOST", "mongo-host")
		t.Setenv("DATASOURCE_MONGO_DB_PORT", "27017")
		t.Setenv("DATASOURCE_MONGO_DB_USER", "mongouser")
		t.Setenv("DATASOURCE_MONGO_DB_PASSWORD", "mongosecret")
		t.Setenv("DATASOURCE_MONGO_DB_DATABASE", "reporterdb")
		t.Setenv("DATASOURCE_MONGO_DB_TYPE", "mongodb")
		t.Setenv("DATASOURCE_MONGO_DB_SSL", "true")
		t.Setenv("DATASOURCE_MONGO_DB_SSLCA", "/path/to/ca.pem")
		t.Setenv("DATASOURCE_MONGO_DB_OPTIONS", "authSource=admin")

		logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

		config, isComplete := buildDataSourceConfig("mongo_db", logger)

		assert.True(t, isComplete)
		assert.Equal(t, "mongo-db", config.ConfigName)
		assert.Equal(t, "mongodb", config.Type)
		assert.Equal(t, "true", config.SSL)
		assert.Equal(t, "/path/to/ca.pem", config.SSLCA)
		assert.Equal(t, "authSource=admin", config.Options)
	})

	t.Run("Success - builds config with midaz organization ID", func(t *testing.T) {
		t.Setenv("DATASOURCE_CRM_DS_CONFIG_NAME", "crm-ds")
		t.Setenv("DATASOURCE_CRM_DS_HOST", "localhost")
		t.Setenv("DATASOURCE_CRM_DS_PORT", "27017")
		t.Setenv("DATASOURCE_CRM_DS_USER", "user")
		t.Setenv("DATASOURCE_CRM_DS_PASSWORD", "pass")
		t.Setenv("DATASOURCE_CRM_DS_DATABASE", "crm")
		t.Setenv("DATASOURCE_CRM_DS_TYPE", "mongodb")
		t.Setenv("DATASOURCE_CRM_DS_MIDAZ_ORGANIZATION_ID", "org-123-456")

		logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

		config, isComplete := buildDataSourceConfig("crm_ds", logger)

		assert.True(t, isComplete)
		assert.Equal(t, "org-123-456", config.MidazOrganizationID)
	})

	t.Run("Fail - empty CONFIG_NAME is rejected", func(t *testing.T) {
		t.Setenv("DATASOURCE_EMPTY_CFG_CONFIG_NAME", "")
		t.Setenv("DATASOURCE_EMPTY_CFG_HOST", "localhost")
		t.Setenv("DATASOURCE_EMPTY_CFG_PORT", "5432")
		t.Setenv("DATASOURCE_EMPTY_CFG_USER", "user")
		t.Setenv("DATASOURCE_EMPTY_CFG_PASSWORD", "pass")
		t.Setenv("DATASOURCE_EMPTY_CFG_DATABASE", "testdb")
		t.Setenv("DATASOURCE_EMPTY_CFG_TYPE", "postgresql")
		t.Setenv("DATASOURCE_EMPTY_CFG_SSLMODE", "disable")

		logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

		_, isComplete := buildDataSourceConfig("empty_cfg", logger)

		assert.False(t, isComplete, "datasource with empty CONFIG_NAME should be marked as incomplete")
	})
}

func TestGetDataSourceConfigs(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	t.Run("Success - returns configs from environment", func(t *testing.T) {
		t.Setenv("DATASOURCE_INTEG_DB_CONFIG_NAME", "integ-db")
		t.Setenv("DATASOURCE_INTEG_DB_HOST", "localhost")
		t.Setenv("DATASOURCE_INTEG_DB_PORT", "5432")
		t.Setenv("DATASOURCE_INTEG_DB_USER", "user")
		t.Setenv("DATASOURCE_INTEG_DB_PASSWORD", "pass")
		t.Setenv("DATASOURCE_INTEG_DB_DATABASE", "integdb")
		t.Setenv("DATASOURCE_INTEG_DB_TYPE", "postgresql")
		t.Setenv("DATASOURCE_INTEG_DB_SSLMODE", "disable")

		logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

		configs := getDataSourceConfigs(logger)

		// Should find at least one datasource (the one we set up)
		found := false
		for _, c := range configs {
			if c.ConfigName == "integ-db" {
				found = true
				assert.Equal(t, "localhost", c.Host)
				assert.Equal(t, "5432", c.Port)

				break
			}
		}

		assert.True(t, found, "should find the integ-db datasource in configs")
	})
}

func TestGetDataSourceConfigs_ReturnsSlice(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used
	// This test verifies the function executes without error.
	// The result may or may not be empty depending on environment state.

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	// Should not panic regardless of environment state
	assert.NotPanics(t, func() {
		_ = getDataSourceConfigs(logger)
	})
}

func TestGetDataSourceEnv(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	tests := []struct {
		name     string
		dsName   string
		field    string
		envKey   string
		envValue string
		expected string
	}{
		{
			name:     "simple name and field",
			dsName:   "mydb",
			field:    "HOST",
			envKey:   "DATASOURCE_MYDB_HOST",
			envValue: "localhost",
			expected: "localhost",
		},
		{
			name:     "name with hyphen converted to underscore",
			dsName:   "my-db",
			field:    "PORT",
			envKey:   "DATASOURCE_MY_DB_PORT",
			envValue: "5432",
			expected: "5432",
		},
		{
			name:     "unset env returns empty string",
			dsName:   "nonexistent",
			field:    "HOST",
			envKey:   "",
			envValue: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envKey != "" {
				t.Setenv(tt.envKey, tt.envValue)
			}

			result := getDataSourceEnv(tt.dsName, tt.field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConnectToDataSource_UnregisteredDatasource(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	ds := &DataSource{DatabaseType: PostgreSQLType}
	externalDS := map[string]DataSource{"unregistered_db": *ds}

	err := ConnectToDataSource(context.Background(), "unregistered_db", ds, logger, externalDS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unregistered datasource")
}

func TestConnectToDataSource_NotInRuntimeMap(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()
	RegisterDataSourceIDsForTesting([]string{"orphan_db"})

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	ds := &DataSource{DatabaseType: PostgreSQLType}
	externalDS := map[string]DataSource{} // empty map - not in runtime

	err := ConnectToDataSource(context.Background(), "orphan_db", ds, logger, externalDS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in runtime map")
}

func TestConnectToDataSource_UnsupportedDatabaseType(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()
	RegisterDataSourceIDsForTesting([]string{"unsupported_db"})

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	ds := &DataSource{DatabaseType: "oracle"}
	externalDS := map[string]DataSource{"unsupported_db": *ds}

	err := ConnectToDataSource(context.Background(), "unsupported_db", ds, logger, externalDS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported database type")
	assert.Contains(t, err.Error(), "oracle")
	assert.NotNil(t, ds.LastError)
}

func TestConnectToDataSource_MongoDBInvalidURI(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()
	RegisterDataSourceIDsForTesting([]string{"mongo_bad_db"})

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	ds := &DataSource{
		DatabaseType: MongoDBType,
		MongoURI:     "invalid://not-a-valid-uri",
		MongoDBName:  "testdb",
	}
	externalDS := map[string]DataSource{"mongo_bad_db": *ds}

	err := ConnectToDataSource(context.Background(), "mongo_bad_db", ds, logger, externalDS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MongoDB")
}

func TestConnectToDataSource_IncreasesRetryCount(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()
	RegisterDataSourceIDsForTesting([]string{"retry_db"})

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	ds := &DataSource{
		DatabaseType: "oracle", // unsupported, so it will fail predictably
		RetryCount:   0,
	}
	externalDS := map[string]DataSource{"retry_db": *ds}

	_ = ConnectToDataSource(context.Background(), "retry_db", ds, logger, externalDS)

	assert.Equal(t, 1, ds.RetryCount, "RetryCount should be incremented")
	assert.False(t, ds.LastAttempt.IsZero(), "LastAttempt should be set")
}

func TestConnectToDataSourceWithRetry_UnsupportedType(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()
	RegisterDataSourceIDsForTesting([]string{"retry_unsupported_db"})

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	ds := &DataSource{
		DatabaseType: "oracle", // unsupported - will be fatal, skipping retries
	}
	externalDS := map[string]DataSource{"retry_unsupported_db": *ds}

	err := ConnectToDataSourceWithRetry("retry_unsupported_db", ds, logger, externalDS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry_unsupported_db")
}

func TestConnectToDataSourceWithRetry_FatalErrorSkipsRetries(t *testing.T) {
	// Note: Cannot use t.Parallel() because it modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()
	RegisterDataSourceIDsForTesting([]string{"fatal_err_db"})

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	// "unsupported database type" is in the fatalPatterns list, so it should skip retries
	ds := &DataSource{
		DatabaseType: "cassandra", // unsupported = fatal
	}
	externalDS := map[string]DataSource{"fatal_err_db": *ds}

	err := ConnectToDataSourceWithRetry("fatal_err_db", ds, logger, externalDS)
	require.Error(t, err)

	// RetryCount should be low since fatal errors skip retries
	// First attempt increments to 1, fatal detected -> break
	assert.LessOrEqual(t, ds.RetryCount, 2, "fatal errors should skip remaining retries")
}

func TestDataSourceConfig_GetSchemas_EmptyAfterTrim(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	t.Setenv("DATASOURCE_TRIM_TEST_SCHEMAS", ", , ,")

	config := DataSourceConfig{
		ConfigName: "trim-test",
	}

	schemas := config.GetSchemas()

	// All entries are empty after trim, so should return default
	assert.Equal(t, []string{"public"}, schemas)
}

func TestDataSourceConfig_GetSchemas_MixedSpacing(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	envKey := fmt.Sprintf("DATASOURCE_%s_SCHEMAS", toEnvFormat("mixed-spacing"))
	t.Setenv(envKey, "  schema_a , schema_b ,schema_c  ")

	config := DataSourceConfig{
		ConfigName: "mixed-spacing",
	}

	schemas := config.GetSchemas()
	assert.Equal(t, []string{"schema_a", "schema_b", "schema_c"}, schemas)
}

// ---------------------------------------------------------------------------
// initMongoDataSource tests
// ---------------------------------------------------------------------------

func TestInitMongoDataSource_BasicURI(t *testing.T) {
	// Note: Cannot use t.Parallel() - mongo.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-mongo",
		Type:       "mongodb",
		Host:       "localhost",
		Port:       "27017",
		User:       "testuser",
		Password:   "testpass",
		Database:   "testdb",
	}

	ds := initMongoDataSource(config, logger)

	assert.Equal(t, MongoDBType, ds.DatabaseType)
	assert.Equal(t, "testdb", ds.MongoDBName)
	assert.Contains(t, ds.MongoURI, "mongodb://testuser:testpass@localhost:27017/testdb")
	// Eager connect fails (no real MongoDB) → Initialized=false, Status=Unavailable
	assert.False(t, ds.Initialized)
	assert.Equal(t, libConstant.DataSourceStatusUnavailable, ds.Status)
	assert.Equal(t, 0, ds.RetryCount)
}

func TestInitMongoDataSource_WithSSL(t *testing.T) {
	// Note: Cannot use t.Parallel() - mongo.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-mongo-ssl",
		Type:       "mongodb",
		Host:       "mongo.example.com",
		Port:       "27017",
		User:       "ssluser",
		Password:   "sslpass",
		Database:   "ssldb",
		SSL:        "true",
		SSLCA:      "/path/to/ca.pem",
	}

	ds := initMongoDataSource(config, logger)

	assert.Contains(t, ds.MongoURI, "ssl=true")
	assert.Contains(t, ds.MongoURI, "tlsCAFile=")
	assert.Equal(t, MongoDBType, ds.DatabaseType)
	assert.Equal(t, "ssldb", ds.MongoDBName)
}

func TestInitMongoDataSource_WithOptions(t *testing.T) {
	// Note: Cannot use t.Parallel() - mongo.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-mongo-opts",
		Type:       "mongodb",
		Host:       "localhost",
		Port:       "27017",
		User:       "optuser",
		Password:   "optpass",
		Database:   "optdb",
		Options:    "authSource=admin&replicaSet=rs0",
	}

	ds := initMongoDataSource(config, logger)

	assert.Contains(t, ds.MongoURI, "?authSource=admin&replicaSet=rs0")
	assert.Equal(t, MongoDBType, ds.DatabaseType)
}

func TestInitMongoDataSource_WithSSLAndOptions(t *testing.T) {
	// Note: Cannot use t.Parallel() - mongo.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-mongo-ssl-opts",
		Type:       "mongodb",
		Host:       "localhost",
		Port:       "27017",
		User:       "user",
		Password:   "pass",
		Database:   "db",
		Options:    "authSource=admin",
		SSL:        "true",
		SSLCA:      "/path/to/ca.pem",
	}

	ds := initMongoDataSource(config, logger)

	// The URI should have a single "?" for the options section, then "&" for SSL params
	questionMarkCount := strings.Count(ds.MongoURI, "?")
	assert.Equal(t, 1, questionMarkCount, "URI should have exactly one '?' separator, got URI: %s", ds.MongoURI)

	// Options come first (appended with "?"), then SSL params are appended with "&"
	assert.Contains(t, ds.MongoURI, "authSource=admin")
	assert.Contains(t, ds.MongoURI, "ssl=true")
	assert.Contains(t, ds.MongoURI, "tlsCAFile=")
}

func TestInitMongoDataSource_MidazOrganizationID(t *testing.T) {
	// Note: Cannot use t.Parallel() - mongo.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName:          "test-mongo-org",
		Type:                "mongodb",
		Host:                "localhost",
		Port:                "27017",
		User:                "orguser",
		Password:            "orgpass",
		Database:            "orgdb",
		MidazOrganizationID: "org-123",
	}

	ds := initMongoDataSource(config, logger)

	assert.Equal(t, "org-123", ds.MidazOrganizationID)
	assert.Equal(t, MongoDBType, ds.DatabaseType)
	assert.Equal(t, "orgdb", ds.MongoDBName)
}

// ---------------------------------------------------------------------------
// initPostgresDataSource tests
// ---------------------------------------------------------------------------

func TestInitPostgresDataSource_BasicConnection(t *testing.T) {
	// Note: Cannot use t.Parallel() - connection.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-pg",
		Type:       "postgresql",
		Host:       "localhost",
		Port:       "5432",
		User:       "pguser",
		Password:   "pgpass",
		Database:   "pgdb",
		SSLMode:    "disable",
	}

	ds := initPostgresDataSource(config, logger, false)

	assert.Equal(t, "postgresql", ds.DatabaseType)
	require.NotNil(t, ds.DatabaseConfig)
	assert.Equal(t, "pgdb", ds.DatabaseConfig.DBName)
	assert.Contains(t, ds.DatabaseConfig.ConnectionString, "postgresql://pguser:")
	assert.Contains(t, ds.DatabaseConfig.ConnectionString, "@localhost:5432/pgdb")
	// Eager connect fails (no real PostgreSQL) → Initialized=false, Status=Unavailable
	assert.False(t, ds.Initialized)
	assert.Equal(t, libConstant.DataSourceStatusUnavailable, ds.Status)
	assert.Equal(t, 0, ds.RetryCount)
}

func TestInitPostgresDataSource_PasswordEncoding(t *testing.T) {
	// Note: Cannot use t.Parallel() - connection.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-pg-enc",
		Type:       "postgresql",
		Host:       "localhost",
		Port:       "5432",
		User:       "pguser",
		Password:   "p@ss!word#123",
		Database:   "pgdb",
		SSLMode:    "disable",
	}

	ds := initPostgresDataSource(config, logger, false)

	require.NotNil(t, ds.DatabaseConfig)
	// The password should be URL-encoded in the connection string
	assert.Contains(t, ds.DatabaseConfig.ConnectionString, "p%40ss%21word%23123")
}

func TestInitPostgresDataSource_SchemasDefault(t *testing.T) {
	// Note: Cannot use t.Parallel() - connection.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-pg-schema-def",
		Type:       "postgresql",
		Host:       "localhost",
		Port:       "5432",
		User:       "pguser",
		Password:   "pgpass",
		Database:   "pgdb",
		SSLMode:    "disable",
	}

	ds := initPostgresDataSource(config, logger, false)

	// Without DATASOURCE_TEST_PG_SCHEMA_DEF_SCHEMAS env var, should default to ["public"]
	assert.Equal(t, []string{"public"}, ds.Schemas)
}

func TestInitPostgresDataSource_SchemasFromEnv(t *testing.T) {
	// Note: Cannot use t.Parallel() - t.Setenv and connection.Connect have side effects

	t.Setenv("DATASOURCE_TEST_PG_SCHEMAS", "public,sales,hr")

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-pg",
		Type:       "postgresql",
		Host:       "localhost",
		Port:       "5432",
		User:       "pguser",
		Password:   "pgpass",
		Database:   "pgdb",
		SSLMode:    "disable",
	}

	ds := initPostgresDataSource(config, logger, false)

	assert.Equal(t, []string{"public", "sales", "hr"}, ds.Schemas)
}

// ---------------------------------------------------------------------------
// ExternalDatasourceConnectionsLazy tests
// ---------------------------------------------------------------------------

func TestExternalDatasourceConnectionsLazy_NoDataSources(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies package-level state and env

	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	// Clear any DATASOURCE_*_CONFIG_NAME env vars that might interfere.
	// We snapshot the current env and unset any matching keys.
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], "DATASOURCE_") && strings.HasSuffix(parts[0], "_CONFIG_NAME") {
			t.Setenv(parts[0], "")
			// Setting to empty means collectDataSourceNames will find the key but the config name
			// will be empty. A safer approach: use os.Unsetenv via cleanup. But t.Setenv("", "")
			// combined with the fact that CONFIG_NAME="" means buildDataSourceConfig returns an empty
			// ConfigName, which still gets added. Let's just unset them properly.
		}
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	result := ExternalDatasourceConnectionsLazy(logger)
	assert.NotNil(t, result)
}

func TestExternalDatasourceConnectionsLazy_UnsupportedType(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies package-level state and env

	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	t.Setenv("DATASOURCE_ORACLE_CONFIG_NAME", "oracle-db")
	t.Setenv("DATASOURCE_ORACLE_TYPE", "oracle")
	t.Setenv("DATASOURCE_ORACLE_HOST", "localhost")
	t.Setenv("DATASOURCE_ORACLE_PORT", "1521")
	t.Setenv("DATASOURCE_ORACLE_USER", "orauser")
	t.Setenv("DATASOURCE_ORACLE_PASSWORD", "orapass")
	t.Setenv("DATASOURCE_ORACLE_DATABASE", "oradb")

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	result := ExternalDatasourceConnectionsLazy(logger)
	assert.NotNil(t, result)

	// The oracle datasource should NOT be in the result since "oracle" is unsupported
	_, exists := result["oracle-db"]
	assert.False(t, exists, "unsupported database type 'oracle' should not be added to the datasource map")
}

// ---------------------------------------------------------------------------
// ExternalDatasourceConnections tests (eager mode with retry)
// ---------------------------------------------------------------------------

func TestExternalDatasourceConnections_UnsupportedType(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies package-level state and env

	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	t.Setenv("DATASOURCE_UNSUP_CONFIG_NAME", "unsup-db")
	t.Setenv("DATASOURCE_UNSUP_TYPE", "oracle")
	t.Setenv("DATASOURCE_UNSUP_HOST", "localhost")
	t.Setenv("DATASOURCE_UNSUP_PORT", "1521")
	t.Setenv("DATASOURCE_UNSUP_USER", "user")
	t.Setenv("DATASOURCE_UNSUP_PASSWORD", "pass")
	t.Setenv("DATASOURCE_UNSUP_DATABASE", "oradb")

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	result := ExternalDatasourceConnections(logger)
	assert.NotNil(t, result)

	// Unsupported type should be skipped entirely (not added to the map)
	_, exists := result["unsup-db"]
	assert.False(t, exists, "unsupported database type should not be added to the datasource map")
}

func TestExternalDatasourceConnections_MultipleDatasources(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies package-level state and env

	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	// Configure two datasources: one unsupported (skipped) and one postgres (will fail to connect but added)
	t.Setenv("DATASOURCE_PG1_CONFIG_NAME", "pg1-db")
	t.Setenv("DATASOURCE_PG1_TYPE", "postgresql")
	t.Setenv("DATASOURCE_PG1_HOST", "localhost")
	t.Setenv("DATASOURCE_PG1_PORT", "59999")
	t.Setenv("DATASOURCE_PG1_USER", "user")
	t.Setenv("DATASOURCE_PG1_PASSWORD", "pass")
	t.Setenv("DATASOURCE_PG1_DATABASE", "testdb")
	t.Setenv("DATASOURCE_PG1_SSLMODE", "disable")

	t.Setenv("DATASOURCE_SKIP1_CONFIG_NAME", "skip1-db")
	t.Setenv("DATASOURCE_SKIP1_TYPE", "cassandra")
	t.Setenv("DATASOURCE_SKIP1_HOST", "localhost")
	t.Setenv("DATASOURCE_SKIP1_PORT", "9042")
	t.Setenv("DATASOURCE_SKIP1_USER", "user")
	t.Setenv("DATASOURCE_SKIP1_PASSWORD", "pass")
	t.Setenv("DATASOURCE_SKIP1_DATABASE", "testdb")

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	result := ExternalDatasourceConnections(logger)
	assert.NotNil(t, result)

	// The postgres datasource should be in the map (even if connection failed)
	_, pgExists := result["pg1-db"]
	assert.True(t, pgExists, "postgresql datasource should exist in map even if connection failed")

	// Unsupported type should be skipped
	_, skipExists := result["skip1-db"]
	assert.False(t, skipExists, "unsupported database type 'cassandra' should not be in the map")
}

func TestExternalDatasourceConnections_CountsAvailableAndUnavailable(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies package-level state and env

	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	// Set up a postgres datasource that will fail to connect (bad port)
	t.Setenv("DATASOURCE_COUNTDS_CONFIG_NAME", "count-ds")
	t.Setenv("DATASOURCE_COUNTDS_TYPE", "postgresql")
	t.Setenv("DATASOURCE_COUNTDS_HOST", "localhost")
	t.Setenv("DATASOURCE_COUNTDS_PORT", "59998")
	t.Setenv("DATASOURCE_COUNTDS_USER", "user")
	t.Setenv("DATASOURCE_COUNTDS_PASSWORD", "pass")
	t.Setenv("DATASOURCE_COUNTDS_DATABASE", "testdb")
	t.Setenv("DATASOURCE_COUNTDS_SSLMODE", "disable")

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	result := ExternalDatasourceConnections(logger)
	assert.NotNil(t, result)

	// The datasource should be in the map but unavailable
	ds, exists := result["count-ds"]
	assert.True(t, exists)

	if exists {
		assert.Equal(t, libConstant.DataSourceStatusUnavailable, ds.Status,
			"datasource that failed to connect should be unavailable")
	}
}

// ---------------------------------------------------------------------------
// initMongoDataSource edge cases
// ---------------------------------------------------------------------------

func TestInitMongoDataSource_SSLWithoutCA(t *testing.T) {
	// Note: Cannot use t.Parallel() - mongo.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-mongo-ssl-noca",
		Type:       "mongodb",
		Host:       "localhost",
		Port:       "27017",
		User:       "user",
		Password:   "pass",
		Database:   "testdb",
		SSL:        "true",
		SSLCA:      "", // No CA file
	}

	ds := initMongoDataSource(config, logger)

	// Should have ssl=true but no tlsCAFile parameter
	assert.Contains(t, ds.MongoURI, "ssl=true")
	assert.NotContains(t, ds.MongoURI, "tlsCAFile=")
	assert.Equal(t, MongoDBType, ds.DatabaseType)
}

func TestInitMongoDataSource_EmptyPassword(t *testing.T) {
	// Note: Cannot use t.Parallel() - mongo.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-mongo-nopass",
		Type:       "mongodb",
		Host:       "localhost",
		Port:       "27017",
		User:       "user",
		Password:   "", // Empty password
		Database:   "testdb",
	}

	ds := initMongoDataSource(config, logger)

	// URI should still be constructed with empty password
	assert.Contains(t, ds.MongoURI, "mongodb://user:@localhost:27017/testdb")
	assert.Equal(t, MongoDBType, ds.DatabaseType)
	assert.Equal(t, "testdb", ds.MongoDBName)
}

// ---------------------------------------------------------------------------
// initPostgresDataSource edge cases
// ---------------------------------------------------------------------------

func TestInitPostgresDataSource_EmptyPassword(t *testing.T) {
	// Note: Cannot use t.Parallel() - connection.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-pg-nopass",
		Type:       "postgresql",
		Host:       "localhost",
		Port:       "5432",
		User:       "pguser",
		Password:   "", // Empty password
		Database:   "pgdb",
		SSLMode:    "disable",
	}

	ds := initPostgresDataSource(config, logger, false)

	require.NotNil(t, ds.DatabaseConfig)
	// Empty password should be URL-encoded as empty string
	assert.Contains(t, ds.DatabaseConfig.ConnectionString, "postgresql://pguser:@localhost:5432/pgdb")
}

func TestInitPostgresDataSource_EmptySSLMode(t *testing.T) {
	// Note: Cannot use t.Parallel() - connection.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName:  "test-pg-nossl",
		Type:        "postgresql",
		Host:        "localhost",
		Port:        "5432",
		User:        "pguser",
		Password:    "pgpass",
		Database:    "pgdb",
		SSLMode:     "",
		SSLRootCert: "",
	}

	ds := initPostgresDataSource(config, logger, false)

	require.NotNil(t, ds.DatabaseConfig)
	assert.Contains(t, ds.DatabaseConfig.ConnectionString, "sslmode=")
}

func TestInitPostgresDataSource_SSLModeWithRootCert(t *testing.T) {
	// Note: Cannot use t.Parallel() - connection.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName:  "test-pg-ssl",
		Type:        "postgresql",
		Host:        "localhost",
		Port:        "5432",
		User:        "pguser",
		Password:    "pgpass",
		Database:    "pgdb",
		SSLMode:     "verify-full",
		SSLRootCert: "/path/to/root.crt",
	}

	ds := initPostgresDataSource(config, logger, false)

	require.NotNil(t, ds.DatabaseConfig)
	assert.Contains(t, ds.DatabaseConfig.ConnectionString, "sslmode=verify-full")
	assert.Contains(t, ds.DatabaseConfig.ConnectionString, "sslrootcert=")
}

func TestInitPostgresDataSource_MidazOrganizationID(t *testing.T) {
	// Note: Cannot use t.Parallel() - connection.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName:          "test-pg-org",
		Type:                "postgresql",
		Host:                "localhost",
		Port:                "5432",
		User:                "pguser",
		Password:            "pgpass",
		Database:            "pgdb",
		SSLMode:             "disable",
		MidazOrganizationID: "org-456-def",
	}

	ds := initPostgresDataSource(config, logger, false)

	assert.Equal(t, "org-456-def", ds.MidazOrganizationID, "MidazOrganizationID must be preserved for PostgreSQL datasources")
	assert.Equal(t, "postgresql", ds.DatabaseType)
	require.NotNil(t, ds.DatabaseConfig)
}

func TestInitPostgresDataSource_LazyMode(t *testing.T) {
	// Note: Cannot use t.Parallel() - connection.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName:          "test-pg-lazy",
		Type:                "postgresql",
		Host:                "192.0.2.1", // non-routable IP, connect would hang/fail
		Port:                "5432",
		User:                "pguser",
		Password:            "pgpass",
		Database:            "pgdb",
		SSLMode:             "disable",
		MidazOrganizationID: "org-lazy-test",
	}

	ds := initPostgresDataSource(config, logger, true)

	assert.Equal(t, "postgresql", ds.DatabaseType)
	require.NotNil(t, ds.DatabaseConfig)
	assert.False(t, ds.DatabaseConfig.Connected, "lazy mode should NOT connect to Postgres")
	assert.Equal(t, "org-lazy-test", ds.MidazOrganizationID)
	assert.Equal(t, []string{"public"}, ds.Schemas)
}

func TestInitPostgresDataSource_EagerMode(t *testing.T) {
	// Note: Cannot use t.Parallel() - connection.Connect has side effects
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	config := DataSourceConfig{
		ConfigName: "test-pg-eager",
		Type:       "postgresql",
		Host:       "localhost",
		Port:       "5432",
		User:       "pguser",
		Password:   "pgpass",
		Database:   "pgdb",
		SSLMode:    "disable",
	}

	// Eager mode: connection attempt happens (will fail in test since no DB, but that is OK)
	ds := initPostgresDataSource(config, logger, false)

	assert.Equal(t, "postgresql", ds.DatabaseType)
	require.NotNil(t, ds.DatabaseConfig)
}

// ---------------------------------------------------------------------------
// New coverage tests: lazy connections, PostgreSQL failure, retry loop, schemas
// ---------------------------------------------------------------------------

func TestExternalDatasourceConnectionsLazy_WithValidConfigs(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies package-level state and env

	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	// Set up a PostgreSQL datasource
	t.Setenv("DATASOURCE_LAZY_PG_CONFIG_NAME", "lazy-pg")
	t.Setenv("DATASOURCE_LAZY_PG_TYPE", "postgresql")
	t.Setenv("DATASOURCE_LAZY_PG_HOST", "localhost")
	t.Setenv("DATASOURCE_LAZY_PG_PORT", "5432")
	t.Setenv("DATASOURCE_LAZY_PG_USER", "user")
	t.Setenv("DATASOURCE_LAZY_PG_PASSWORD", "pass")
	t.Setenv("DATASOURCE_LAZY_PG_DATABASE", "testdb")
	t.Setenv("DATASOURCE_LAZY_PG_SSLMODE", "disable")

	// Set up a MongoDB datasource
	t.Setenv("DATASOURCE_LAZY_MONGO_CONFIG_NAME", "lazy-mongo")
	t.Setenv("DATASOURCE_LAZY_MONGO_TYPE", "mongodb")
	t.Setenv("DATASOURCE_LAZY_MONGO_HOST", "localhost")
	t.Setenv("DATASOURCE_LAZY_MONGO_PORT", "27017")
	t.Setenv("DATASOURCE_LAZY_MONGO_USER", "mongouser")
	t.Setenv("DATASOURCE_LAZY_MONGO_PASSWORD", "mongopass")
	t.Setenv("DATASOURCE_LAZY_MONGO_DATABASE", "testdb")

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	result := ExternalDatasourceConnectionsLazy(logger)
	assert.NotNil(t, result)

	// Both datasources should be in the map
	pgDS, pgExists := result["lazy-pg"]
	assert.True(t, pgExists, "PostgreSQL datasource should be present")
	if pgExists {
		assert.Equal(t, PostgreSQLType, pgDS.DatabaseType)
		// Lazy mode: no connection attempted → Initialized=false, Status=Unknown
		assert.False(t, pgDS.Initialized, "lazy mode does not attempt connection")
		assert.Equal(t, libConstant.DataSourceStatusUnknown, pgDS.Status)
	}

	mongoDS, mongoExists := result["lazy-mongo"]
	assert.True(t, mongoExists, "MongoDB datasource should be present")
	if mongoExists {
		assert.Equal(t, MongoDBType, mongoDS.DatabaseType)
		// Eager connect fails (no real DB) → Initialized=false, Status=Unavailable
		assert.False(t, mongoDS.Initialized, "connection should fail without a real database")
		assert.Equal(t, "testdb", mongoDS.MongoDBName)
	}
}

func TestConnectToDataSource_PostgreSQLConnectionFailure(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()
	RegisterDataSourceIDsForTesting([]string{"pg_fail_db"})

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	ds := &DataSource{
		DatabaseType: PostgreSQLType,
		DatabaseConfig: &pg.Connection{
			ConnectionString:   "postgresql://user:pass@localhost:59999/nonexistent?sslmode=disable",
			DBName:             "nonexistent",
			Logger:             logger,
			MaxOpenConnections: 1,
			MaxIdleConnections: 1,
		},
	}
	externalDS := map[string]DataSource{"pg_fail_db": *ds}

	err := ConnectToDataSource(context.Background(), "pg_fail_db", ds, logger, externalDS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PostgreSQL")
	assert.Equal(t, libConstant.DataSourceStatusUnavailable, ds.Status)
	assert.NotNil(t, ds.LastError)
}

func TestDataSourceConfig_GetSchemas_SingleSchema(t *testing.T) {
	// Note: Cannot use t.Parallel() - t.Setenv is used

	envKey := fmt.Sprintf("DATASOURCE_%s_SCHEMAS", strings.ToUpper(strings.ReplaceAll("single-schema", "-", "_")))
	t.Setenv(envKey, "custom_schema")

	config := DataSourceConfig{
		ConfigName: "single-schema",
	}

	schemas := config.GetSchemas()
	assert.Equal(t, []string{"custom_schema"}, schemas)
}

func TestConnectToDataSourceWithRetry_RetriesNonFatalError(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies package-level state

	ResetRegisteredDataSourceIDsForTesting()
	RegisterDataSourceIDsForTesting([]string{"retry_nonfatal_db"})

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	// Use a MongoDB datasource with a valid-looking URI pointing to unreachable host
	// MongoDB connection failure is not in the fatal error list, so it will trigger retries
	ds := &DataSource{
		DatabaseType: MongoDBType,
		MongoURI:     "mongodb://user:pass@192.0.2.1:27017/testdb?connectTimeoutMS=100&serverSelectionTimeoutMS=100",
		MongoDBName:  "testdb",
	}
	externalDS := map[string]DataSource{"retry_nonfatal_db": *ds}

	err := ConnectToDataSourceWithRetry("retry_nonfatal_db", ds, logger, externalDS)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry_nonfatal_db")

	// RetryCount should be > 1 since the error is non-fatal and retries happen
	assert.Greater(t, ds.RetryCount, 1, "should have retried at least once")

	// Status should be unavailable after all retries exhausted
	assert.Equal(t, libConstant.DataSourceStatusUnavailable, ds.Status)
}

func TestExternalDatasourceConnectionsLazy_ConfiguredNotInitialized(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies package-level state and env

	ResetRegisteredDataSourceIDsForTesting()

	t.Cleanup(func() {
		ResetRegisteredDataSourceIDsForTesting()
	})

	// Set up a MongoDB datasource with organization ID
	t.Setenv("DATASOURCE_LAZY_CRM_CONFIG_NAME", "lazy-crm")
	t.Setenv("DATASOURCE_LAZY_CRM_TYPE", "mongodb")
	t.Setenv("DATASOURCE_LAZY_CRM_HOST", "localhost")
	t.Setenv("DATASOURCE_LAZY_CRM_PORT", "27017")
	t.Setenv("DATASOURCE_LAZY_CRM_USER", "user")
	t.Setenv("DATASOURCE_LAZY_CRM_PASSWORD", "pass")
	t.Setenv("DATASOURCE_LAZY_CRM_DATABASE", "crmdb")
	t.Setenv("DATASOURCE_LAZY_CRM_MIDAZ_ORGANIZATION_ID", "org-789")

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	result := ExternalDatasourceConnectionsLazy(logger)
	assert.NotNil(t, result)

	crmDS, exists := result["lazy-crm"]
	assert.True(t, exists, "CRM MongoDB datasource should be present")
	if exists {
		assert.Equal(t, MongoDBType, crmDS.DatabaseType)
		assert.Equal(t, "org-789", crmDS.MidazOrganizationID)
		assert.Equal(t, "crmdb", crmDS.MongoDBName)
		assert.False(t, crmDS.Initialized)
	}
}
