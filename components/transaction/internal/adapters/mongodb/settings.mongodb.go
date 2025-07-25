package mongodb

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
	"reflect"
)

const (
	Database = "settings"
)

// SettingsRepository is an interface for operations related to settings entities.
type SettingsRepository interface {
	Upsert(ctx context.Context, upsert bool, settings *mmodel.Settings) error
	Find(ctx context.Context, organizationID, ledgerID, applicationName string) (*mmodel.Settings, error)
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

	collection := db.Database(Database).Collection(reflect.TypeOf(mmodel.SettingsMongoDBModel{}).Name())

	return &SettingsMongoDBRepository{
		collection: collection,
	}
}

func (smr *SettingsMongoDBRepository) Upsert(ctx context.Context, upsert bool, settings *mmodel.Settings) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_settings")
	defer span.End()

	_, err := smr.collection.UpdateOne(ctx, settings.ToEntity(), options.Update().SetUpsert(upsert))
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to upsert settings", err)

		logger.Errorf("Failed to upsert settings: %v", err)

		return err
	}

	return nil
}

func (smr *SettingsMongoDBRepository) Find(ctx context.Context, organizationID, ledgerID, applicationName string) (*mmodel.Settings, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_settings")
	defer span.End()

	filter := bson.M{"organization_id": organizationID, "ledger_id": ledgerID, "application_name": applicationName}
	record := mmodel.SettingsMongoDBModel{}

	if err := smr.collection.FindOne(ctx, filter).Decode(&record); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find settings", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		logger.Errorf("Failed to find settings: %s", err)

		return nil, err
	}

	return record.ToDTO(), nil
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
