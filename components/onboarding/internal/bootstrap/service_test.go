// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	httpin "github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/http/in"
)

func TestService_Close_NilReceiver(t *testing.T) {
	t.Parallel()

	var svc *Service
	require.NoError(t, svc.Close())
}

func TestService_Close_EmptyService(t *testing.T) {
	t.Parallel()

	// A Service with nil external connections must Close cleanly and be idempotent.
	svc := &Service{}
	require.NoError(t, svc.Close())
	// Second call returns cached (nil) error without doing work.
	require.NoError(t, svc.Close())
}

func TestService_GetRunnables_ReturnsServer(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	svc := &Service{Server: srv}

	rs := svc.GetRunnables()
	require.Len(t, rs, 1)
	assert.Equal(t, "Onboarding Server", rs[0].Name)
}

func TestService_GetRouteRegistrar_CallsRegister(t *testing.T) {
	t.Parallel()

	// The registrar should not panic when called with zero-value handlers.
	svc := &Service{
		accountHandler:      &httpin.AccountHandler{},
		portfolioHandler:    &httpin.PortfolioHandler{},
		ledgerHandler:       &httpin.LedgerHandler{},
		assetHandler:        &httpin.AssetHandler{},
		organizationHandler: &httpin.OrganizationHandler{},
		segmentHandler:      &httpin.SegmentHandler{},
		accountTypeHandler:  &httpin.AccountTypeHandler{},
	}

	reg := svc.GetRouteRegistrar()
	require.NotNil(t, reg)

	app := fiber.New()
	reg(app)

	// Routes should have been registered even without a configured AuthClient.
	assert.NotEmpty(t, app.GetRoutes())
}

func TestService_GetMetadataIndexPort_NilByDefault(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	assert.Nil(t, svc.GetMetadataIndexPort())
}

func TestService_Close_WithRedisAndPostgresAndTelemetry(t *testing.T) {
	t.Parallel()

	// Build an sqlmock-backed dbresolver so Close can exercise the postgres
	// close branch without opening a real connection.
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()

	resolver := dbresolver.New(dbresolver.WithPrimaryDBs(db), dbresolver.WithReplicaDBs(db))

	pg := &libPostgres.PostgresConnection{ConnectionDB: &resolver}

	// Build a miniature redis client pointed at a non-listening address. Close
	// is graceful in go-redis and simply releases pool resources.
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	redisConn := &libRedis.RedisConnection{Client: redisClient, Connected: true}

	svc := &Service{
		postgresConnection: pg,
		redisConnection:    redisConn,
		// telemetry left nil so ShutdownTelemetry branch is skipped (it requires
		// a fully-initialized provider to avoid nil dereference).
		// mongoConnection left nil so that branch is skipped.
	}

	require.NoError(t, svc.Close())
	// Idempotent: second call should use cached result.
	require.NoError(t, svc.Close())
}
