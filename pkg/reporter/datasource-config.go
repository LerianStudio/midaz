// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	pg "github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"

	libConstant "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/LerianStudio/lib-observability/log"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// registeredDataSourceIDs holds the immutable set of valid datasource IDs.
// This is populated once at startup and never modified, providing a source of truth
// for validating datasource names and preventing map corruption from invalid IDs.
var (
	registeredDataSourceIDs     = make(map[string]struct{})
	registeredDataSourceIDsOnce sync.Once
	registeredDataSourceIDsLock sync.RWMutex
)

// initRegisteredDataSourceIDs initializes the immutable set of valid datasource IDs.
// This should be called once at startup before any datasource operations.
func initRegisteredDataSourceIDs(ids []string) {
	registeredDataSourceIDsOnce.Do(func() {
		registeredDataSourceIDsLock.Lock()
		defer registeredDataSourceIDsLock.Unlock()

		for _, id := range ids {
			registeredDataSourceIDs[id] = struct{}{}
		}
	})
}

// RegisterDataSourceIDsForTesting allows tests to register datasource IDs.
// This should ONLY be used in tests. In production, IDs are registered at startup.
func RegisterDataSourceIDsForTesting(ids []string) {
	registeredDataSourceIDsLock.Lock()
	defer registeredDataSourceIDsLock.Unlock()

	for _, id := range ids {
		registeredDataSourceIDs[id] = struct{}{}
	}
}

// ResetRegisteredDataSourceIDsForTesting clears all registered IDs and resets the sync.Once.
// This should ONLY be used in tests to ensure test isolation.
func ResetRegisteredDataSourceIDsForTesting() {
	registeredDataSourceIDsLock.Lock()
	defer registeredDataSourceIDsLock.Unlock()

	registeredDataSourceIDs = make(map[string]struct{})
	registeredDataSourceIDsOnce = sync.Once{}
}

// IsValidDataSourceID checks if a datasource ID was registered at startup.
// This is the authoritative check for valid datasource names.
func IsValidDataSourceID(id string) bool {
	registeredDataSourceIDsLock.RLock()
	defer registeredDataSourceIDsLock.RUnlock()

	_, exists := registeredDataSourceIDs[id]

	return exists
}

// DataSourceConfig represents the configuration required to establish a connection to a data source.
// Fields include name, connection details, authentication, database, type, and SSL mode.
type DataSourceConfig struct {
	ConfigName          string
	Name                string
	Host                string
	Port                string
	User                string
	Password            string
	Database            string
	Type                string
	SSLMode             string
	SSLCert             string
	SSLRootCert         string
	SSL                 string
	SSLCA               string
	Options             string
	MidazOrganizationID string // Used for CRM datasources to construct collection names
}

// getDataSourceEnv reads an environment variable for a datasource field using the
// DATASOURCE_{NAME}_{FIELD} naming convention. This centralizes env var access for
// dynamic datasource discovery, where the number of datasources is not known at compile time.
func getDataSourceEnv(name, field string) string {
	upperName := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	return os.Getenv(fmt.Sprintf("DATASOURCE_%s_%s", upperName, field))
}

// GetSchemas returns the configured schemas for this datasource.
// It reads from the environment variable DATASOURCE_{NAME}_SCHEMAS.
// If not configured, it defaults to ["public"].
func (c *DataSourceConfig) GetSchemas() []string {
	schemasStr := getDataSourceEnv(c.ConfigName, "SCHEMAS")

	if schemasStr == "" {
		return []string{"public"}
	}

	rawSchemas := strings.Split(schemasStr, ",")
	schemas := make([]string, 0, len(rawSchemas))

	for _, s := range rawSchemas {
		s = strings.TrimSpace(s)
		if s != "" {
			schemas = append(schemas, s)
		}
	}

	if len(schemas) == 0 {
		return []string{"public"}
	}

	return schemas
}

