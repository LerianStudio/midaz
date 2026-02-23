// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package broker

import "strings"

// ParseSeedBrokers parses a comma-separated broker list and removes empty values.
func ParseSeedBrokers(raw string) []string {
	parts := strings.Split(raw, ",")
	brokers := make([]string, 0, len(parts))

	for _, part := range parts {
		broker := strings.TrimSpace(part)
		if broker == "" {
			continue
		}

		brokers = append(brokers, broker)
	}

	return brokers
}
