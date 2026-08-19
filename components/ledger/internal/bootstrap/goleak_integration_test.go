//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"

	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	rabbitmqtestutil "github.com/LerianStudio/midaz/v4/tests/utils/rabbitmq"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

func init() {
	cleanupReusableFixtures = func() error {
		return errors.Join(
			pgtestutil.CleanupReusableContainers(),
			mongotestutil.CleanupReusableContainers(),
			redistestutil.CleanupReusableContainers(),
			rabbitmqtestutil.CleanupReusableContainers(),
		)
	}
}
