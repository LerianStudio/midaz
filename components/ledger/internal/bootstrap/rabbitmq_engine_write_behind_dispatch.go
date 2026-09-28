// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vmihailenco/msgpack/v5"

	postgresTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

type rabbitEngineEvidenceResolver struct {
	repository txRedis.EngineWriteBehindRepository
}

func (resolver rabbitEngineEvidenceResolver) ResolveTransactionEvidence(ctx context.Context, reference command.TransactionEvidenceReference) (*command.TransactionWriteBehindEnvelope, error) {
	raw, _, err := resolver.repository.GetEngineTransactionEvidence(ctx, reference.OrganizationID, reference.LedgerID, reference.TransactionID, reference.ExecutionID)
	if err != nil {
		return nil, err
	}

	envelope, err := command.DecodeTransactionWriteBehindEnvelope(raw)
	if err != nil {
		return nil, err
	}

	return envelope, nil
}

type rabbitTransactionDispatcher struct {
	useCase        *command.UseCase
	completer      command.AppliedTransactionCompleter
	bulkCompleter  command.AppliedTransactionBulkCompleter
	resolver       command.TransactionEvidenceResolver
	metricsFactory *metrics.MetricsFactory
	tenantPolicy   rabbitEngineTenantPolicy
}

type rabbitEngineTenantPolicy uint8

const (
	rabbitEngineSingleTenant rabbitEngineTenantPolicy = iota
	rabbitEngineMultiTenant
)

var (
	errRabbitTransactionUseCaseNotConfigured       = errors.New("rabbitmq transaction dispatcher requires a command use case")
	errRabbitTransactionCompleterNotConfigured     = errors.New("rabbitmq transaction dispatcher requires an applied transaction completer")
	errRabbitTransactionBulkCompleterNotConfigured = errors.New("rabbitmq transaction dispatcher requires an applied transaction bulk completer")
)

func newRabbitTransactionDispatcher(useCase *command.UseCase, tenantPolicy rabbitEngineTenantPolicy, bulkRequired bool, factories ...*metrics.MetricsFactory) (*rabbitTransactionDispatcher, error) {
	if useCase == nil {
		return nil, errRabbitTransactionUseCaseNotConfigured
	}

	if useCase.AppliedTransactionCompleter == nil {
		return nil, errRabbitTransactionCompleterNotConfigured
	}

	bulkCompleter, hasBulkCompleter := useCase.AppliedTransactionCompleter.(command.AppliedTransactionBulkCompleter)
	if bulkRequired && !hasBulkCompleter {
		return nil, errRabbitTransactionBulkCompleterNotConfigured
	}

	dispatcher := &rabbitTransactionDispatcher{useCase: useCase, tenantPolicy: tenantPolicy}
	if len(factories) > 0 {
		dispatcher.metricsFactory = factories[0]
	}

	dispatcher.completer = useCase.AppliedTransactionCompleter

	dispatcher.bulkCompleter = bulkCompleter
	if repository, ok := useCase.TransactionRedisRepo.(txRedis.EngineWriteBehindRepository); ok {
		dispatcher.resolver = rabbitEngineEvidenceResolver{repository: repository}
	}

	return dispatcher, nil
}

func (dispatcher *rabbitTransactionDispatcher) handle(ctx context.Context, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if dispatcher == nil || dispatcher.useCase == nil {
		return fmt.Errorf("RabbitMQ transaction dispatcher is not configured")
	}

	direct, isEngine, err := decodeRabbitEngineValue(body)
	if err != nil {
		return err
	}

	if isEngine {
		return dispatcher.completeEngine(ctx, direct)
	}

	var wrapper mmodel.Queue
	if err := msgpack.Unmarshal(body, &wrapper); err != nil {
		return fmt.Errorf("decode transaction queue wrapper: %w", err)
	}

	if len(wrapper.QueueData) == 0 {
		return fmt.Errorf("transaction queue wrapper has no entries")
	}

	for _, entry := range wrapper.QueueData {
		envelope, engine, err := decodeRabbitEngineValue(entry.Value)
		if err != nil {
			return err
		}

		if engine {
			if err := validateRabbitWrapperScope(wrapper, envelope); err != nil {
				return err
			}

			if err := dispatcher.completeEngine(ctx, envelope); err != nil {
				return err
			}

			continue
		}

		legacy := wrapper

		legacy.QueueData = []mmodel.QueueData{entry}
		if err := dispatcher.useCase.CreateBalanceTransactionOperationsAsync(ctx, legacy); err != nil {
			return err
		}
	}

	return nil
}

