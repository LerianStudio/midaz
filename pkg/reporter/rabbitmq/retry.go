// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import "time"

// SleepFunc is a function type for sleeping between retries.
// Injecting a no-op or recording implementation in tests enables
// deterministic behavior without real delays.
type SleepFunc func(time.Duration)