// DataSource represents a configuration for an external data source, specifying the database type and repository used.
type DataSource struct {
	// DatabaseType specifies the type of database being used, such as "postgresql" or "mongodb".
	DatabaseType string

	// PostgresRepository is an interface for querying PostgreSQL tables and fields in an external data source.
	PostgresRepository pg.Repository

	// MongoDBRepository is an interface for querying MongoDB collections and fields in an external data source.
	MongoDBRepository mongodb.Repository

	// DatabaseConfig holds the configuration needed to establish a connection
	DatabaseConfig *pg.Connection

	// MongoURI holds the MongoDB connection string
	MongoURI string

	// MongoDBName holds the MongoDB database name
	MongoDBName string

	// Connection holds the actual database connection that can be closed
	Connection *pg.Connection

	// Initialized indicates if the connection has been established
	Initialized bool

	// Status indicates the current health status of the datasource
	Status string

	// LastError stores the most recent error encountered
	LastError error

	// LastAttempt stores the timestamp of the last connection attempt
	LastAttempt time.Time

	// RetryCount tracks how many times we've attempted to connect
	RetryCount int

	// Schemas holds the list of database schemas to query (PostgreSQL only)
	// Defaults to ["public"] if not configured
	Schemas []string

	// MidazOrganizationID holds the Midaz organization ID for CRM datasources
	// Used to construct collection names like "holder_{org_id}"
	MidazOrganizationID string
}

// ConnectToDataSource establishes a connection to a data source if not already initialized.
func ConnectToDataSource(ctx context.Context, databaseName string, dataSource *DataSource, logger log.Logger, externalDataSources map[string]DataSource) error {
	// Primary validation: check against immutable set of registered IDs (source of truth)
	if !IsValidDataSourceID(databaseName) {
		logger.Log(ctx, log.LevelError, "Attempted to connect to unregistered datasource - not in immutable registry, operation rejected", log.String("datasource", databaseName))
		return fmt.Errorf("cannot connect to unregistered datasource: %s", databaseName)
	}

	// Secondary validation: ensure datasource exists in the runtime map
	if _, exists := externalDataSources[databaseName]; !exists {
		logger.Log(ctx, log.LevelError, "Datasource is registered but not in runtime map - possible corruption", log.String("datasource", databaseName))
		return fmt.Errorf("datasource %s not found in runtime map", databaseName)
	}

	dataSource.LastAttempt = time.Now()
	dataSource.RetryCount++

	switch dataSource.DatabaseType {
	case PostgreSQLType:
		pgRepo, pgErr := pg.NewDataSourceRepository(dataSource.DatabaseConfig)
		if pgErr != nil {
			dataSource.Status = libConstant.DataSourceStatusUnavailable
			dataSource.LastError = pgErr
			logger.Log(ctx, log.LevelError, "Failed to establish PostgreSQL connection", log.String("datasource", databaseName), log.Err(pgErr))

			return fmt.Errorf("failed to establish PostgreSQL connection to %s: %w", databaseName, pgErr)
		}

		// Only assign to the interface when the concrete value is non-nil to avoid
		// the typed-nil-in-interface trap (non-nil interface wrapping nil pointer).
		dataSource.PostgresRepository = pgRepo

		logger.Log(ctx, log.LevelInfo, "Established PostgreSQL connection", log.String("datasource", databaseName))

		dataSource.Status = libConstant.DataSourceStatusAvailable

	case MongoDBType:
		mongoRepo, mongoErr := mongodb.NewDataSourceRepository(dataSource.MongoURI, dataSource.MongoDBName, logger)
		if mongoErr != nil {
			dataSource.Status = libConstant.DataSourceStatusUnavailable
			dataSource.LastError = mongoErr
			logger.Log(ctx, log.LevelError, "Failed to establish MongoDB connection", log.String("datasource", databaseName), log.Err(mongoErr))

			return fmt.Errorf("failed to establish MongoDB connection to %s: %w", databaseName, mongoErr)
		}

		// Only assign to the interface when the concrete value is non-nil.
		dataSource.MongoDBRepository = mongoRepo

		logger.Log(ctx, log.LevelInfo, "Established MongoDB connection", log.String("datasource", databaseName))

		dataSource.Status = libConstant.DataSourceStatusAvailable

	default:
		dataSource.Status = libConstant.DataSourceStatusUnavailable
		dataSource.LastError = fmt.Errorf("unsupported database type: %s", dataSource.DatabaseType)

		return fmt.Errorf("unsupported database type: %s for database: %s", dataSource.DatabaseType, databaseName)
	}

	dataSource.Initialized = true
	dataSource.LastError = nil
	externalDataSources[databaseName] = *dataSource

	return nil
}

