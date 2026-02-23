// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import brokerpkg "github.com/LerianStudio/midaz/v3/pkg/broker"

// ParseSeedBrokers parses a comma-separated broker list and removes empty values.
func ParseSeedBrokers(raw string) []string {
	return brokerpkg.ParseSeedBrokers(raw)
}
