// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package testutils

import (
	"github.com/docker/docker/api/types/container"
)

// ApplyResourceLimits configures memory and CPU limits on a container's HostConfig.
// This is the recommended approach for testcontainers-go (HostConfigModifier).
//
// Parameters:
//   - hostConfig: The container host configuration to modify
//   - memoryMB: Memory limit in megabytes (0 = no limit)
//   - cpuLimit: CPU limit in cores, e.g., 0.5 = half a core (0 = no limit)
//
// Usage with testcontainers-go:
//
//	req := testcontainers.ContainerRequest{
//	    HostConfigModifier: func(hc *container.HostConfig) {
//	        testutils.ApplyResourceLimits(hc, 512, 1.0)
//	    },
//	}
func ApplyResourceLimits(hostConfig *container.HostConfig, memoryMB int64, cpuLimit float64) {
	if memoryMB > 0 {
		hostConfig.Memory = memoryMB * 1024 * 1024 // Convert MB to bytes
	}

	if cpuLimit > 0 {
		hostConfig.NanoCPUs = int64(cpuLimit * 1e9) // Convert cores to nanoseconds
	}
}
