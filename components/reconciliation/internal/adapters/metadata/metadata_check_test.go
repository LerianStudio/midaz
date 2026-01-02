package metadata

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestMetadataChecker_checkOutboxPublished_UsesOnboardingMongoForOnboardingTypes(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Return 2 entries: 1 onboarding (ledger) and 1 transaction (transaction).
	rows := sqlmock.NewRows([]string{"entity_id", "entity_type", "total_count"}).
		AddRow("ldg-1", "ledger", 2).
		AddRow("txn-1", "transaction", 2)

	mock.ExpectQuery(regexp.QuoteMeta("FROM metadata_outbox")).
		WillReturnRows(rows)

	onbMongo := &mongo.Database{}
	txnMongo := &mongo.Database{}

	var (
		onboardingLookups  int
		transactionLookups int
	)

	c := &MetadataChecker{
		db:               db,
		onboardingMongo:  onbMongo,
		transactionMongo: txnMongo,
		findIDs: func(ctx context.Context, db *mongo.Database, collection string, ids []string) (map[string]struct{}, error) {
			switch db {
			case onbMongo:
				onboardingLookups++
				assert.Equal(t, "ledger", collection)
				return map[string]struct{}{"ldg-1": {}}, nil
			case txnMongo:
				transactionLookups++
				assert.Equal(t, "transaction", collection)
				// Simulate missing txn-1 in Mongo.
				return map[string]struct{}{}, nil
			default:
				t.Fatalf("unexpected mongo db pointer")
				return nil, nil
			}
		},
	}

	res, err := c.checkOutboxPublished(context.Background(), 7, 5000, 10)
	require.NoError(t, err)

	assert.Equal(t, 2, res.ScannedCount)
	assert.Equal(t, 2, res.ValidatedCount)
	assert.Equal(t, 1, res.FoundCount)
	assert.Equal(t, 1, res.MissingCount)
	assert.Len(t, res.MissingEntities, 1)
	assert.Equal(t, "txn-1", res.MissingEntities[0].EntityID)
	assert.Equal(t, "transaction", res.MissingEntities[0].EntityType)

	assert.Equal(t, 1, onboardingLookups)
	assert.Equal(t, 1, transactionLookups)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataChecker_checkOutboxPublished_SkipsTypesWithNoConfiguredMongo(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Only onboarding type present, but onboarding mongo is nil.
	rows := sqlmock.NewRows([]string{"entity_id", "entity_type", "total_count"}).
		AddRow("ldg-1", "ledger", 1)

	mock.ExpectQuery(regexp.QuoteMeta("FROM metadata_outbox")).
		WillReturnRows(rows)

	txnMongo := &mongo.Database{}

	c := &MetadataChecker{
		db:               db,
		onboardingMongo:  nil,
		transactionMongo: txnMongo,
		findIDs: func(ctx context.Context, db *mongo.Database, collection string, ids []string) (map[string]struct{}, error) {
			t.Fatalf("findIDs should not be called when no suitable MongoDB is configured")
			return nil, nil
		},
	}

	res, err := c.checkOutboxPublished(context.Background(), 7, 5000, 10)
	require.NoError(t, err)

	assert.Equal(t, 1, res.ScannedCount)
	assert.Equal(t, 0, res.ValidatedCount)
	assert.Equal(t, 0, res.FoundCount)
	assert.Equal(t, 0, res.MissingCount)
	assert.Empty(t, res.MissingEntities)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataChecker_checkOutboxPublished_SkipsUnsupportedEntityTypes(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	rows := sqlmock.NewRows([]string{"entity_id", "entity_type", "total_count"}).
		AddRow("weird-1", "new_entity_type", 1)

	mock.ExpectQuery(regexp.QuoteMeta("FROM metadata_outbox")).
		WillReturnRows(rows)

	c := &MetadataChecker{
		db:               db,
		onboardingMongo:  &mongo.Database{},
		transactionMongo: &mongo.Database{},
		findIDs: func(ctx context.Context, db *mongo.Database, collection string, ids []string) (map[string]struct{}, error) {
			t.Fatalf("findIDs should not be called for unsupported entity types")
			return nil, nil
		},
	}

	res, err := c.checkOutboxPublished(context.Background(), 7, 5000, 10)
	require.NoError(t, err)

	assert.Equal(t, 1, res.ScannedCount)
	assert.Equal(t, 0, res.ValidatedCount)
	assert.Equal(t, 0, res.FoundCount)
	assert.Equal(t, 0, res.MissingCount)
	assert.Empty(t, res.MissingEntities)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataChecker_checkOutboxPublished_ChecksRouteCollectionAliases(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	rows := sqlmock.NewRows([]string{"entity_id", "entity_type", "total_count"}).
		AddRow("tr-1", "transaction_route", 1)

	mock.ExpectQuery(regexp.QuoteMeta("FROM metadata_outbox")).
		WillReturnRows(rows)

	txnMongo := &mongo.Database{}
	collections := map[string]int{}

	c := &MetadataChecker{
		db:               db,
		onboardingMongo:  &mongo.Database{},
		transactionMongo: txnMongo,
		findIDs: func(ctx context.Context, db *mongo.Database, collection string, ids []string) (map[string]struct{}, error) {
			require.Same(t, txnMongo, db)
			collections[collection]++
			// Only return found for one of the aliases.
			if collection == "transaction_route" {
				return map[string]struct{}{"tr-1": {}}, nil
			}
			return map[string]struct{}{}, nil
		},
	}

	res, err := c.checkOutboxPublished(context.Background(), 7, 5000, 10)
	require.NoError(t, err)

	assert.Equal(t, 1, res.ValidatedCount)
	assert.Equal(t, 1, res.FoundCount)
	assert.Equal(t, 0, res.MissingCount)
	assert.Equal(t, 1, collections["transactionroute"])
	assert.Equal(t, 1, collections["transaction_route"])
	assert.NoError(t, mock.ExpectationsWereMet())
}
