// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package shared

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	libLog "github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/addons/e2ekit"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/minio"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/postgres"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/rabbitmq"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/itestkit/infra/redis"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// E2EEnv holds all infrastructure and app references for E2E tests.
type E2EEnv struct {
	Suite          *itestkit.Suite
	Mongo          *mongodb.MongoDBInfra // Reporter metadata
	Postgres       *postgres.PostgresInfra
	RabbitMQ       *rabbitmq.RabbitInfra
	Redis          *redis.RedisInfra
	Minio          *minio.MinioInfra     // S3-compatible storage
	PluginCRMMongo *mongodb.MongoDBInfra // plugin_crm datasource (separate instance)
	ManagerApp     *e2ekit.RunningApp
	WorkerApp      *e2ekit.RunningApp
	ManagerBaseURL string
}

// rabbitMQ topology constants for DLQ arguments.
const (
	rabbitDLQMessageTTL = 604800000 // 7 days in milliseconds
	rabbitDLQMaxLength  = 10000
)

// Setup creates all infrastructure, seeds data, and optionally starts Manager and Worker.
// Callers are responsible for calling Teardown when done.
func Setup(ctx context.Context) (*E2EEnv, error) {
	infra := newCoreInfra()

	suite, err := itestkit.New(nil).
		WithInfras(infra.infras()...).
		Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("build suite: %w", err)
	}

	env := &E2EEnv{
		Suite:          suite,
		Mongo:          infra.mongo,
		Postgres:       infra.postgres,
		RabbitMQ:       infra.rabbit,
		Redis:          infra.redis,
		Minio:          infra.minio,
		PluginCRMMongo: infra.pluginCRMMongo,
	}

	// Create RabbitMQ topology programmatically (exchanges, queues, bindings).
	if err := setupRabbitMQTopology(ctx, infra.rabbit); err != nil {
		_ = suite.Terminate(ctx)
		return nil, fmt.Errorf("setup rabbitmq topology: %w", err)
	}

	// Seed plugin_crm MongoDB with test holders data.
	if err := seedPluginCRMMongo(ctx, infra.pluginCRMMongo); err != nil {
		_ = suite.Terminate(ctx)
		return nil, fmt.Errorf("seed plugin_crm mongo: %w", err)
	}

	// If infra-only mode is requested, skip starting apps.
	if isInfraOnly() {
		log.Println("[e2e] E2E_INFRA_ONLY=true: skipping app containers")
		return env, nil
	}

	// Build app environment from infra endpoints.
	appEnv, err := BuildAppEnv(suite.Network(), env)
	if err != nil {
		_ = suite.Terminate(ctx)
		return nil, fmt.Errorf("build app env: %w", err)
	}

	log.Printf("[e2e] S3 Endpoint: %s, Bucket: %s", appEnv.S3Endpoint, appEnv.S3Bucket)

	managerCfg := resolveManagerConfig()
	workerCfg := resolveWorkerConfig()

	// Start Manager unless skipped.
	if !isSkipManager() {
		manager, startErr := StartManager(nil, ctx, appEnv, managerCfg)
		if startErr != nil {
			_ = suite.Terminate(ctx)
			return nil, fmt.Errorf("start manager: %w", startErr)
		}

		env.ManagerApp = manager
		env.ManagerBaseURL = manager.BaseURL
	}

	// Start Worker unless skipped.
	if !isSkipWorker() {
		worker, startErr := StartWorker(nil, ctx, appEnv, workerCfg)
		if startErr != nil {
			if env.ManagerApp != nil {
				_ = env.ManagerApp.Container.Terminate(ctx)
			}

			_ = suite.Terminate(ctx)

			return nil, fmt.Errorf("start worker: %w", startErr)
		}

		env.WorkerApp = worker
	}

	return env, nil
}

// Teardown stops all running apps and infrastructure in reverse order.
func Teardown(ctx context.Context, env *E2EEnv) {
	if env == nil {
		return
	}

	if env.WorkerApp != nil {
		_ = env.WorkerApp.Container.Terminate(ctx)
	}

	if env.ManagerApp != nil {
		_ = env.ManagerApp.Container.Terminate(ctx)
	}

	if env.Suite != nil {
		_ = env.Suite.Terminate(ctx)
	}
}

