// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import "context"

// AuthorizerPublisher encapsulates outbound publishing ownership when authorizer is enabled.
type AuthorizerPublisher interface {
	Enabled() bool
	PublishBalanceOperations(ctx context.Context, exchange, routingKey string, payload []byte, headers map[string]string) error
}
