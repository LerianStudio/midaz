// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"
)

// ErrBrokerUnhealthy indicates broker connectivity validation failed.
var ErrBrokerUnhealthy = errors.New("redpanda health check failed")

// BrokerHealthChecker is the minimal contract required for health checks.
type BrokerHealthChecker interface {
	Ping(ctx context.Context) error
}

// CheckBrokerHealth validates broker availability.
func CheckBrokerHealth(ctx context.Context, checker BrokerHealthChecker) error {
	if checker == nil {
		return ErrBrokerUnhealthy
	}

	if err := checker.Ping(ctx); err != nil {
		return errors.Join(ErrBrokerUnhealthy, err)
	}

	return nil
}
