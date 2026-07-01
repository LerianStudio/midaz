// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"sync"

	base "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	libLog "github.com/LerianStudio/lib-observability/log"
	mg "go.mongodb.org/mongo-driver/v2/mongo"
)

// MongoConnection wraps the mongo-driver v2 client (via lib-commons v5) while
// preserving the MongoConnection field-level API used throughout the codebase.
type MongoConnection struct {
	ConnectionStringSource string
	Database               string
	Logger                 libLog.Logger
	MaxPoolSize            uint64
	// TLSCACert is an optional base64-encoded PEM CA certificate for TLS
	// validation. When set, it is passed to lib-commons/mongo as TLSConfig.
	// When empty, TLS is not configured (connection relies on URI parameters).
	TLSCACert string

	// DB allows tests to inject a pre-connected *mongo.Client directly.
	DB *mg.Client

	mu     sync.Mutex
	client *base.Client
}

// GetDB lazily initialises the underlying connection via base.NewClient and
// returns the raw *mongo.Client. Repeated calls reuse the same connection once
// it is established successfully.
//
// A failed initialisation is NOT cached: if base.NewClient returns an error the
// next GetDB call will retry, so a transient startup failure (e.g. a brief Mongo
// outage or a cancelled context) does not permanently poison the connection.
//
// If DB was set directly (e.g. by tests), it is returned immediately.
func (c *MongoConnection) GetDB(ctx context.Context) (*mg.Client, error) {
	if c.DB != nil {
		return c.DB, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Fast path: connection already established.
	if c.client != nil {
		return c.client.Client(ctx)
	}

	// Slow path: attempt to initialise. Only cache on success so that a transient
	// failure (network blip, cancelled context at startup) allows future retries.
	cfg := base.Config{
		URI:         c.ConnectionStringSource,
		Database:    c.Database,
		MaxPoolSize: c.MaxPoolSize,
		Logger:      c.Logger,
	}

	if c.TLSCACert != "" {
		cfg.TLS = &base.TLSConfig{CACertBase64: c.TLSCACert}
	}

	client, err := base.NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	c.client = client

	return c.client.Client(ctx)
}

// Close gracefully shuts down the underlying connection, if one was created.
func (c *MongoConnection) Close(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		return c.client.Close(ctx)
	}

	return nil
}
