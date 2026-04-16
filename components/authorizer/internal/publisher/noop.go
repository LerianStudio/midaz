// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package publisher

import (
	"context"
	"errors"
	"strings"
)

// ErrNoopPublisherRefusesDurableTopic is returned by the no-op publisher when a caller
// attempts to publish to a topic whose payloads must be durable (e.g. commit intents,
// balance operations). This is a belt-and-suspenders defense so that a misconfigured
// runtime that selects the no-op publisher cannot silently drop durable events.
var ErrNoopPublisherRefusesDurableTopic = errors.New("no-op publisher refuses to accept durable topic payloads")

// durableTopicMarkers are substrings that identify topics whose payloads MUST be durable.
// Match is case-insensitive and uses substring containment so minor naming drift across
// environments still refuses the message.
var durableTopicMarkers = []string{
	"commit-intent",
	"commit.intent",
	"commitintent",
	"balance-operations",
	"balance.operations",
	"balanceoperations",
}

// Message is a publish request envelope.
type Message struct {
	Topic        string
	PartitionKey string
	Payload      []byte
	Headers      map[string]string
	ContentType  string
}

// Publisher represents async publish behavior for authorized operations.
type Publisher interface {
	Publish(ctx context.Context, message Message) error
	Close() error
}

type noopPublisher struct{}

// NewNoopPublisher returns a publisher that discards all non-durable payloads and
// explicitly refuses payloads targeted at durable topics.
func NewNoopPublisher() Publisher {
	return noopPublisher{}
}

// Publish discards non-durable payloads and returns ErrNoopPublisherRefusesDurableTopic
// for any message whose topic matches a known durable-topic marker.
func (noopPublisher) Publish(_ context.Context, message Message) error {
	if isDurableTopic(message.Topic) {
		return ErrNoopPublisherRefusesDurableTopic
	}

	return nil
}

// Close is a no-op that always returns nil.
func (noopPublisher) Close() error {
	return nil
}

// isDurableTopic reports whether the given topic name contains any substring
// that marks it as requiring durable publication.
func isDurableTopic(topic string) bool {
	lowered := strings.ToLower(topic)
	for _, marker := range durableTopicMarkers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}

	return false
}
