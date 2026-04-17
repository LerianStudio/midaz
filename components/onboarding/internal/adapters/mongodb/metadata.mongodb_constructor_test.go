// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
)

func TestNewMetadataMongoDBRepository_WithExistingConnection(t *testing.T) {
	t.Parallel()

	// Build a disconnected mongo.Client just to plug a non-nil DB reference
	// without spinning up a network connection. mongo.NewClient is deprecated
	// but still works for the purposes of this constructor smoke test.
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err)

	mc := &libMongo.MongoConnection{
		DB:        client,
		Database:  "onboarding",
		Connected: true,
	}

	r, err := NewMetadataMongoDBRepository(mc)
	require.NoError(t, err)
	require.NotNil(t, r)
}
