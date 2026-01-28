//go:build integration

package rabbitmq

import (
	"sync"
	"time"
	_ "unsafe" // Required for go:linkname
)

// These linknames allow tests to access private variables from pkg/utils
// for resetting the jitter configuration singleton between tests.
//
//go:linkname jitterConfigMu github.com/LerianStudio/midaz/v3/pkg/utils.configMu
var jitterConfigMu sync.Mutex

//go:linkname jitterConfigOnce github.com/LerianStudio/midaz/v3/pkg/utils.configOnce
var jitterConfigOnce sync.Once

//go:linkname jitterConfig github.com/LerianStudio/midaz/v3/pkg/utils.config
var jitterConfig struct {
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	backoffFactor  float64
}

// resetJitterConfigForTesting resets the jitter configuration singleton.
// This allows tests to modify RETRY_* environment variables and have them take effect.
func resetJitterConfigForTesting() {
	jitterConfigMu.Lock()
	defer jitterConfigMu.Unlock()
	jitterConfigOnce = sync.Once{}
	jitterConfig = struct {
		maxRetries     int
		initialBackoff time.Duration
		maxBackoff     time.Duration
		backoffFactor  float64
	}{}
}
