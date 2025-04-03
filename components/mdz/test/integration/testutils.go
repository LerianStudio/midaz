//go:build integration
// +build integration

package integration

import (
	"bytes"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/Netflix/go-expect"
)

const interactionDelay = 500 * time.Millisecond

// up Send up arrow
// func up(t *testing.T, console *expect.Console) {
// 	_, err := console.Send("\x1b[A")
// 	if err != nil {
// 		t.Fatal(err.Error())
// 	}
//
// 	time.Sleep(interactionDelay)
// }

// down send down arrow
func down(t *testing.T, console *expect.Console) {
	_, err := console.Send("\x1b[B")

	if err != nil {
		t.Fatal(err.Error())
	}

	time.Sleep(interactionDelay)
}

// enter Send Enter key
func enter(t *testing.T, console *expect.Console) {
	_, err := console.Send("\r")

	if err != nil {
		t.Fatal(err.Error())
	}

	time.Sleep(interactionDelay)
}

// sendInput Send a value and then Enter
func sendInput(t *testing.T, console *expect.Console, input string) {
	_, err := console.Send(input + "\r")

	if err != nil {
		t.Fatal(err.Error())
	}

	time.Sleep(interactionDelay)
}

// cmdRun run command and check error
func cmdRun(t *testing.T, cmd *exec.Cmd) (string, string) {
	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout

	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Error executing command: %v\nStderr: %s", err, stderr.String())
	}

	t.Log(cmd.Stdout)
	t.Log(cmd.Stderr)

	return stdout.String(), stderr.String()
}

// getIDListOutput get id list command
func getIDListOutput(t *testing.T, stdout string) string {
	re := regexp.MustCompile(`[0-9a-fA-F-]{36}`)
	id := re.FindString(stdout)

	if id == "" {
		t.Fatal("No ID found in output")
	}

	return id
}
