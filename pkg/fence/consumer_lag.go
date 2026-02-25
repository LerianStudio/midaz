// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fence

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const defaultLagCacheTTL = 500 * time.Millisecond

// ConsumerLagChecker verifies whether a Redpanda / Kafka partition is fully
// consumed by a given consumer group.
type ConsumerLagChecker interface {
	// PartitionLag returns outstanding lag for topic+partition.
	//
	// On success, lag >= 0.
	// On error, lag is returned as 0 together with the error so callers can
	// choose fail-open or fail-closed behavior.
	PartitionLag(ctx context.Context, topic string, partition int32) (int64, error)

	// IsPartitionCaughtUp returns true when lag == 0.
	//
	// On lag-inspection errors, behavior depends on checker mode:
	//   - fail-open: logs warning and returns true (availability first)
	//   - fail-closed: logs warning and returns false (consistency first)
	IsPartitionCaughtUp(ctx context.Context, topic string, partition int32) bool
}

type lagAdminClient interface {
	FetchOffsets(ctx context.Context, group string) (kadm.OffsetResponses, error)
	ListEndOffsets(ctx context.Context, topics ...string) (kadm.ListedOffsets, error)
}

type cacheEntry struct {
	lag       int64
	expiresAt time.Time
}

// FranzConsumerLagChecker is a franz-go/kadm based implementation of
// ConsumerLagChecker.
type FranzConsumerLagChecker struct {
	consumerGroup string
	cacheTTL      time.Duration
	failOpen      bool
	admin         lagAdminClient
	now           func() time.Time

	cache sync.Map
}

// NewFranzConsumerLagChecker creates a lag checker backed by franz-go's
// kadm client.
func NewFranzConsumerLagChecker(client *kgo.Client, consumerGroup string, cacheTTL time.Duration) ConsumerLagChecker {
	return NewFranzConsumerLagCheckerWithMode(client, consumerGroup, cacheTTL, true)
}

// NewFranzConsumerLagCheckerWithMode creates a lag checker and lets callers
// choose fail-open (availability) or fail-closed (consistency) behavior.
func NewFranzConsumerLagCheckerWithMode(client *kgo.Client, consumerGroup string, cacheTTL time.Duration, failOpen bool) ConsumerLagChecker {
	if client == nil {
		return NoopConsumerLagChecker{}
	}

	return newFranzConsumerLagChecker(kadm.NewClient(client), consumerGroup, cacheTTL, failOpen)
}

func newFranzConsumerLagChecker(admin lagAdminClient, consumerGroup string, cacheTTL time.Duration, failOpen bool) *FranzConsumerLagChecker {
	if cacheTTL <= 0 {
		cacheTTL = defaultLagCacheTTL
	}

	return &FranzConsumerLagChecker{
		consumerGroup: strings.TrimSpace(consumerGroup),
		cacheTTL:      cacheTTL,
		failOpen:      failOpen,
		admin:         admin,
		now:           time.Now,
	}
}

// PartitionLag returns outstanding lag for a single topic partition.
func (c *FranzConsumerLagChecker) PartitionLag(ctx context.Context, topic string, partition int32) (int64, error) {
	if c == nil || c.admin == nil || c.consumerGroup == "" {
		return 0, nil
	}

	topic = strings.TrimSpace(topic)
	if topic == "" {
		return 0, nil
	}

	cacheKey := topicPartitionKey(topic, partition)
	now := c.now()

	if cached, ok := c.cache.Load(cacheKey); ok {
		entry, casted := cached.(cacheEntry)
		if casted && now.Before(entry.expiresAt) {
			return entry.lag, nil
		}

		c.cache.Delete(cacheKey)
	}

	lag, err := c.fetchLag(ctx, topic, partition)
	if err != nil {
		return 0, err
	}

	c.cache.Store(cacheKey, cacheEntry{
		lag:       lag,
		expiresAt: now.Add(c.cacheTTL),
	})

	return lag, nil
}

// IsPartitionCaughtUp returns true when partition lag is zero.
//
// Errors are treated as caught-up (fail-open) and logged.
func (c *FranzConsumerLagChecker) IsPartitionCaughtUp(ctx context.Context, topic string, partition int32) bool {
	lag, err := c.PartitionLag(ctx, topic, partition)
	if err != nil {
		logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)
		mode := "fail-closed"
		if c.failOpen {
			mode = "fail-open"
		}

		logger.Warnf("Consumer lag check failed (%s) group=%s topic=%s partition=%d err=%v", mode, c.consumerGroup, topic, partition, err)

		return c.failOpen
	}

	return lag == 0
}

func (c *FranzConsumerLagChecker) fetchLag(ctx context.Context, topic string, partition int32) (int64, error) {
	fetched, err := c.admin.FetchOffsets(ctx, c.consumerGroup)
	if err != nil {
		return 0, fmt.Errorf("fetch offsets for group %s: %w", c.consumerGroup, err)
	}

	committed := int64(0)
	if offsetResp, ok := fetched.Lookup(topic, partition); ok {
		if offsetResp.Err != nil {
			return 0, fmt.Errorf("fetch offset response for topic=%s partition=%d: %w", topic, partition, offsetResp.Err)
		}

		committed = offsetResp.At
		if committed < 0 {
			committed = 0
		}
	}

	endOffsets, err := c.admin.ListEndOffsets(ctx, topic)
	if err != nil {
		return 0, fmt.Errorf("list end offsets for topic %s: %w", topic, err)
	}

	endOffset, exists := endOffsets.Lookup(topic, partition)
	if !exists {
		return 0, fmt.Errorf("missing end offset for topic=%s partition=%d", topic, partition)
	}

	if endOffset.Err != nil {
		return 0, fmt.Errorf("list end offset response for topic=%s partition=%d: %w", topic, partition, endOffset.Err)
	}

	highWatermark := endOffset.Offset
	if highWatermark < 0 {
		highWatermark = 0
	}

	lag := highWatermark - committed
	if lag < 0 {
		return 0, nil
	}

	return lag, nil
}

func topicPartitionKey(topic string, partition int32) string {
	return fmt.Sprintf("%s:%d", topic, partition)
}

// NoopConsumerLagChecker always reports partitions as caught up.
type NoopConsumerLagChecker struct{}

func (NoopConsumerLagChecker) PartitionLag(_ context.Context, _ string, _ int32) (int64, error) {
	return 0, nil
}

func (NoopConsumerLagChecker) IsPartitionCaughtUp(_ context.Context, _ string, _ int32) bool {
	return true
}
