// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"errors"
	"testing"

	base "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mg "go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoConnection_Close_OnDisconnectedClient_ClearsState(t *testing.T) {
	t.Parallel()

	client, err := mg.Connect()
	require.NoError(t, err)

	conn := &MongoConnection{DB: client}
	err = conn.Close()
	assert.NoError(t, err)
	assert.Nil(t, conn.DB)
}

func TestMongoConnection_Close_OnNilConnection_IsNoOp(t *testing.T) {
	t.Parallel()

	conn := &MongoConnection{}
	err := conn.Close()
	assert.NoError(t, err)
}

func TestMongoConnection_Close_ResetsLifecycleState(t *testing.T) {
	t.Parallel()

	client, err := mg.Connect()
	require.NoError(t, err)

	conn := &MongoConnection{DB: client, initErr: errors.New("stale init error")}
	conn.once.Do(func() {})

	err = conn.Close()
	assert.NoError(t, err)
	assert.Nil(t, conn.DB)
	assert.Nil(t, conn.client)
	assert.NoError(t, conn.initErr)

	called := false
	conn.once.Do(func() { called = true })
	assert.True(t, called)
}

func TestMongoConnection_Close_WithTLS_ResetsState(t *testing.T) {
	t.Parallel()

	client, err := mg.Connect()
	require.NoError(t, err)

	conn := &MongoConnection{
		DB: client,
		TLS: &base.TLSConfig{
			CACertBase64: "dGVzdC1jZXJ0LWRhdGE=",
		},
	}
	err = conn.Close()
	assert.NoError(t, err)
	assert.Nil(t, conn.DB)
	// TLS config is a construction parameter, not runtime state --
	// it is NOT cleared by Close (same as ConnectionStringSource, Database, etc.)
	assert.NotNil(t, conn.TLS)
}
