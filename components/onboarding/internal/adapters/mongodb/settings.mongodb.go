package mongodb

//go:generate mockgen --destination=settings.mongodb_mock.go --package=mongodb . SettingsRepository

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libMongo "github.com/LerianStudio/lib-commons/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

const (
	Database = "settings"
	Table    = "settings"
)

// SettingsRepository is an interface for operations related to settings entities.
type SettingsRepository interface {
	Create(ctx context.Context, settings *mmodel.Settings) error
	Update(ctx context.Context, settings *mmodel.Settings) error
	FindAll(ctx context.Context, organizationID, ledgerID string) ([]*mmodel.Settings, error)
	Delete(ctx context.Context, organizationID, ledgerID, applicationName string) error
}

// SettingsMongoDBRepository is a MongoDD-specific implementation of the SettingsRepository.
type SettingsMongoDBRepository struct {
	collection *mongo.Collection
}

// NewSettingsMongoDBRepository returns a collection of SettingsMongoDBRepository using the given MongoDB connection.
func NewSettingsMongoDBRepository(mc *libMongo.MongoConnection) *SettingsMongoDBRepository {
	db, err := mc.GetDB(context.Background())
	if err != nil {
		panic("Failed to connect mongodb")
	}

	collection := db.Database(Database).Collection(Table)

	return &SettingsMongoDBRepository{
		collection: collection,
	}
}

func (smr *SettingsMongoDBRepository) Create(ctx context.Context, settings *mmodel.Settings) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_settings")
	defer span.End()

	_, err := smr.collection.InsertOne(ctx, settings.ToEntity())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to upsert settings", err)

		logger.Errorf("Failed to upsert settings: %v", err)

		return err
	}

	return nil
}

func (smr *SettingsMongoDBRepository) Update(ctx context.Context, settings *mmodel.Settings) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_settings")
	defer span.End()

	filter := bson.M{"organization_id": settings.OrganizationID, "ledger_id": settings.LedgerID, "application_name": settings.ApplicationName}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "settings", Value: settings.Settings}, {Key: "updated_at", Value: time.Now()}}}}

	updated, err := smr.collection.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update settings", err)

		logger.Errorf("Failed to update settings: %v", err)

		return err
	}

	if updated.ModifiedCount > 0 {
		logger.Infoln("updated a settings with entity_id: ")
	}

	return nil
}

func (smr *SettingsMongoDBRepository) FindAll(ctx context.Context, organizationID, ledgerID string) ([]*mmodel.Settings, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_settings")
	defer span.End()

	filter := bson.M{"organization_id": organizationID, "ledger_id": ledgerID}

	cur, err := smr.collection.Find(ctx, filter, options.Find())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find settings", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		logger.Errorf("Failed to find settings: %s", err)

		return nil, err
	}

	settings := make([]*mmodel.Settings, 0)
	for cur.Next(ctx) {
		var record mmodel.SettingsMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode metadata", err)

			logger.Errorf("Failed to decode metadata: %v", err)

			return nil, err
		}

		settings = append(settings, record.ToDTO())
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate metadata", err)

		logger.Errorf("Failed to iterate metadata: %v", err)

		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to close cursor", err)

		logger.Errorf("Failed to close cursor: %v", err)

		return nil, err
	}

	return settings, nil
}

func (smr *SettingsMongoDBRepository) Delete(ctx context.Context, organizationID, ledgerID, applicationName string) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_settings")
	defer span.End()

	filter := bson.M{"organization_id": organizationID, "ledger_id": ledgerID, "application_name": applicationName}

	deleted, err := smr.collection.DeleteOne(ctx, filter, options.Delete())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete settings", err)

		logger.Errorf("Failed to delete settings: %s", err)

		return err
	}

	if deleted.DeletedCount > 0 {
		logger.Infof("total deleted a document with success: %v", deleted.DeletedCount)
	}

	return nil
}
