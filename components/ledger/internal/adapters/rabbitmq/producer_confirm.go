// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DefaultPublishConfirmTimeout bounds the wait for a broker confirmation when
// no other ceiling is configured.
const DefaultPublishConfirmTimeout = 5 * time.Second

var (
	// ErrPublishNacked indicates the broker refused a published message.
	ErrPublishNacked = errors.New("rabbitmq publish nacked")
	// ErrPublishUnconfirmed indicates the broker outcome of a publish is unknown:
	// the wait ended or the channel closed before a confirmation arrived.
	ErrPublishUnconfirmed = errors.New("rabbitmq publish unconfirmed")
)

// ProducerOption configures a ProducerDefault publisher.
type ProducerOption func(*producerSettings)

type producerSettings struct {
	confirmTimeout time.Duration
}

// WithPublishConfirmTimeout sets the ceiling on the wait for a broker
// confirmation. A caller deadline that expires earlier still ends the wait
// first. Non-positive values keep DefaultPublishConfirmTimeout.
func WithPublishConfirmTimeout(timeout time.Duration) ProducerOption {
	return func(settings *producerSettings) {
		if timeout > 0 {
			settings.confirmTimeout = timeout
		}
	}
}

func newProducerSettings(opts []ProducerOption) producerSettings {
	settings := producerSettings{confirmTimeout: DefaultPublishConfirmTimeout}

	for _, opt := range opts {
		if opt != nil {
			opt(&settings)
		}
	}

	return settings
}

// awaitDeferredConfirmation waits for the confirmation of one message published
// on a channel shared by concurrent publishers. The client nacks every pending
// confirmation when the channel closes, so a negative answer on a closed channel
// is reported as unconfirmed rather than refused.
func awaitDeferredConfirmation(ctx context.Context, confirmation *amqp.DeferredConfirmation, channelClosed func() bool) error {
	if confirmation == nil {
		return fmt.Errorf("%w: channel is not in confirm mode", ErrPublishUnconfirmed)
	}

	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrPublishUnconfirmed, err)
	}

	if acked {
		return nil
	}

	if channelClosed() {
		return fmt.Errorf("%w: channel closed", ErrPublishUnconfirmed)
	}

	return ErrPublishNacked
}

// awaitSingleConfirmation waits for the confirmation of the only message
// published on a channel owned by the caller.
func awaitSingleConfirmation(ctx context.Context, confirmations <-chan amqp.Confirmation) error {
	select {
	case confirmation, ok := <-confirmations:
		if !ok {
			return fmt.Errorf("%w: channel closed", ErrPublishUnconfirmed)
		}

		if !confirmation.Ack {
			return ErrPublishNacked
		}

		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrPublishUnconfirmed, ctx.Err())
	}
}
