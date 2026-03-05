// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

const decisionEventType string = "transaction.decision"

func (uc *UseCase) decisionEventsTopic() string {
	topic := strings.TrimSpace(uc.DecisionEventsTopic)
	if topic != "" {
		return topic
	}

	return uc.EventsTopic
}

// SendDecisionLifecycleEvent publishes decision lifecycle events to the broker.
func (uc *UseCase) SendDecisionLifecycleEvent(
	ctx context.Context,
	tran *transaction.Transaction,
	decisionContract pkgTransaction.DecisionContract,
	action pkgTransaction.DecisionLifecycleAction,
) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	if !uc.EventsEnabled {
		logger.Info("Decision lifecycle events are disabled")
		return
	}

	if tran == nil {
		logger.Errorf("Failed to send decision lifecycle event: transaction payload is nil")
		return
	}

	if uc.BrokerRepo == nil {
		logger.Errorf("Failed to send decision lifecycle event: broker repository is not configured")
		return
	}

	ctxSendDecisionEvent, spanDecisionEvent := tracer.Start(ctx, "command.send_decision_lifecycle_event_async")
	defer spanDecisionEvent.End()

	payload := pkgTransaction.NormalizeDecisionLifecycleEvent(pkgTransaction.DecisionLifecycleEvent{
		TransactionID:     tran.ID,
		Action:            action,
		DecisionContract:  decisionContract,
		TransactionStatus: tran.Status.Code,
	})

	marshaledPayload, err := json.Marshal(payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanDecisionEvent, "Failed to marshal decision lifecycle payload", err)
		logger.Errorf("Failed to marshal decision lifecycle payload: %s", err)

		return
	}

	event := mmodel.Event{
		Source:         Source,
		EventType:      decisionEventType,
		Action:         string(action),
		TimeStamp:      time.Now().UTC(),
		Version:        uc.Version,
		OrganizationID: tran.OrganizationID,
		LedgerID:       tran.LedgerID,
		Payload:        marshaledPayload,
	}

	key := tran.ID
	if key == "" {
		key = Source + "." + decisionEventType + "." + string(action)
	}

	message, err := json.Marshal(event)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanDecisionEvent, "Failed to marshal decision lifecycle event struct", err)
		logger.Errorf("Failed to marshal decision lifecycle event struct")

		return
	}

	if _, err := uc.BrokerRepo.ProducerDefault(
		ctxSendDecisionEvent,
		uc.decisionEventsTopic(),
		key,
		message,
	); err != nil {
		libOpentelemetry.HandleSpanError(&spanDecisionEvent, "Failed to send decision lifecycle event to topic", err)
		logger.Errorf("Failed to send decision lifecycle event message: %s", err)
	}
}
