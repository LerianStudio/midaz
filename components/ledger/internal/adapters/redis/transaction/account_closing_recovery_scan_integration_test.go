//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"strconv"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// accountClosingScanFixture is one tenant-scoped recovery hash under test.
type accountClosingScanFixture struct {
	ctx       context.Context
	repo      *RedisConsumerRepository
	client    goredis.UniversalClient
	legacyKey string
	engineKey string
}

func newAccountClosingScanFixture(t *testing.T, client goredis.UniversalClient) accountClosingScanFixture {
	t.Helper()

	ctx := tmcore.ContextWithTenantID(t.Context(), "closing-scan-"+uuid.NewString())

	legacyKey, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	require.NoError(t, err)

	engineKey, err := tenantKeyFromContextOrError(ctx, cachepolicy.EngineRecoverQueue)
	require.NoError(t, err)

	repo, err := NewConsumerRedis(&recoveryAckClient{client: client})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, client.Del(context.Background(), legacyKey, engineKey).Err()) })

	return accountClosingScanFixture{ctx: ctx, repo: repo, client: client, legacyKey: legacyKey, engineKey: engineKey}
}

// closingScanPayload builds a record payload of a realistic size. A recovery
// envelope is far larger than the listpack threshold, so the hash under test is
// stored the way a real one is and HSCAN answers in several pages.
func closingScanPayload(index int) string {
	return `{"formatVersion":2,"index":` + strconv.Itoa(index) + `,"padding":"` + strings.Repeat("x", 96) + `"}`
}

// walk reads one origin to its terminal cursor and returns every record it saw.
func (f accountClosingScanFixture) walk(t *testing.T, source RecoveryQueueSource, count int64) map[string]string {
	t.Helper()

	seen := make(map[string]string)

	for cursor, pages := uint64(0), 0; ; pages++ {
		require.Less(t, pages, 1000, "the walk must terminate")

		page, err := f.repo.ScanRecoveryMessages(f.ctx, source, cursor, count)
		require.NoError(t, err)
		require.Equal(t, source, page.Source)

		for _, record := range page.Records {
			seen[record.Field] = record.Payload
		}

		cursor = page.Cursor
		if page.Complete() {
			return seen
		}
	}
}

// TestIntegration_AccountClosingRecoveryScanWalksEveryCursor proves a bounded
// cursor walk returns every persisted record of its own hash, that a single page
// is not the walk, and that the two origins keep independent cursors even when
// they carry the same field.
func TestIntegration_AccountClosingRecoveryScanWalksEveryCursor(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newAccountClosingScanFixture(t, container.Client)

	const records = 250

	expected := make(map[string]string, records)

	for index := range records {
		field := "11111111-1111-4111-8111-" + strconv.Itoa(1_000_000_000_000 + index)[1:]
		payload := closingScanPayload(index)
		expected[field] = payload

		require.NoError(t, f.client.HSet(f.ctx, f.legacyKey, field, payload).Err())
	}

	shared := "22222222-2222-4222-8222-222222222222:33333333-3333-4333-8333-333333333333"
	require.NoError(t, f.client.HSet(f.ctx, f.engineKey, shared, `{"formatVersion":2,"origin":"engine"}`).Err())

	legacy := f.walk(t, RecoveryQueueSourceLegacyBackup, 10)
	assert.Equal(t, expected, legacy, "the walk must return every record of its own hash")

	engine := f.walk(t, RecoveryQueueSourceEngineRecover, 10)
	assert.Equal(t, map[string]string{shared: `{"formatVersion":2,"origin":"engine"}`}, engine,
		"each origin is walked on its own; records are never merged by field")

	assert.Equal(t, int64(records), f.client.HLen(f.ctx, f.legacyKey).Val(), "the scan must not remove a record")
}

// TestIntegration_AccountClosingRecoveryScanSurvivesAConcurrentAcknowledgement
// proves a record acknowledged while the walk is in flight simply stops being
// returned: the scan never resurrects it, never fails on it and never
// acknowledges anything itself.
func TestIntegration_AccountClosingRecoveryScanSurvivesAConcurrentAcknowledgement(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newAccountClosingScanFixture(t, container.Client)

	const records = 300

	fields := make([]string, 0, records)

	for index := range records {
		field := "44444444-4444-4444-8444-" + strconv.Itoa(1_000_000_000_000 + index)[1:]
		fields = append(fields, field)

		require.NoError(t, f.client.HSet(f.ctx, f.legacyKey, field, closingScanPayload(index)).Err())
	}

	page, err := f.repo.ScanRecoveryMessages(f.ctx, RecoveryQueueSourceLegacyBackup, 0, 10)
	require.NoError(t, err)
	require.False(t, page.Complete(), "the fixture must need more than one page")

	// The acknowledgment happens between two pages, exactly as the completer would
	// do it while a closing is walking the hash.
	acknowledged := fields[len(fields)-1]
	require.NoError(t, f.client.HDel(f.ctx, f.legacyKey, acknowledged).Err())

	seen := map[string]string{}
	for _, record := range page.Records {
		seen[record.Field] = record.Payload
	}

	for cursor := page.Cursor; cursor != 0; {
		next, err := f.repo.ScanRecoveryMessages(f.ctx, RecoveryQueueSourceLegacyBackup, cursor, 10)
		require.NoError(t, err)

		for _, record := range next.Records {
			seen[record.Field] = record.Payload
		}

		cursor = next.Cursor
	}

	assert.NotContains(t, seen, acknowledged, "an acknowledged record must not come back")
	assert.Equal(t, int64(records-1), f.client.HLen(f.ctx, f.legacyKey).Val())
}

// TestIntegration_AccountClosingRecoveryScanIsTenantScoped proves the scan reads
// only the hash of its own tenant, so a closing in one tenant never sees another
// tenant's recovery evidence.
func TestIntegration_AccountClosingRecoveryScanIsTenantScoped(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	first := newAccountClosingScanFixture(t, container.Client)
	second := newAccountClosingScanFixture(t, container.Client)

	require.NoError(t, first.client.HSet(first.ctx, first.legacyKey, "field-a", "payload-a").Err())
	require.NoError(t, second.client.HSet(second.ctx, second.legacyKey, "field-b", "payload-b").Err())

	assert.Equal(t, map[string]string{"field-a": "payload-a"}, first.walk(t, RecoveryQueueSourceLegacyBackup, 100))
	assert.Equal(t, map[string]string{"field-b": "payload-b"}, second.walk(t, RecoveryQueueSourceLegacyBackup, 100))
}

// TestIntegration_AccountClosingRecoveryScanRejectsAnUnknownOrigin proves the
// origin stays a closed set: a scan cannot be pointed at an arbitrary key.
func TestIntegration_AccountClosingRecoveryScanRejectsAnUnknownOrigin(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	f := newAccountClosingScanFixture(t, container.Client)

	_, err := f.repo.ScanRecoveryMessages(f.ctx, RecoveryQueueSource("arbitrary-key"), 0, 10)
	require.Error(t, err)
}
