// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"

var BalanceSynced = metrics.Metric{
	Name:        "balance_synced",
	Unit:        "1",
	Description: "Measures the number of balances synced.",
}
