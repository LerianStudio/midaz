package helpers

import (
	"fmt"
	"strings"
	"time"
)

// StartLogCapture marks a start timestamp and returns a function that writes docker logs
// for the given containers since that timestamp to ./reports/logs/<container>_<testName>.log.
// Intended for use within tests; paths are relative to the package CWD (e.g., tests/chaos).
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
