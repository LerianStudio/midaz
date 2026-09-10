// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"fmt"

	redisengine "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func configureBalanceEngine(useCase *command.UseCase, provider redisengine.RedisClientProvider) error {
	if useCase == nil {
		return errors.New("balance engine configuration requires a command owner")
	}

	if useCase.AppliedTransactionCompleter == nil {
		return errors.New("balance engine configuration requires an applied transaction completer")
	}

	adapter, err := redisengine.NewAdapter(provider)
	if err != nil {
		return fmt.Errorf("initialize balance engine adapter: %w", err)
	}

	useCase.BalanceEngine = adapter

	return nil
}
