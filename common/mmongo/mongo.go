package mmongo

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// MongoConnection is a hub which deal with mongodb connections.
type MongoConnection struct {
	ConnectionStringSource string
	DB                     *mongo.Client
	Connected              bool
	Database               string
}

// Connect keeps a singleton connection with postgres.
func (mc *MongoConnection) Connect(ctx context.Context) error {
	fmt.Println("Connecting to mongodb...")

	clientOptions := options.Client().ApplyURI(mc.ConnectionStringSource)

	noSQLDB, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatal("failed to open connect to mongodb", zap.Error(err))
		return nil
	}

	if err := noSQLDB.Ping(ctx, nil); err != nil {
		log.Printf("MongoDBConnection.Ping %v",
			zap.Error(err))

		return err
	}

	fmt.Println("Connected to mongodb ✅ ")

	mc.Connected = true

	mc.DB = noSQLDB

	return nil
}

// GetDB returns a pointer to the mongodb connection, initializing it if necessary.
func (mc *MongoConnection) GetDB(ctx context.Context) (*mongo.Client, error) {
	if mc.DB == nil {
		err := mc.Connect(ctx)
		if err != nil {
			log.Printf("ERRCONECT %s", err)
			return nil, err
		}
	}

	return mc.DB, nil
}
