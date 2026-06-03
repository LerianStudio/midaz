// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"
	"strings"

	tmCore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ResolveDatabase returns the tenant-scoped Mongo database when present.
// If a tenant ID exists in context but no tenant database has been injected,
// it fails closed with ErrTenantContextRequired instead of silently falling
// back to the shared static connection.
//
// When no tenant ID is present, the static single-tenant connection is used.
func ResolveDatabase(ctx context.Context, conn *MongoConnection, database string) (*mongo.Database, error) {
	if tenantDB := tmCore.GetMBContext(ctx); tenantDB != nil {
		return tenantDB, nil
	}

	if tmCore.GetTenantIDContext(ctx) != "" {
		return nil, tmCore.ErrTenantContextRequired
	}

	if conn == nil {
		return nil, fmt.Errorf("mongodb connection is nil and no tenant database in context")
	}

	client, err := conn.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	return client.Database(strings.ToLower(database)), nil
}
