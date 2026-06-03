// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"errors"
	"sync"
	"time"

	base "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	"github.com/LerianStudio/lib-observability/log"
	mg "go.mongodb.org/mongo-driver/v2/mongo"
)

type MongoConnection struct {
	ConnectionStringSource string
	Database               string
	Logger                 log.Logger
	MaxPoolSize            uint64
	TLS                    *base.TLSConfig

	DB *mg.Client

	mu      sync.Mutex
	once    sync.Once
	initErr error
	client  *base.Client
}

func (c *MongoConnection) GetDB(ctx context.Context) (*mg.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.once.Do(func() {
		if c.DB != nil {
			return
		}

		client, err := base.NewClient(ctx, base.Config{
			URI:         c.ConnectionStringSource,
			Database:    c.Database,
			MaxPoolSize: c.MaxPoolSize,
			TLS:         c.TLS,
			Logger:      c.Logger,
		})
		if err != nil {
			c.initErr = err
			return
		}

		dbClient, err := client.Client(ctx)
		if err != nil {
			c.initErr = err
			return
		}

		c.client = client
		c.DB = dbClient
	})

	if c.initErr != nil {
		return nil, c.initErr
	}

	return c.DB, nil
}

func (c *MongoConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if c.client != nil {
		err := c.client.Close(ctx)
		c.resetState()

		return err
	}

	if c.DB != nil {
		err := c.DB.Disconnect(ctx)
		c.resetState()

		if errors.Is(err, mg.ErrClientDisconnected) {
			return nil
		}

		return err
	}

	c.resetState()

	return nil
}

func (c *MongoConnection) resetState() {
	c.DB = nil
	c.client = nil
	c.initErr = nil
	c.once = sync.Once{}
}