// isFatalError checks if an error is fatal (no point in retrying)
func isFatalError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// DNS/network errors that won't be fixed by retrying
	fatalPatterns := []string{
		"no such host",
		"lookup",
		"server misbehaving",
		"connection refused",
		"unsupported database type",
		"invalid connection string",
		"authentication failed",
		"authorization failed",
		"access denied",
	}

	for _, pattern := range fatalPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// ConnectToDataSourceWithRetry attempts to connect to a datasource with exponential backoff retry logic.
func ConnectToDataSourceWithRetry(databaseName string, dataSource *DataSource, logger log.Logger, externalDataSources map[string]DataSource) error {
	backoff := constant.DataSourceInitialBackoff

	for attempt := 0; attempt <= constant.DataSourceMaxRetries; attempt++ {
		if attempt > 0 {
			logger.Log(context.Background(), log.LevelWarn, "Retry attempt for datasource", log.Int("attempt", attempt), log.Int("max_retries", constant.DataSourceMaxRetries), log.String("datasource", databaseName), log.Any("backoff", backoff))
			time.Sleep(backoff)

			// Calculate next backoff (exponential with max cap)
			backoff = time.Duration(float64(backoff) * constant.DataSourceBackoffMultiplier)
			if backoff > constant.DataSourceMaxBackoff {
				backoff = constant.DataSourceMaxBackoff
			}
		}

		err := ConnectToDataSource(context.Background(), databaseName, dataSource, logger, externalDataSources)
		if err == nil {
			logger.Log(context.Background(), log.LevelInfo, "Successfully connected to datasource", log.String("datasource", databaseName), log.Int("attempt", attempt+1))
			return nil
		}

		logger.Log(context.Background(), log.LevelError, "Failed to connect to datasource", log.String("datasource", databaseName), log.Int("attempt", attempt+1), log.Int("max_attempts", constant.DataSourceMaxRetries+1), log.Err(err))

		// Check if error is fatal (no point in retrying)
		if isFatalError(err) {
			logger.Log(context.Background(), log.LevelWarn, "Fatal error detected for datasource - skipping remaining retries", log.String("datasource", databaseName))
			break
		}

		// Don't retry on last attempt
		if attempt == constant.DataSourceMaxRetries {
			break
		}
	}

	logger.Log(context.Background(), log.LevelError, "Exhausted all retry attempts for datasource - marking as unavailable", log.String("datasource", databaseName))

	dataSource.Status = libConstant.DataSourceStatusUnavailable
	externalDataSources[databaseName] = *dataSource

	return fmt.Errorf("failed to connect to datasource %s after %d attempts", databaseName, constant.DataSourceMaxRetries+1)
}

// ExternalDatasourceConnectionsLazy initializes datasource configurations WITHOUT attempting connections.
// Useful for components that connect on-demand (like Manager).
func ExternalDatasourceConnectionsLazy(logger log.Logger) map[string]DataSource {
	externalDataSources := make(map[string]DataSource)

	dataSourceConfigs := getDataSourceConfigs(logger)

	// Collect valid IDs and register them in the immutable set
	validIDs := make([]string, 0, len(dataSourceConfigs))
	for _, dataSource := range dataSourceConfigs {
		validIDs = append(validIDs, dataSource.ConfigName)
	}

	initRegisteredDataSourceIDs(validIDs)
	logger.Log(context.Background(), log.LevelInfo, "Registered immutable datasource IDs", log.Int("count", len(validIDs)), log.Any("ids", validIDs))

	for _, dataSource := range dataSourceConfigs {
		var ds DataSource

		switch strings.ToLower(dataSource.Type) {
		case MongoDBType:
			ds = initMongoDataSource(dataSource, logger)
		case PostgreSQLType:
			ds = initPostgresDataSource(dataSource, logger, true)
		default:
			logger.Log(context.Background(), log.LevelError, "Unsupported database type for data source", log.String("type", dataSource.Type), log.String("datasource", dataSource.Name))
			continue
		}

		// Add datasource WITHOUT attempting connection
		externalDataSources[dataSource.ConfigName] = ds
		logger.Log(context.Background(), log.LevelInfo, "Datasource configured (lazy mode - will connect on first use)", log.String("datasource", dataSource.ConfigName))
	}

	logger.Log(context.Background(), log.LevelInfo, "Datasource lazy initialization complete", log.Int("configured", len(externalDataSources)))

	return externalDataSources
}

