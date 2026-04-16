// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package publisher

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopPublisher_Allows_NonDurableTopic(t *testing.T) {
	t.Parallel()

	p := NewNoopPublisher()

	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	cases := []string{
		"authorizer.metrics",
		"authorizer.audit",
		"some.random.telemetry",
		"",
	}

	for _, topic := range cases {
		t.Run("topic="+topic, func(t *testing.T) {
			t.Parallel()

			err := p.Publish(context.Background(), Message{Topic: topic, Payload: []byte("x")})
			assert.NoError(t, err)
		})
	}
}

func TestNoopPublisher_RefusesCommitIntentTopic(t *testing.T) {
	t.Parallel()

	p := NewNoopPublisher()

	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	cases := []string{
		"commit-intent",
		"authorizer.commit-intent.v1",
		"commit.intent",
		"commit.intent.v1",
		"commitintent",
		"COMMIT-INTENT",
		"Authorizer.Commit.Intent",
	}

	for _, topic := range cases {
		t.Run("topic="+topic, func(t *testing.T) {
			t.Parallel()

			err := p.Publish(context.Background(), Message{Topic: topic, Payload: []byte("x")})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrNoopPublisherRefusesDurableTopic)
		})
	}
}

func TestNoopPublisher_RefusesBalanceOperationsTopic(t *testing.T) {
	t.Parallel()

	p := NewNoopPublisher()

	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	cases := []string{
		"balance-operations",
		"authorizer.balance-operations.v1",
		"balance.operations",
		"balance.operations.events",
		"balanceoperations",
		"BALANCE-OPERATIONS",
		"Balance.Operations",
	}

	for _, topic := range cases {
		t.Run("topic="+topic, func(t *testing.T) {
			t.Parallel()

			err := p.Publish(context.Background(), Message{Topic: topic, Payload: []byte("x")})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrNoopPublisherRefusesDurableTopic)
		})
	}
}
