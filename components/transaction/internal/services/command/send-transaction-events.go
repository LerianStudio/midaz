// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

const (
	Source    string = "midaz"
	EventType string = "transaction"
)

func (uc *UseCase) SendTransactionEvents(ctx context.Context, tran *transaction.Transaction) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	if !uc.EventsEnabled {
		logger.Info("Transaction events are disabled")
		return
	}

	if tran == nil {
		logger.Errorf("Failed to send transaction event: transaction payload is nil")
		return
	}

	if uc.BrokerRepo == nil {
		logger.Errorf("Failed to send transaction event: broker repository is not configured")
		return
	}

	ctxSendTransactionEvents, spanTransactionEvents := tracer.Start(ctx, "command.send_transaction_events_async")
	defer spanTransactionEvents.End()

	payload, err := json.Marshal(tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanTransactionEvents, "Failed to marshal transaction to JSON string", err)

		logger.Errorf("Failed to marshal transaction to JSON string: %s", err.Error())

		return
	}

	event := mmodel.Event{
		Source:         Source,
		EventType:      EventType,
		Action:         tran.Status.Code,
		TimeStamp:      time.Now(),
		Version:        uc.Version,
		OrganizationID: tran.OrganizationID,
		LedgerID:       tran.LedgerID,
		Payload:        payload,
	}

	key := tran.ID
	if key == "" {
		key = Source + "." + EventType + "." + tran.Status.Code
	}

	logger.Infof("Sending transaction events to key: %s", key)

	message, err := json.Marshal(event)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanTransactionEvents, "Failed to marshal event message struct", err)

		logger.Errorf("Failed to marshal event message struct")

		return
	}

	if _, err := uc.BrokerRepo.ProducerDefault(
		ctxSendTransactionEvents,
		uc.EventsTopic,
		key,
		message,
	); err != nil {
		libOpentelemetry.HandleSpanError(&spanTransactionEvents, "Failed to send transaction events to topic", err)

		logger.Errorf("Failed to send message: %s", err.Error())
	}
}
