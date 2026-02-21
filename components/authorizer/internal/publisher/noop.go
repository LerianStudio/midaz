// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package publisher

import "context"

// Message is a publish request envelope.
type Message struct {
	Exchange    string
	RoutingKey  string
	Payload     []byte
	Headers     map[string]string
	ContentType string
}

// Publisher represents async publish behavior for authorized operations.
type Publisher interface {
	Publish(ctx context.Context, message Message) error
	Close() error
}

type noopPublisher struct{}

// NewNoopPublisher returns a publisher that discards all payloads.
func NewNoopPublisher() Publisher {
	return noopPublisher{}
}

func (noopPublisher) Publish(_ context.Context, _ Message) error {
	return nil
}

func (noopPublisher) Close() error {
	return nil
}
