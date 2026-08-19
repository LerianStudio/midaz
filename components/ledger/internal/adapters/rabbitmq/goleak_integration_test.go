//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import rabbitmqtestutil "github.com/LerianStudio/midaz/v4/tests/utils/rabbitmq"

func init() {
	cleanupReusableRabbitMQFixture = rabbitmqtestutil.CleanupReusableContainers
}
