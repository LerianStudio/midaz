// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

type engineWriteBehindPublisher interface {
	Publish(context.Context, rabbitmq.EngineWriteBehindMessage) error
}

type engineWriteBehindDispatcher struct {
	publisher engineWriteBehindPublisher
}

func (dispatcher engineWriteBehindDispatcher) DispatchTransactionWriteBehind(
	ctx context.Context,
	envelope *command.TransactionWriteBehindEnvelope,
) error {
	if dispatcher.publisher == nil || envelope == nil {
		return fmt.Errorf("engine write-behind dispatcher is not configured")
	}

	body, err := command.EncodeTransactionWriteBehindEnvelope(*envelope)
	if err != nil {
		return fmt.Errorf("encode engine write-behind dispatch: %w", err)
	}

	return dispatcher.publisher.Publish(ctx, rabbitmq.EngineWriteBehindMessage{
		TenantID:      envelope.Record.TenantID,
		TransactionID: envelope.Record.TransactionID,
		ExecutionID:   envelope.Record.ExecutionID,
		Body:          body,
	})
}
