// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/vmihailenco/msgpack/v5"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// errReplayConsumerClosed is returned when the replay consumer is closed while reading.
var errReplayConsumerClosed = errors.New("replay consumer closed")

const defaultReplayTimeout = 2 * time.Second

// StaleBalanceRecoverer rebuilds latest balances from not-yet-consumed records.
type StaleBalanceRecoverer interface {
	RecoverLaggedAliases(
		ctx context.Context,
		topic string,
		organizationID, ledgerID uuid.UUID,
		laggedAliasesByPartition map[int32][]string,
	) (map[string]*mmodel.Balance, error)
}

// FranzStaleBalanceRecoverer replays unconsumed records from Redpanda partitions.
type FranzStaleBalanceRecoverer struct {
	adminClient   *kgo.Client
	brokers       []string
	securityOpts  []kgo.Opt
	consumerGroup string
}

// NewFranzStaleBalanceRecoverer builds a franz-go based stale-balance recoverer.
func NewFranzStaleBalanceRecoverer(
	adminClient *kgo.Client,
	brokers []string,
	securityOpts []kgo.Opt,
	consumerGroup string,
) *FranzStaleBalanceRecoverer {
	return &FranzStaleBalanceRecoverer{
		adminClient:   adminClient,
		brokers:       brokers,
		securityOpts:  securityOpts,
		consumerGroup: consumerGroup,
	}
}

// RecoverLaggedAliases replays records in [committedOffset, endOffset) for lagged
// partitions and returns the latest recovered balances keyed by alias#key.
func (r *FranzStaleBalanceRecoverer) RecoverLaggedAliases(
	ctx context.Context,
	topic string,
	organizationID, ledgerID uuid.UUID,
	laggedAliasesByPartition map[int32][]string,
) (map[string]*mmodel.Balance, error) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.recover_lagged_aliases")
	defer span.End()

	if r == nil || r.adminClient == nil || topic == "" || len(laggedAliasesByPartition) == 0 {
		return map[string]*mmodel.Balance{}, nil
	}

	admin := kadm.NewClient(r.adminClient)
	recovered := make(map[string]*mmodel.Balance)

	for partition, aliases := range laggedAliasesByPartition {
		if len(aliases) == 0 {
			continue
		}

		committedOffset, endOffset, err := r.partitionReplayBounds(ctx, admin, topic, partition)
		if err != nil {
			return nil, err
		}

		if endOffset <= committedOffset {
			continue
		}

		aliasSet := make(map[string]struct{}, len(aliases))
		for _, alias := range aliases {
			aliasSet[alias] = struct{}{}
		}

		latestByAlias, err := r.replayPartitionRange(ctx, topic, partition, committedOffset, endOffset, organizationID, ledgerID, aliasSet)
		if err != nil {
			return nil, err
		}

		for aliasWithKey, balance := range latestByAlias {
			recovered[aliasWithKey] = balance
		}
	}

	return recovered, nil
}

func (r *FranzStaleBalanceRecoverer) partitionReplayBounds(
	ctx context.Context,
	admin *kadm.Client,
	topic string,
	partition int32,
) (int64, int64, error) {
	committedOffsets, err := admin.FetchOffsets(ctx, r.consumerGroup)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch consumer committed offsets: %w", err)
	}

	committed, ok := committedOffsets.Lookup(topic, partition)
	if !ok {
		return 0, 0, nil
	}

	if committed.Err != nil {
		return 0, 0, fmt.Errorf("fetch offset response for topic=%s partition=%d: %w", topic, partition, committed.Err)
	}

	endOffsets, err := admin.ListEndOffsets(ctx, topic)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch partition end offsets: %w", err)
	}

	end, ok := endOffsets.Lookup(topic, partition)
	if !ok {
		return 0, 0, nil
	}

	if end.Err != nil {
		return 0, 0, fmt.Errorf("list end offset response for topic=%s partition=%d: %w", topic, partition, end.Err)
	}

	committedOffset := committed.At
	endOffset := end.Offset

	if committedOffset < 0 {
		committedOffset = 0
	}

	if endOffset < 0 {
		endOffset = 0
	}

	return committedOffset, endOffset, nil
}

