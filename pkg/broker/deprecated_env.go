// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package broker

import (
	"sort"
	"strings"
)

var deprecatedBrokerEnvPrefixes = [...]string{
	"RABBITMQ_",
	"AUTHORIZER_RABBITMQ_",
}

var deprecatedBrokerEnvKeys = map[string]struct{}{
	"BROKER_HEALTH_CHECK_TIMEOUT": {},
	"BROKER_HEALTHCHECK_TIMEOUT":  {},
}

// DeprecatedBrokerEnvVariables returns deprecated broker-related env vars currently set.
func DeprecatedBrokerEnvVariables(environ []string) []string {
	deprecated := make([]string, 0)
	seen := make(map[string]struct{})

	for _, envPair := range environ {
		const envPairParts = 2

		parts := strings.SplitN(envPair, "=", envPairParts)
		if len(parts) == 0 {
			continue
		}

		key := parts[0]

		if _, ok := deprecatedBrokerEnvKeys[key]; ok {
			if _, exists := seen[key]; !exists {
				deprecated = append(deprecated, key)
				seen[key] = struct{}{}
			}

			continue
		}

		for _, prefix := range deprecatedBrokerEnvPrefixes {
			if !strings.HasPrefix(key, prefix) {
				continue
			}

			if _, exists := seen[key]; !exists {
				deprecated = append(deprecated, key)
				seen[key] = struct{}{}
			}

			break
		}
	}

	sort.Strings(deprecated)

	return deprecated
}
