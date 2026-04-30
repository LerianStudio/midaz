// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

var (
	organizationAllowedStatuses = []string{"ACTIVE", "INACTIVE"}
	ledgerAllowedStatuses       = []string{"ACTIVE", "INACTIVE"}
	accountAllowedStatuses      = []string{"ACTIVE", "INACTIVE", "BLOCKED"}
)

func isValidStatus(status string, allowed []string) bool {
	for _, allowedStatus := range allowed {
		if status == allowedStatus {
			return true
		}
	}

	return false
}
