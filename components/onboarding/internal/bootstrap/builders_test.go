// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// noopLogger satisfies libLog.Logger without pulling in zap. It's enough for the
// builders under test because they only store the logger reference.
type noopLogger struct{}

func (noopLogger) Info(_ ...any)                     {}
func (noopLogger) Infof(_ string, _ ...any)          {}
func (noopLogger) Infoln(_ ...any)                   {}
func (noopLogger) Error(_ ...any)                    {}
func (noopLogger) Errorf(_ string, _ ...any)         {}
func (noopLogger) Errorln(_ ...any)                  {}
func (noopLogger) Warn(_ ...any)                     {}
func (noopLogger) Warnf(_ string, _ ...any)          {}
func (noopLogger) Warnln(_ ...any)                   {}
func (noopLogger) Debug(_ ...any)                    {}
func (noopLogger) Debugf(_ string, _ ...any)         {}
func (noopLogger) Debugln(_ ...any)                  {}
func (noopLogger) Fatal(_ ...any)                    {}
func (noopLogger) Fatalf(_ string, _ ...any)         {}
func (noopLogger) Fatalln(_ ...any)                  {}
func (noopLogger) WithFields(_ ...any) libLog.Logger { return noopLogger{} }
func (noopLogger) WithDefaultMessageTemplate(_ string) libLog.Logger {
	return noopLogger{}
}
func (noopLogger) Sync() error { return nil }

var _ libLog.Logger = noopLogger{}

func TestBuildPostgresConnection_UsesPrefixedFields(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		PrefixedPrimaryDBHost:      "pg-primary",
		PrefixedPrimaryDBUser:      "admin",
		PrefixedPrimaryDBPassword:  "secret",
		PrefixedPrimaryDBName:      "db",
		PrefixedPrimaryDBPort:      "5432",
		PrefixedPrimaryDBSSLMode:   "disable",
		PrefixedReplicaDBHost:      "pg-replica",
		PrefixedReplicaDBPort:      "5433",
		PrefixedMaxOpenConnections: 10,
		PrefixedMaxIdleConnections: 5,
	}

	conn := buildPostgresConnection(cfg, noopLogger{})

	require.NotNil(t, conn)
	assert.Contains(t, conn.ConnectionStringPrimary, "host=pg-primary")
	assert.Contains(t, conn.ConnectionStringPrimary, "user=admin")
	assert.Contains(t, conn.ConnectionStringPrimary, "dbname=db")
	assert.Contains(t, conn.ConnectionStringReplica, "host=pg-replica")
	assert.Equal(t, "db", conn.PrimaryDBName)
	assert.Equal(t, 10, conn.MaxOpenConnections)
	assert.Equal(t, 5, conn.MaxIdleConnections)
}

func TestBuildPostgresConnection_FallsBackToNonPrefixed(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		PrimaryDBHost:      "legacy-host",
		PrimaryDBName:      "legacydb",
		PrimaryDBPort:      "5432",
		ReplicaDBHost:      "legacy-replica",
		MaxOpenConnections: 20,
	}

	conn := buildPostgresConnection(cfg, noopLogger{})

	require.NotNil(t, conn)
	assert.Contains(t, conn.ConnectionStringPrimary, "host=legacy-host")
	assert.Equal(t, "legacydb", conn.PrimaryDBName)
	assert.Equal(t, 20, conn.MaxOpenConnections)
}

func TestBuildMongoConnection_UsesPrefixedFields(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		PrefixedMongoURI:    "mongodb",
		PrefixedMongoDBHost: "mongo-host",
		PrefixedMongoDBName: "mongo-db",
		PrefixedMongoDBUser: "mongo-user",
		PrefixedMongoDBPort: "27017",
		PrefixedMaxPoolSize: 50,
	}

	conn := buildMongoConnection(cfg, noopLogger{})

	require.NotNil(t, conn)
	assert.Equal(t, "mongo-db", conn.Database)
	assert.Equal(t, uint64(50), conn.MaxPoolSize)
	assert.Contains(t, conn.ConnectionStringSource, "mongo-host")
}

func TestBuildMongoConnection_DefaultPoolSize(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		MongoURI:    "mongodb",
		MongoDBHost: "host",
		MongoDBName: "db",
		MongoDBPort: "27017",
	}

	conn := buildMongoConnection(cfg, noopLogger{})
	require.NotNil(t, conn)
	// Default when pool size is 0 must be 100.
	assert.Equal(t, uint64(100), conn.MaxPoolSize)
}