// ExternalDatasourceConnections initializes and returns a map of external data source connections.
// Uses graceful degradation - continues initialization even if some datasources fail.
// Attempts connection with retry for each datasource (use for Worker).
func ExternalDatasourceConnections(logger log.Logger) map[string]DataSource {
	externalDataSources := make(map[string]DataSource)

	dataSourceConfigs := getDataSourceConfigs(logger)

	// Collect valid IDs and register them in the immutable set
	validIDs := make([]string, 0, len(dataSourceConfigs))
	for _, dataSource := range dataSourceConfigs {
		validIDs = append(validIDs, dataSource.ConfigName)
	}

	initRegisteredDataSourceIDs(validIDs)
	logger.Log(context.Background(), log.LevelInfo, "Registered immutable datasource IDs", log.Int("count", len(validIDs)), log.Any("ids", validIDs))

	for _, dataSource := range dataSourceConfigs {
		var ds DataSource

		switch strings.ToLower(dataSource.Type) {
		case MongoDBType:
			ds = initMongoDataSource(dataSource, logger)
		case PostgreSQLType:
			ds = initPostgresDataSource(dataSource, logger, false)
		default:
			logger.Log(context.Background(), log.LevelError, "Unsupported database type for data source", log.String("type", dataSource.Type), log.String("datasource", dataSource.Name))
			continue
		}

		externalDataSources[dataSource.ConfigName] = ds

		// Attempt connection with retry
		err := ConnectToDataSourceWithRetry(dataSource.ConfigName, &ds, logger, externalDataSources)
		if err != nil {
			logger.Log(context.Background(), log.LevelError, "Datasource is UNAVAILABLE - system will continue without it", log.String("datasource", dataSource.ConfigName), log.Err(err))
			externalDataSources[dataSource.ConfigName] = ds
		} else {
			logger.Log(context.Background(), log.LevelInfo, "Datasource initialized successfully", log.String("datasource", dataSource.ConfigName))
			externalDataSources[dataSource.ConfigName] = ds
		}
	}

	available := 0
	unavailable := 0

	for name, ds := range externalDataSources {
		if ds.Status == libConstant.DataSourceStatusAvailable {
			available++
		} else {
			unavailable++

			logger.Log(context.Background(), log.LevelWarn, "Datasource status", log.String("datasource", name), log.String("status", ds.Status))
		}
	}

	logger.Log(context.Background(), log.LevelInfo, "Datasource initialization complete", log.Int("available", available), log.Int("unavailable", unavailable))

	return externalDataSources
}

