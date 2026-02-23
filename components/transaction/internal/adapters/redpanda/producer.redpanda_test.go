// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

type testStringer struct{}

func (testStringer) String() string {
	return "stringer-value"
}

func TestNewProducerRedpandaWithSecurity_RequiresBrokers(t *testing.T) {
	repo, err := NewProducerRedpandaWithSecurity(nil, 0, 0, true, ClientSecurityConfig{})
	assert.Nil(t, repo)
	require.Error(t, err)
	assert.ErrorContains(t, err, "at least one redpanda broker is required")
}

func TestProducerRedpandaRepository_CheckHealth_NilReceiver(t *testing.T) {
	var repo *ProducerRedpandaRepository
	assert.False(t, repo.CheckHealth())
}

func TestProducerRedpandaRepository_CheckHealth_TransactionSyncReturnsTrue(t *testing.T) {
	repo := &ProducerRedpandaRepository{client: new(kgo.Client), transactionAsync: false}
	assert.True(t, repo.CheckHealth())
}

func TestProducerRedpandaRepository_CheckHealth_AsyncPingFailure(t *testing.T) {
	repo, err := NewProducerRedpanda([]string{"127.0.0.1:1"}, 0, 0, true)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, repo.Close())
	})

	assert.False(t, repo.CheckHealth())
}

func TestProducerRedpandaRepository_ProducerDefault_NilReceiver(t *testing.T) {
	var repo *ProducerRedpandaRepository

	_, err := repo.ProducerDefault(context.Background(), "topic", "key", []byte("payload"))
	assert.Error(t, err)
}

func TestProducerRedpandaRepository_ProducerDefaultWithContext_NilReceiver(t *testing.T) {
	var repo *ProducerRedpandaRepository

	_, err := repo.ProducerDefaultWithContext(context.Background(), "topic", "key", []byte("payload"))
	assert.Error(t, err)
}

func TestToHeaderBytes(t *testing.T) {
	assert.Equal(t, []byte("abc"), toHeaderBytes("abc"))
	assert.Equal(t, []byte("42"), toHeaderBytes(42))
	assert.Equal(t, []byte("raw"), toHeaderBytes([]byte("raw")))
	assert.Equal(t, []byte("stringer-value"), toHeaderBytes(testStringer{}))
	assert.Equal(t, []byte("<nil>"), toHeaderBytes(nil))
}

func TestBuildRecordHeaders(t *testing.T) {
	headers := buildRecordHeaders(map[string]any{
		"a": "1",
		"b": fmt.Stringer(testStringer{}),
	})

	assert.Len(t, headers, 2)
}

func TestProducerRedpandaRepository_ProducerDefault_EmptyTopic(t *testing.T) {
	repo := &ProducerRedpandaRepository{client: new(kgo.Client)}

	_, err := repo.ProducerDefault(context.Background(), " ", "key", []byte("payload"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "topic cannot be empty")
}

func TestProducerRedpandaRepository_ProducerDefault_ReturnsPublishError(t *testing.T) {
	repo, err := NewProducerRedpanda([]string{"127.0.0.1:1"}, 0, 0, true)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, repo.Close())
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = repo.ProducerDefault(ctx, "ledger.balance.operations", "key", []byte("payload"))
	assert.Error(t, err)
}

func TestProducerRedpandaRepository_Close_NilReceiver(t *testing.T) {
	var repo *ProducerRedpandaRepository
	assert.NoError(t, repo.Close())
}
