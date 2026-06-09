// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRedis is an in-memory redisRepository capturing the keys it is asked for.
type fakeRedis struct {
	data    map[string]string
	getErr  error
	lastKey string
}

func (f *fakeRedis) Get(_ context.Context, key string) (string, error) {
	f.lastKey = key
	if f.getErr != nil {
		return "", f.getErr
	}

	return f.data[key], nil
}

func (f *fakeRedis) Set(_ context.Context, key, value string, _ time.Duration) error {
	f.lastKey = key
	f.data[key] = value

	return nil
}

func newFakeRedis() *fakeRedis { return &fakeRedis{data: map[string]string{}} }

func TestSchemaCache_TenantScopedKey(t *testing.T) {
	t.Parallel()

	redis := newFakeRedis()
	c := NewSchemaCache(redis, time.Minute)

	snapshot := fetcher.SchemaSnapshot{ConfigName: "ledger"}
	require.NoError(t, c.PutSchema(context.Background(), fetcher.TenantContext{TenantID: "tenant-a"}, snapshot))

	assert.Equal(t, "reporter:engine:schema:tenant-a:ledger", redis.lastKey)

	// A different tenant uses a different key — no cross-tenant serving.
	_, _, _ = c.GetSchema(context.Background(), fetcher.TenantContext{TenantID: "tenant-b"}, "ledger")
	assert.Equal(t, "reporter:engine:schema:tenant-b:ledger", redis.lastKey)
}

func TestSchemaCache_RoundTrip(t *testing.T) {
	t.Parallel()

	redis := newFakeRedis()
	c := NewSchemaCache(redis, time.Minute)
	tenant := fetcher.TenantContext{TenantID: "tenant-a"}

	in := fetcher.SchemaSnapshot{
		ConfigName: "ledger",
		Tables:     []fetcher.TableSnapshot{{Name: "public.accounts", Fields: []string{"id", "balance"}}},
	}
	require.NoError(t, c.PutSchema(context.Background(), tenant, in))

	out, ok, err := c.GetSchema(context.Background(), tenant, "ledger")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "ledger", out.ConfigName)
	require.True(t, out.HasTable("public.accounts"))
}

func TestSchemaCache_RedisErrorIsCacheMiss(t *testing.T) {
	t.Parallel()

	redis := &fakeRedis{data: map[string]string{}, getErr: errors.New("redis down")}
	c := NewSchemaCache(redis, time.Minute)

	_, ok, err := c.GetSchema(context.Background(), fetcher.TenantContext{TenantID: "t"}, "ledger")
	require.NoError(t, err, "a redis outage must degrade to a cache miss, not an error")
	assert.False(t, ok)
}

func TestSchemaCache_CorruptEntryIsCacheMiss(t *testing.T) {
	t.Parallel()

	redis := newFakeRedis()
	redis.data["reporter:engine:schema:t:ledger"] = "not-json"
	c := NewSchemaCache(redis, time.Minute)

	_, ok, err := c.GetSchema(context.Background(), fetcher.TenantContext{TenantID: "t"}, "ledger")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSchemaCache_AbsentEntryIsCacheMiss(t *testing.T) {
	t.Parallel()

	c := NewSchemaCache(newFakeRedis(), time.Minute)

	_, ok, err := c.GetSchema(context.Background(), fetcher.TenantContext{TenantID: "t"}, "ledger")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSchemaCache_PutMarshalsValidJSON(t *testing.T) {
	t.Parallel()

	redis := newFakeRedis()
	c := NewSchemaCache(redis, time.Minute)

	require.NoError(t, c.PutSchema(context.Background(), fetcher.TenantContext{TenantID: "t"}, fetcher.SchemaSnapshot{ConfigName: "ledger"}))

	var decoded fetcher.SchemaSnapshot
	require.NoError(t, json.Unmarshal([]byte(redis.data["reporter:engine:schema:t:ledger"]), &decoded))
	assert.Equal(t, "ledger", decoded.ConfigName)
}
