// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type staticRedisProvider struct {
	client redis.UniversalClient
	err    error
}

func (p *staticRedisProvider) GetClient(_ context.Context) (redis.UniversalClient, error) {
	if p.err != nil {
		return nil, p.err
	}

	return p.client, nil
}