func TestBuildRedisConnection_MapsTimeoutsCorrectly(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		RedisHost:                    "a,b,c",
		RedisPassword:                "pwd",
		RedisDB:                      2,
		RedisProtocol:                3,
		RedisMasterName:              "master",
		RedisTLS:                     true,
		RedisCACert:                  "cert",
		RedisUseGCPIAM:               true,
		RedisServiceAccount:          "sa",
		GoogleApplicationCredentials: "creds",
		RedisTokenLifeTime:           30,
		RedisTokenRefreshDuration:    5,
		RedisPoolSize:                10,
		RedisMinIdleConns:            1,
		RedisReadTimeout:             5,
		RedisWriteTimeout:            5,
		RedisDialTimeout:             2,
		RedisPoolTimeout:             4,
		RedisMaxRetries:              3,
		RedisMinRetryBackoff:         100,
		RedisMaxRetryBackoff:         1,
	}

	conn := buildRedisConnection(cfg, noopLogger{})

	require.NotNil(t, conn)
	assert.Equal(t, []string{"a", "b", "c"}, conn.Address)
	assert.Equal(t, "pwd", conn.Password)
	assert.Equal(t, 2, conn.DB)
	assert.True(t, conn.UseTLS)
	assert.Equal(t, 30*time.Minute, conn.TokenLifeTime)
	assert.Equal(t, 5*time.Second, conn.ReadTimeout)
	assert.Equal(t, 100*time.Millisecond, conn.MinRetryBackoff)
	assert.Equal(t, 1*time.Second, conn.MaxRetryBackoff)
	assert.Equal(t, 3, conn.MaxRetries)
}

func TestResolveLogger_UsesProvidedLogger(t *testing.T) {
	t.Parallel()

	provided := noopLogger{}
	opts := &Options{Logger: provided}

	got, err := resolveLogger(opts)
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestResolveLogger_BuildsDefaultWhenNil(t *testing.T) {
	t.Parallel()

	got, err := resolveLogger(nil)
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestResolveBalancePort_UnifiedModeWithoutPort(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	opts := &Options{UnifiedMode: true}

	_, err := resolveBalancePort(cfg, opts, noopLogger{})
	require.ErrorIs(t, err, ErrUnifiedModeRequiresBalancePort)
}

func TestResolveBalancePort_NonUnifiedDefaultsAddressAndPort(t *testing.T) {
	t.Parallel()

	// Non-unified mode creates a gRPC adapter. Passing an unresolvable address
	// still creates the adapter (the lazy dial happens on first call), which
	// lets us exercise the "set defaults + build adapter" branch.
	cfg := &Config{}
	opts := &Options{UnifiedMode: false}

	port, err := resolveBalancePort(cfg, opts, noopLogger{})
	require.NoError(t, err)
	require.NotNil(t, port)
	// Defaults should have been applied.
	require.Equal(t, "midaz-transaction", cfg.TransactionGRPCAddress)
	require.Equal(t, "3011", cfg.TransactionGRPCPort)
}

func TestResolveBalancePort_NonUnifiedPreservesExplicitAddress(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		TransactionGRPCAddress: "custom-host",
		TransactionGRPCPort:    "9090",
	}

	port, err := resolveBalancePort(cfg, &Options{UnifiedMode: false}, noopLogger{})
	require.NoError(t, err)
	require.NotNil(t, port)
	require.Equal(t, "custom-host", cfg.TransactionGRPCAddress)
	require.Equal(t, "9090", cfg.TransactionGRPCPort)
}

func TestNewServer(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		PrefixedServerAddress: ":8080",
	}

	tele := &libOpentelemetry.Telemetry{}
	srv := NewServer(cfg, fiber.New(), noopLogger{}, tele)

	require.NotNil(t, srv)
	assert.Equal(t, ":8080", srv.ServerAddress())
}

func TestNewServer_FallsBackToNonPrefixed(t *testing.T) {
	t.Parallel()

	cfg := &Config{ServerAddress: ":9090"}

	tele := &libOpentelemetry.Telemetry{}
	srv := NewServer(cfg, fiber.New(), noopLogger{}, tele)

	require.NotNil(t, srv)
	assert.Equal(t, ":9090", srv.ServerAddress())
}