// coreInfra holds all infrastructure components.
type coreInfra struct {
	mongo          *mongodb.MongoDBInfra
	postgres       *postgres.PostgresInfra
	rabbit         *rabbitmq.RabbitInfra
	redis          *redis.RedisInfra
	minio          *minio.MinioInfra
	pluginCRMMongo *mongodb.MongoDBInfra
}

// newCoreInfra creates all infrastructure components with their configuration.
func newCoreInfra() *coreInfra {
	mongoOpts := []mongodb.MongoDBOption{}
	pgOpts := []postgres.PostgresOption{
		postgres.WithPGInitFile(initPostgresPath(), "init.sql"),
	}
	rabbitOpts := []rabbitmq.RabbitOption{}
	redisOpts := []redis.RedisOption{}
	minioOpts := []minio.MinioOption{}
	pluginCRMMongoOpts := []mongodb.MongoDBOption{}

	if isFixedPortEnabled() {
		mongoOpts = append(mongoOpts, mongodb.WithMongoDBFixedPort("27017"))
		pgOpts = append(pgOpts, postgres.WithPGFixedPort("5432"))
		rabbitOpts = append(rabbitOpts, rabbitmq.WithRabbitFixedPort("5672"))
		redisOpts = append(redisOpts, redis.WithRedisFixedPort("6379"))
		minioOpts = append(minioOpts, minio.WithMinioFixedPort("9000"))
		pluginCRMMongoOpts = append(pluginCRMMongoOpts, mongodb.WithMongoDBFixedPort("27018"))
	}

	return &coreInfra{
		mongo: mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
			Name:     "reporter",
			Username: CoreInfraUsername,
			Password: CoreInfraPassword,
			Options:  mongoOpts,
		}),
		postgres: postgres.NewPostgresInfra(postgres.PostgresConfig{
			Name:     "midaz-onboarding",
			Database: DSMidazOnboarding,
			Username: CoreInfraUsername,
			Password: CoreInfraPassword,
			Options:  pgOpts,
		}),
		rabbit: rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
			Name:     "reporter",
			Username: CoreInfraUsername,
			Password: CoreInfraPassword,
			Options:  rabbitOpts,
		}),
		redis: redis.NewRedisInfra(redis.RedisConfig{
			Name:     "reporter",
			Password: CoreInfraPassword,
			Options:  redisOpts,
		}),
		minio: minio.NewMinioInfra(minio.MinioConfig{
			Name:    "reporter",
			Bucket:  "reporter-storage",
			Options: minioOpts,
		}),
		pluginCRMMongo: mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
			Name:     "plugin-crm",
			Username: CoreInfraUsername,
			Password: PluginCRMPassword,
			Options:  pluginCRMMongoOpts,
		}),
	}
}

// infras returns all components as a slice of itestkit.Infra for the suite builder.
func (c *coreInfra) infras() []itestkit.Infra {
	return []itestkit.Infra{
		c.mongo,
		c.postgres,
		c.rabbit,
		c.redis,
		c.minio,
		c.pluginCRMMongo,
	}
}

// setupRabbitMQTopology creates the exchanges, queues, and bindings that the Reporter
// application expects. Uses amqp091-go to connect directly to the RabbitMQ container.
func setupRabbitMQTopology(ctx context.Context, rabbitInfra *rabbitmq.RabbitInfra) error {
	amqpURL, err := rabbitInfra.AMQPURL()
	if err != nil {
		return fmt.Errorf("get amqp url: %w", err)
	}

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return fmt.Errorf("dial amqp: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	// Declare exchanges.
	if err := ch.ExchangeDeclare(RabbitExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %s: %w", RabbitExchange, err)
	}

	if err := ch.ExchangeDeclare(RabbitDLX, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %s: %w", RabbitDLX, err)
	}

	// Declare main queue with dead-letter routing.
	_, err = ch.QueueDeclare(RabbitQueue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    RabbitDLX,
		"x-dead-letter-routing-key": RabbitDLQRoutingKey,
	})
	if err != nil {
		return fmt.Errorf("declare queue %s: %w", RabbitQueue, err)
	}

	// Declare dead-letter queue with TTL and max length.
	_, err = ch.QueueDeclare(RabbitDLQ, true, false, false, false, amqp.Table{
		"x-message-ttl": int32(rabbitDLQMessageTTL),
		"x-max-length":  int32(rabbitDLQMaxLength),
	})
	if err != nil {
		return fmt.Errorf("declare queue %s: %w", RabbitDLQ, err)
	}

	// Bind main queue to exchange.
	if err := ch.QueueBind(RabbitQueue, RabbitRoutingKey, RabbitExchange, false, nil); err != nil {
		return fmt.Errorf("bind %s -> %s: %w", RabbitExchange, RabbitQueue, err)
	}

	// Bind DLQ to DLX.
	if err := ch.QueueBind(RabbitDLQ, RabbitDLQRoutingKey, RabbitDLX, false, nil); err != nil {
		return fmt.Errorf("bind %s -> %s: %w", RabbitDLX, RabbitDLQ, err)
	}

	_ = ctx // context used for tracing if needed in the future

	return nil
}

