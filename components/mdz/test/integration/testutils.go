//go:build integration
// +build integration

package integration

import (
	"bytes"
	"os/exec"
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
func cmdRun(t *testing.T, cmd *exec.Cmd, stderr bytes.Buffer) {
	if err := cmd.Run(); err != nil {
		t.Fatalf("Error executing command: %v\nStderr: %s", err, stderr.String())
	}
}
