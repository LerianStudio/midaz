// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// FaultInjectionHeader is the header used to trigger fault injection in tests.
// Only works when FAULT_INJECTION_ENABLED=true (integration test mode).
const FaultInjectionHeader = "X-Test-Fault-Injection"

// Fault injection types
const (
	FaultTimeout     = "timeout"     // Simulates 504 Gateway Timeout
	FaultUnavailable = "unavailable" // Simulates 503 Service Unavailable
)

// DefaultTestServerURL is the base URL the integration test helpers fall back
// to when neither SERVER_ADDRESS nor SERVER_PORT is present in the environment.
// Centralized here (instead of embedded inline) so that if the service port
// ever changes again the update happens in a single, greppable location.
// Production code must NOT reference this — it is a test-harness last resort.
const DefaultTestServerURL = "http://localhost:4020"
