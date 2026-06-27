// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

// tenantCount returns the number of active tenant worker sets. Test-only
// accessor used by supervisor_test.go / supervisor_launcher_test.go to assert
// supervisor state without exporting the internal map (M7).
//
// Previously lived on the production file guarded by //nolint:unused. Moving
// it to a _test.go file makes the test/prod boundary explicit and drops the
// linter suppression.
func (s *WorkerSupervisor) tenantCount() int {
	return int(s.activeCount.Load())
}