func initMongoDataSource(dataSource DataSourceConfig, logger log.Logger) DataSource {
	mongoURI := fmt.Sprintf("%s://%s:%s@%s:%s/%s",
		dataSource.Type, dataSource.User, dataSource.Password, dataSource.Host, dataSource.Port, dataSource.Database)
	if dataSource.Options != "" {
		mongoURI += "?" + dataSource.Options
	}

	var params []string
	if dataSource.SSL == "true" {
		params = append(params, "ssl=true")
	}

	if dataSource.SSLCA != "" {
		params = append(params, "tlsCAFile="+url.QueryEscape(dataSource.SSLCA))
	}

	if len(params) > 0 {
		if strings.Contains(mongoURI, "?") {
			mongoURI += "&" + strings.Join(params, "&")
		} else {
			mongoURI += "?" + strings.Join(params, "&")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), constant.ConnectionTimeout)
	defer cancel()

	// Configure MongoDB client with pool settings and shorter timeouts
	clientOpts := options.Client().
		ApplyURI(mongoURI).
		SetMaxPoolSize(constant.MongoDBMaxPoolSize).
		SetMinPoolSize(constant.MongoDBMinPoolSize).
		SetMaxConnIdleTime(constant.MongoDBMaxConnIdleTime).
		SetConnectTimeout(constant.ConnectionTimeout).
		SetServerSelectionTimeout(constant.ConnectionTimeout)

	initialStatus := libConstant.DataSourceStatusUnknown

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		logger.Log(context.Background(), log.LevelError, "Failed to connect to MongoDB", log.String("datasource", dataSource.ConfigName), log.Err(err))

		initialStatus = libConstant.DataSourceStatusUnavailable
	} else if err := client.Ping(ctx, nil); err != nil {
		logger.Log(context.Background(), log.LevelError, "Failed to ping MongoDB", log.String("datasource", dataSource.ConfigName), log.Err(err))

		initialStatus = libConstant.DataSourceStatusUnavailable
	} else {
		logger.Log(context.Background(), log.LevelInfo, "Successfully connected to MongoDB", log.String("datasource", dataSource.ConfigName), log.Any("max_pool_size", constant.MongoDBMaxPoolSize), log.Any("min_pool_size", constant.MongoDBMinPoolSize))
	}

	// Only disconnect if client was successfully created
	if client != nil {
		_ = client.Disconnect(ctx)
	}

	return DataSource{
		DatabaseType:        MongoDBType,
		MongoURI:            mongoURI,
		MongoDBName:         dataSource.Database,
		Initialized:         false,
		Status:              initialStatus,
		LastAttempt:         time.Time{},
		RetryCount:          0,
		MidazOrganizationID: dataSource.MidazOrganizationID,
	}
}

func initPostgresDataSource(dataSource DataSourceConfig, logger log.Logger, lazy bool) DataSource {
	connectionString := fmt.Sprintf("%s://%s:%s@%s:%s/%s?sslmode=%s",
		dataSource.Type, dataSource.User, url.QueryEscape(dataSource.Password), dataSource.Host, dataSource.Port, dataSource.Database, dataSource.SSLMode)
	if dataSource.SSLMode != "" {
		connectionString += fmt.Sprintf("&sslrootcert=%s", url.QueryEscape(dataSource.SSLRootCert))
	}

	connection := &pg.Connection{
		ConnectionString:   connectionString,
		DBName:             dataSource.Database,
		Logger:             logger,
		MaxOpenConnections: constant.PostgresMaxOpenConns,
		MaxIdleConnections: constant.PostgresMaxIdleConns,
	}

	initialStatus := libConstant.DataSourceStatusUnknown

	if !lazy {
		if err := connection.Connect(); err != nil {
			logger.Log(context.Background(), log.LevelError, "Failed to connect to Postgres", log.String("datasource", dataSource.ConfigName), log.Err(err))

			initialStatus = libConstant.DataSourceStatusUnavailable
		} else {
			logger.Log(context.Background(), log.LevelInfo, "Successfully connected to Postgres", log.String("datasource", dataSource.ConfigName), log.Int("max_open_conns", constant.PostgresMaxOpenConns), log.Int("max_idle_conns", constant.PostgresMaxIdleConns))
		}
	}

	return DataSource{
		DatabaseType:        dataSource.Type,
		DatabaseConfig:      connection,
		Initialized:         false,
		Status:              initialStatus,
		LastAttempt:         time.Time{},
		RetryCount:          0,
		Schemas:             dataSource.GetSchemas(),
		MidazOrganizationID: dataSource.MidazOrganizationID,
	}
}

