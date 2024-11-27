package audit

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmongo"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"strings"
)

// Repository provides an interface for operations related on mongodb an audit entities.
//
//go:generate mockgen --destination=audit.mock.go --package=audit . Repository
type Repository interface {
	Create(ctx context.Context, collection string, audit *Audit) error
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

// Create inserts a new audit entity into mongodb
func (mmr *AuditMongoDBRepository) Create(ctx context.Context, collection string, audit *Audit) error {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_audit")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database", err)

		return err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))
	record := &AuditMongoDBModel{}

	record.AuditFromEntity(audit)

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_audit.insert")

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanInsert, "Failed to insert audit", err)

		return err
	}

	spanInsert.End()

	return nil
}