func (dispatcher *rabbitTransactionDispatcher) completeEngine(ctx context.Context, envelope *command.TransactionWriteBehindEnvelope) error {
	if dispatcher.completer == nil {
		return fmt.Errorf("applied transaction completer is not configured")
	}

	ctx, err := dispatcher.engineTenantContext(ctx, envelope.Record.TenantID)
	if err != nil {
		return err
	}

	completion, err := command.CompleteTransactionWriteBehind(ctx, envelope, dispatcher.resolver, dispatcher.completer)
	if err != nil {
		return err
	}

	dispatcher.acknowledgeEngine(ctx, &envelope.Record, completion.Current)

	return nil
}

func (dispatcher *rabbitTransactionDispatcher) acknowledgeEngine(ctx context.Context, record *command.TransactionCompletionRecord, result command.TransactionCompletionResult) {
	if dispatcher.useCase.EngineRecoveryAcknowledger != nil {
		// Redis ACK failure leaves the protected recovery record available; SQL
		// and metadata are already durable, so the broker message may be ACKed.
		_ = dispatcher.useCase.EngineRecoveryAcknowledger.AcknowledgeEngineRecovery(ctx, record, result)
	}
}

type decodedRabbitTransactionMessage struct {
	engines []*command.TransactionWriteBehindEnvelope
	legacy  []postgresTransaction.TransactionProcessingPayload
}

type rabbitEngineUnit struct {
	messageIndex int
	envelope     *command.TransactionWriteBehindEnvelope
}

//nolint:gocognit,gocyclo // mixed legacy/versioned deliveries require per-message correlation and fail-closed outcomes
func (dispatcher *rabbitTransactionDispatcher) handleBulk(ctx context.Context, deliveries []amqp.Delivery) ([]rabbitmq.BulkMessageResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if dispatcher == nil || dispatcher.useCase == nil {
		return nil, fmt.Errorf("RabbitMQ transaction dispatcher is not configured")
	}

	decoded := make([]decodedRabbitTransactionMessage, len(deliveries))

	results := make([]rabbitmq.BulkMessageResult, len(deliveries))
	for index := range results {
		results[index] = rabbitmq.BulkMessageResult{Index: index, Success: true}

		message, err := decodeRabbitTransactionMessage(deliveries[index].Body)
		if err != nil {
			results[index].Success, results[index].Error = false, err
			continue
		}

		decoded[index] = message
	}

	engineGroups := make(map[string][]rabbitEngineUnit)
	legacyPayloads := make([]postgresTransaction.TransactionProcessingPayload, 0)
	legacyMessages := make(map[int]struct{})

	for messageIndex, message := range decoded {
		if !results[messageIndex].Success {
			continue
		}

		for _, envelope := range message.engines {
			key := envelope.Record.TenantID + ":" + envelope.Record.OrganizationID.String() + ":" + envelope.Record.LedgerID.String()
			engineGroups[key] = append(engineGroups[key], rabbitEngineUnit{messageIndex: messageIndex, envelope: envelope})
		}

		if len(message.legacy) > 0 {
			legacyMessages[messageIndex] = struct{}{}

			legacyPayloads = append(legacyPayloads, message.legacy...)
		}
	}

	if len(engineGroups) > 0 && dispatcher.bulkCompleter == nil {
		err := fmt.Errorf("applied transaction bulk completer is not configured")
		for _, units := range engineGroups {
			markRabbitEngineUnitsFailed(results, units, err)
		}
	} else {
		for _, units := range engineGroups {
			envelopes := make([]*command.TransactionWriteBehindEnvelope, len(units))
			for index := range units {
				envelopes[index] = units[index].envelope
			}

			groupCtx, err := dispatcher.engineTenantContext(ctx, envelopes[0].Record.TenantID)
			if err != nil {
				markRabbitEngineUnitsFailed(results, units, err)
				continue
			}

			completed, err := command.CompleteTransactionWriteBehindBulk(groupCtx, envelopes, dispatcher.resolver, dispatcher.bulkCompleter)
			if err != nil {
				markRabbitEngineUnitsFailed(results, units, err)
				continue
			}

			for index, completion := range completed {
				dispatcher.acknowledgeEngine(groupCtx, &envelopes[index].Record, completion)
			}
		}
	}

	if len(legacyPayloads) > 0 {
		startedAt := time.Now()

		result, err := dispatcher.useCase.CreateBulkTransactionOperationsAsync(ctx, legacyPayloads)
		if err != nil {
			for messageIndex := range legacyMessages {
				results[messageIndex].Success, results[messageIndex].Error = false, err
			}
		} else if result != nil {
			recordBulkOTelMetrics(ctx, dispatcher.metricsFactory, result, legacyPayloads, time.Since(startedAt))
		}
	}

	return results, nil
}

