package audit

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmongo"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.mongodb.org/mongo-driver/bson"
	"strings"
)

// Repository provides an interface for operations related on mongodb an audit entities.
//
//go:generate mockgen --destination=audit.mock.go --package=audit . Repository
type Repository interface {
	Create(ctx context.Context, collection string, audit *Audit) error
	FindOne(ctx context.Context, collection string, auditID AuditID) (*Audit, error)
}

// AuditMongoDBRepository is a MongoDD-specific implementation of the AuditRepository.
type AuditMongoDBRepository struct {
	connection *mmongo.MongoConnection
	Database   string
}

// NewAuditMongoDBRepository returns a new instance of AuditMongoDBLRepository using the given MongoDB connection.
func NewAuditMongoDBRepository(mc *mmongo.MongoConnection) *AuditMongoDBRepository {
	r := &AuditMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}
	if _, err := r.connection.GetDB(context.Background()); err != nil {
		panic("Failed to connect mongodb")
	}

	return r
}

func (mar *AuditMongoDBRepository) FindOne(ctx context.Context, collection string, auditID AuditID) (*Audit, error) {

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_audit")
	defer span.End()

	db, err := mar.connection.GetDB(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	logger.Infof("Get connection to %v and collection %v", mar.Database, collection)
	coll := db.Database(strings.ToLower(mar.Database)).Collection(strings.ToLower(collection))

	var record AuditMongoDBModel

	ctx, spanFindOne := tracer.Start(ctx, "mongodb.find_by_audit_id.find_one")

	filter := bson.M{
		"_id.organization_id": auditID.OrganizationID,
		"_id.ledger_id":       auditID.LedgerID,
	}

	if err = coll.FindOne(ctx, filter).Decode(&record); err != nil {
		mopentelemetry.HandleSpanError(&spanFindOne, "Failed to find audit by id", err)

		//if errors.Is(err, mongo.ErrNoDocuments) {
		//	return nil, nil
		//}

		return nil, err
	}

	spanFindOne.End()

	return record.ToEntity(), nil

}

// Create inserts a new audit entity into mongodb
func (mar *AuditMongoDBRepository) Create(ctx context.Context, collection string, audit *Audit) error {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_audit")
	defer span.End()

	db, err := mar.connection.GetDB(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database", err)

		return err
	}

	coll := db.Database(strings.ToLower(mar.Database)).Collection(strings.ToLower(collection))
	record := &AuditMongoDBModel{}

	record.FromEntity(audit)

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_audit.insert")

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanInsert, "Failed to insert audit", err)

		return err
	}

	spanInsert.End()

	return nil
}