// newTestCrypto builds a lib-commons Crypto instance with the E2E test keys and initializes
// the AES-GCM cipher. It panics on failure because seed helpers run during TestMain setup.
func newTestCrypto() *libCrypto.Crypto {
	c := &libCrypto.Crypto{
		HashSecretKey:    TestCryptoHashKey,
		EncryptSecretKey: TestCryptoEncryptKey,
		Logger:           &libLog.NopLogger{},
	}

	if err := c.InitializeCipher(); err != nil {
		panic(fmt.Sprintf("initialize test crypto cipher: %v", err))
	}

	return c
}

// mustEncrypt encrypts plaintext using the provided Crypto instance and returns the ciphertext.
// It panics on error because seed helpers run during TestMain setup where returning errors
// would only add noise — a panic with context is clearer.
func mustEncrypt(c *libCrypto.Crypto, plaintext string) string {
	encrypted, err := c.Encrypt(&plaintext)
	if err != nil {
		panic(fmt.Sprintf("encrypt %q: %v", plaintext, err))
	}

	return *encrypted
}

// generateHash returns the HMAC-SHA256 hash of plaintext for searchable encrypted fields.
func generateHash(c *libCrypto.Crypto, plaintext string) string {
	return c.GenerateHash(&plaintext)
}

// seedPluginCRMMongo connects to the plugin_crm MongoDB and inserts test holder documents.
// PII fields (name, document, mother_name, account) are encrypted with AES-256-GCM and
// searchable hashes are stored in the "search" sub-document, matching the Worker's expected
// storage format. The Worker decrypts these fields at report-generation time.
func seedPluginCRMMongo(ctx context.Context, mongoInfra *mongodb.MongoDBInfra) error {
	endpoint, err := mongoInfra.Endpoint()
	if err != nil {
		return fmt.Errorf("get plugin_crm mongo endpoint: %w", err)
	}

	// Use Upstream (host-accessible address) not HostPort() which returns network alias.
	// Build URI with URL-encoded credentials (password contains @ which breaks URI parsing).
	uri := fmt.Sprintf("mongodb://%s:%s@%s",
		url.QueryEscape(CoreInfraUsername),
		url.QueryEscape(PluginCRMPassword),
		endpoint.Upstream)

	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	clientOpts := options.Client().
		ApplyURI(uri).
		SetConnectTimeout(10 * time.Second).
		SetServerSelectionTimeout(10 * time.Second)

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return fmt.Errorf("connect to plugin_crm mongo: %w", err)
	}
	defer client.Disconnect(ctx)

	if err := client.Ping(connectCtx, nil); err != nil {
		return fmt.Errorf("ping plugin_crm mongo: %w", err)
	}

	crypto := newTestCrypto()

	db := client.Database(DSPluginCRM)

	// Create both the base collection (for Manager schema discovery) and the
	// organization-suffixed collection (for Worker report generation).
	// The Worker constructs collection names as "holders_<midazOrgID>" via
	// processPluginCRMCollection in generate-report-data.go.
	orgCollection := CollectionHolders + "_" + PluginCRMMidazOrgID
	collection := db.Collection(orgCollection)

	// Insert test holder documents.
	// PII fields (name, document, mother_name) are encrypted; search.document holds the hash.
	// Non-PII fields (_id, email, status, type, created_at) remain plaintext.
	holders := []interface{}{
		bson.M{
			"_id":      "holder-001",
			"name":     mustEncrypt(crypto, "Alice Johnson"),
			"document": mustEncrypt(crypto, "12345678901"),
			"email":    "alice@example.com",
			"status":   "active",
			"type":     "NATURAL_PERSON",
			"natural_person": bson.M{
				"mother_name": mustEncrypt(crypto, "Maria Johnson"),
			},
			"search": bson.M{
				"document": generateHash(crypto, "12345678901"),
			},
			"created_at": time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		},
		bson.M{
			"_id":      "holder-002",
			"name":     mustEncrypt(crypto, "Bob Smith Corp"),
			"document": mustEncrypt(crypto, "98765432000100"),
			"email":    "bob@example.com",
			"status":   "active",
			"type":     "LEGAL_PERSON",
			"search": bson.M{
				"document": generateHash(crypto, "98765432000100"),
			},
			"created_at": time.Date(2025, 2, 20, 11, 0, 0, 0, time.UTC),
		},
		bson.M{
			"_id":      "holder-003",
			"name":     mustEncrypt(crypto, "Charlie Brown"),
			"document": mustEncrypt(crypto, "11122233344"),
			"email":    "charlie@example.com",
			"status":   "suspended",
			"type":     "NATURAL_PERSON",
			"natural_person": bson.M{
				"mother_name": mustEncrypt(crypto, "Diana Brown"),
			},
			"search": bson.M{
				"document": generateHash(crypto, "11122233344"),
			},
			"created_at": time.Date(2025, 3, 10, 12, 0, 0, 0, time.UTC),
		},
	}

	_, err = collection.InsertMany(connectCtx, holders)
	if err != nil {
		return fmt.Errorf("insert holders into plugin_crm: %w", err)
	}

	// Seed aliases collection for ACCS005 template.
	// Links holders to accounts via document and account_id, with banking details.
	// PII fields (document, banking_details.account) are encrypted.
	aliasCollection := CollectionAliases + "_" + PluginCRMMidazOrgID
	aliases := []interface{}{
		bson.M{
			"_id":        "alias-001",
			"account_id": "c0000000-0000-0000-0000-000000000001", // Operating Account (deposit)
			"document":   mustEncrypt(crypto, "12345678901"),     // matches holder-001 (Alice)
			"banking_details": bson.M{
				"branch":       "0001",
				"type":         "CACC",
				"account":      mustEncrypt(crypto, "123456"),
				"opening_date": "2025-01-25",
			},
			"search": bson.M{
				"document": generateHash(crypto, "12345678901"),
			},
		},
		bson.M{
			"_id":        "alias-002",
			"account_id": "c0000000-0000-0000-0000-000000000002", // Savings Account (savings)
			"document":   mustEncrypt(crypto, "11122233344"),     // matches holder-003 (Charlie)
			"banking_details": bson.M{
				"branch":       "0002",
				"type":         "CACC",
				"account":      mustEncrypt(crypto, "654321"),
				"opening_date": "2025-02-05",
			},
			"search": bson.M{
				"document": generateHash(crypto, "11122233344"),
			},
		},
	}

	_, err = db.Collection(aliasCollection).InsertMany(connectCtx, aliases)
	if err != nil {
		return fmt.Errorf("insert aliases into plugin_crm: %w", err)
	}

	return nil
}

// initPostgresPath returns the absolute path to the PostgreSQL init SQL file.
func initPostgresPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("tests", "e2e", "testdata", "init_postgres.sql")
	}

	return filepath.Join(filepath.Dir(file), "..", "testdata", "init_postgres.sql")
}

// Environment variable helpers.

func isFixedPortEnabled() bool {
	return strings.EqualFold(os.Getenv("FIXED_PORT"), "true")
}

func isInfraOnly() bool {
	return strings.EqualFold(os.Getenv("E2E_INFRA_ONLY"), "true")
}

func isSkipManager() bool {
	return strings.EqualFold(os.Getenv("E2E_SKIP_MANAGER"), "true")
}

func isSkipWorker() bool {
	return strings.EqualFold(os.Getenv("E2E_SKIP_WORKER"), "true")
}