func markRabbitEngineUnitsFailed(results []rabbitmq.BulkMessageResult, units []rabbitEngineUnit, err error) {
	for _, unit := range units {
		results[unit.messageIndex].Success = false
		results[unit.messageIndex].Error = err
	}
}

func decodeRabbitTransactionMessage(body []byte) (decodedRabbitTransactionMessage, error) {
	if envelope, engine, err := decodeRabbitEngineValue(body); err != nil {
		return decodedRabbitTransactionMessage{}, err
	} else if engine {
		return decodedRabbitTransactionMessage{engines: []*command.TransactionWriteBehindEnvelope{envelope}}, nil
	}

	var wrapper mmodel.Queue
	if err := msgpack.Unmarshal(body, &wrapper); err != nil {
		return decodedRabbitTransactionMessage{}, fmt.Errorf("decode transaction queue wrapper: %w", err)
	}

	if len(wrapper.QueueData) == 0 {
		return decodedRabbitTransactionMessage{}, fmt.Errorf("transaction queue wrapper has no entries")
	}

	decoded := decodedRabbitTransactionMessage{}

	for _, entry := range wrapper.QueueData {
		envelope, engine, err := decodeRabbitEngineValue(entry.Value)
		if err != nil {
			return decodedRabbitTransactionMessage{}, err
		}

		if engine {
			if err := validateRabbitWrapperScope(wrapper, envelope); err != nil {
				return decodedRabbitTransactionMessage{}, err
			}

			decoded.engines = append(decoded.engines, envelope)

			continue
		}

		var payload postgresTransaction.TransactionProcessingPayload
		if err := msgpack.Unmarshal(entry.Value, &payload); err != nil {
			return decodedRabbitTransactionMessage{}, fmt.Errorf("decode legacy transaction queue entry: %w", err)
		}

		decoded.legacy = append(decoded.legacy, payload)
	}

	return decoded, nil
}

func decodeRabbitEngineValue(raw []byte) (*command.TransactionWriteBehindEnvelope, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return nil, false, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, false, err
	}

	rawVersion, versioned := fields["formatVersion"]
	if !versioned {
		return nil, false, nil
	}

	var version int
	if err := json.Unmarshal(rawVersion, &version); err != nil {
		return nil, false, fmt.Errorf("decode RabbitMQ transaction version: %w", err)
	}

	switch version {
	case command.TransactionWriteBehindFormatVersion:
		if _, ok := fields["record"]; !ok {
			return nil, false, fmt.Errorf("version-one RabbitMQ transaction is not a write-behind envelope")
		}

		envelope, err := command.DecodeTransactionWriteBehindEnvelope(trimmed)

		return envelope, true, err
	case command.TransactionCompletionFormatVersion:
		record, err := command.DecodeTransactionCompletionRecord(trimmed)
		if err != nil {
			return nil, false, err
		}

		return &command.TransactionWriteBehindEnvelope{
			FormatVersion: command.TransactionWriteBehindFormatVersion, ApplicationState: command.TransactionApplicationConfirmed,
			ReplayState: command.TransactionReplayReconstructible, DurabilityState: command.TransactionDurabilityPending, Record: *record,
		}, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported RabbitMQ transaction version %d", version)
	}
}

func validateRabbitWrapperScope(wrapper mmodel.Queue, envelope *command.TransactionWriteBehindEnvelope) error {
	if wrapper.OrganizationID != uuid.Nil && wrapper.OrganizationID != envelope.Record.OrganizationID {
		return fmt.Errorf("RabbitMQ wrapper organization differs from engine evidence")
	}

	if wrapper.LedgerID != uuid.Nil && wrapper.LedgerID != envelope.Record.LedgerID {
		return fmt.Errorf("RabbitMQ wrapper ledger differs from engine evidence")
	}

	return nil
}

func (dispatcher *rabbitTransactionDispatcher) engineTenantContext(ctx context.Context, tenantID string) (context.Context, error) {
	authenticated := tmcore.GetTenantIDContext(ctx)

	switch dispatcher.tenantPolicy {
	case rabbitEngineSingleTenant:
		if authenticated != "" || tenantID != "" {
			return nil, rabbitmq.ErrEngineWriteBehindTenantUnexpected
		}

		return ctx, nil
	case rabbitEngineMultiTenant:
		if authenticated == "" || tenantID == "" {
			return nil, rabbitmq.ErrEngineWriteBehindTenantRequired
		}

		if authenticated != tenantID {
			return nil, rabbitmq.ErrEngineWriteBehindTenantMismatch
		}

		return ctx, nil
	default:
		return nil, fmt.Errorf("RabbitMQ engine tenant policy is not configured")
	}
}