// getDataSourceConfigs retrieves data source configurations from environment variables in the DATASOURCE_[NAME]_* format.
// It validates and returns a slice of DataSourceConfig, logging warnings for incomplete or missing configurations.
func getDataSourceConfigs(logger log.Logger) []DataSourceConfig {
	var dataSources []DataSourceConfig

	dataSourceNames := collectDataSourceNames()

	for name := range dataSourceNames {
		if config, isComplete := buildDataSourceConfig(name, logger); isComplete {
			dataSources = append(dataSources, config)
		}
	}

	if len(dataSources) == 0 {
		logger.Log(context.Background(), log.LevelWarn, "No external data sources found in environment variables. Configure them with DATASOURCE_[NAME]_HOST/PORT/USER/PASSWORD/DATABASE/TYPE/SSLMODE format.")
	}

	return dataSources
}

// collectDataSourceNames identifies all available data source names from environment variables.
func collectDataSourceNames() map[string]bool {
	dataSourceNamesMap := make(map[string]bool)
	prefix := "DATASOURCE_"
	suffix := "_CONFIG_NAME"

	envVars := os.Environ()

	for _, env := range envVars {
		parts := strings.SplitN(env, "=", constant.SplitKeyValueParts)
		if len(parts) != constant.SplitKeyValueParts {
			continue
		}

		key := parts[0]

		if strings.HasPrefix(key, prefix) && strings.HasSuffix(key, suffix) {
			remaining := key[len(prefix) : len(key)-len(suffix)]
			dataSourceNamesMap[strings.ToLower(remaining)] = true
		}
	}

	return dataSourceNamesMap
}

// buildDataSourceConfig creates a DataSourceConfig for the given name, validating all required fields.
// Returns the config and a boolean indicating if the configuration is complete.
func buildDataSourceConfig(name string, logger log.Logger) (DataSourceConfig, bool) {
	dataSource := DataSourceConfig{
		Name:        name,
		ConfigName:  getDataSourceEnv(name, "CONFIG_NAME"),
		Host:        getDataSourceEnv(name, "HOST"),
		Port:        getDataSourceEnv(name, "PORT"),
		User:        getDataSourceEnv(name, "USER"),
		Password:    getDataSourceEnv(name, "PASSWORD"),
		Database:    getDataSourceEnv(name, "DATABASE"),
		Type:        getDataSourceEnv(name, "TYPE"),
		SSLMode:     getDataSourceEnv(name, "SSLMODE"),
		SSLRootCert: getDataSourceEnv(name, "SSLROOTCERT"),
		SSL:         getDataSourceEnv(name, "SSL"),     // For MongoDB SSL
		SSLCA:       getDataSourceEnv(name, "SSLCA"),   // For MongoDB CA file
		Options:     getDataSourceEnv(name, "OPTIONS"), // For MongoDB URI options
		// MidazOrganizationID is deprecated — plugin_crm now discovers all org-scoped
		// collections via prefix matching (ListCollectionNames). Kept for backward compat.
		MidazOrganizationID: getDataSourceEnv(name, "MIDAZ_ORGANIZATION_ID"),
	}

	if dataSource.ConfigName == "" {
		logger.Log(context.Background(), log.LevelWarn, "Datasource has empty CONFIG_NAME - skipping", log.String("datasource", name))
		return dataSource, false
	}

	// Reject incomplete configurations early — Host, Port, Database, and Type are
	// required for any datasource connection to succeed.
	requiredFields := map[string]string{
		"HOST":     dataSource.Host,
		"PORT":     dataSource.Port,
		"DATABASE": dataSource.Database,
		"TYPE":     dataSource.Type,
	}

	for field, value := range requiredFields {
		if value == "" {
			logger.Log(context.Background(), log.LevelWarn, "Datasource missing required field - skipping",
				log.String("datasource", name), log.String("missing_field", field))

			return dataSource, false
		}
	}

	logger.Log(context.Background(), log.LevelInfo, "Found external data source",
		log.String("name", name), log.String("config_name", dataSource.ConfigName),
		log.String("database", dataSource.Database), log.String("type", dataSource.Type),
		log.String("sslmode", dataSource.SSLMode), log.String("ssl", dataSource.SSL), log.String("sslca", dataSource.SSLCA))

	return dataSource, true
}