func (r *FranzStaleBalanceRecoverer) replayPartitionRange(
	ctx context.Context,
	topic string,
	partition int32,
	startOffset, endOffset int64,
	organizationID, ledgerID uuid.UUID,
	aliases map[string]struct{},
) (map[string]*mmodel.Balance, error) {
	if startOffset >= endOffset {
		return map[string]*mmodel.Balance{}, nil
	}

	replayCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc

		replayCtx, cancel = context.WithTimeout(ctx, defaultReplayTimeout)
		defer cancel()
	}

	consumePartitions := map[string]map[int32]kgo.Offset{
		topic: {
			partition: kgo.NewOffset().At(startOffset),
		},
	}

	const baseOptCount = 3

	opts := make([]kgo.Opt, 0, baseOptCount+len(r.securityOpts))
	opts = append(opts,
		kgo.SeedBrokers(r.brokers...),
		kgo.ConsumePartitions(consumePartitions),
		kgo.FetchIsolationLevel(kgo.ReadCommitted()),
	)
	opts = append(opts, r.securityOpts...)

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize replay consumer: %w", err)
	}
	defer client.Close()

	latestByAlias := make(map[string]*mmodel.Balance)
	nextOffset := startOffset

	for nextOffset < endOffset {
		fetches := client.PollFetches(replayCtx)
		if fetches.IsClientClosed() {
			return nil, fmt.Errorf("%w: partition %d", errReplayConsumerClosed, partition)
		}

		if errs := fetches.Errors(); len(errs) > 0 {
			return nil, fmt.Errorf("failed to replay partition records: %w", errs[0].Err)
		}

		iter := fetches.RecordIter()
		if iter.Done() {
			if replayCtx.Err() != nil {
				return nil, replayCtx.Err()
			}

			continue
		}

		for !iter.Done() {
			record := iter.Next()
			if record == nil || record.Offset >= endOffset {
				continue
			}

			recovered, parseErr := extractRecoveredBalances(record.Value, organizationID, ledgerID, aliases, latestByAlias)
			if parseErr != nil {
				return nil, parseErr
			}

			latestByAlias = recovered

			nextOffset = record.Offset + 1
		}
	}

	return latestByAlias, nil
}

func extractRecoveredBalances(
	recordValue []byte,
	organizationID, ledgerID uuid.UUID,
	aliases map[string]struct{},
	latestByAlias map[string]*mmodel.Balance,
) (map[string]*mmodel.Balance, error) {
	queueMessage := mmodel.Queue{}
	if err := msgpack.Unmarshal(recordValue, &queueMessage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal queue envelope during stale-balance replay: %w", err)
	}

	if queueMessage.OrganizationID != organizationID || queueMessage.LedgerID != ledgerID {
		if latestByAlias != nil {
			return latestByAlias, nil
		}

		return map[string]*mmodel.Balance{}, nil
	}

	recovered := latestByAlias
	if recovered == nil {
		recovered = make(map[string]*mmodel.Balance)
	}

	for _, item := range queueMessage.QueueData {
		payload := transaction.TransactionProcessingPayload{}
		if err := msgpack.Unmarshal(item.Value, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal queue payload during stale-balance replay: %w", err)
		}

		if payload.Validate == nil || len(payload.Balances) == 0 {
			continue
		}

		fromTo := make(map[string]pkgTransaction.Amount, len(payload.Validate.From)+len(payload.Validate.To))
		for alias, amount := range payload.Validate.From {
			fromTo[alias] = amount
		}

		for alias, amount := range payload.Validate.To {
			fromTo[alias] = amount
		}

		var err error

		recovered, err = applyRecoveredBalanceOperations(payload.Balances, fromTo, aliases, recovered)
		if err != nil {
			return nil, err
		}
	}

	return recovered, nil
}

// applyRecoveredBalanceOperations replays balance mutations from a single payload
// onto the accumulated balance state, updating only balances matching the target aliases.
func applyRecoveredBalanceOperations(
	balances []*mmodel.Balance,
	fromTo map[string]pkgTransaction.Amount,
	aliases map[string]struct{},
	recovered map[string]*mmodel.Balance,
) (map[string]*mmodel.Balance, error) {
	for _, balance := range balances {
		if balance == nil {
			continue
		}

		aliasWithKey := pkgTransaction.SplitAliasWithKey(balance.Alias)
		if _, ok := aliases[aliasWithKey]; !ok {
			continue
		}

		amount, ok := fromTo[balance.Alias]
		if !ok {
			continue
		}

		baseBalance := balance
		if accumulated, exists := recovered[aliasWithKey]; exists && accumulated != nil {
			baseBalance = accumulated
		}

		updatedAmounts, err := pkgTransaction.OperateBalances(amount, *baseBalance.ToTransactionBalance())
		if err != nil {
			return nil, fmt.Errorf("failed to replay balance operation for alias %s: %w", aliasWithKey, err)
		}

		accountAlias, balanceKey := shard.SplitAliasAndBalanceKey(aliasWithKey)
		recovered[aliasWithKey] = &mmodel.Balance{
			ID:             baseBalance.ID,
			OrganizationID: baseBalance.OrganizationID,
			LedgerID:       baseBalance.LedgerID,
			AccountID:      baseBalance.AccountID,
			Alias:          accountAlias,
			Key:            balanceKey,
			AssetCode:      baseBalance.AssetCode,
			Available:      updatedAmounts.Available,
			OnHold:         updatedAmounts.OnHold,
			Version:        updatedAmounts.Version,
			AccountType:    baseBalance.AccountType,
			AllowSending:   baseBalance.AllowSending,
			AllowReceiving: baseBalance.AllowReceiving,
		}
	}

	return recovered, nil
}
