// Package helpers provides reusable utilities and setup functions to streamline
// integration and end-to-end tests.
// This file contains logging utilities for test output and debugging.
package helpers

import (
	"fmt"
	"strings"
	"time"
)

// StartLogCapture returns a deferred function that captures Docker logs from a
// specified point in time, saving them to a file for debugging purposes.
//
// This is useful for capturing the logs of specific containers during a test run,
// which can be invaluable for diagnosing failures in a CI/CD environment.
func StartLogCapture(containers []string, testName string) func() {
	since := time.Now().Format(time.RFC3339)
	safeName := strings.ReplaceAll(testName, "/", "_")

	return func() {
		for _, c := range containers {
			log, _ := DockerLogsSince(c, since, 0)
			_ = WriteTextFile(fmt.Sprintf("reports/logs/%s_%s.log", c, safeName), fmt.Sprintf("--- %s logs since %s\n%s", c, since, log))
		}
	}
}
