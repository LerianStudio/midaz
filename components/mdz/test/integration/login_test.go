//go:build integration
// +build integration

package integration

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Netflix/go-expect"
	"gotest.tools/golden"
)

// \1 performs an operation
func TestMDZLogin(t *testing.T) {
	cmd := exec.Command("mdz", "login", "--username", "user_john", "--password", "Lerian@123")

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout

	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Error executing command: %v\nStderr: %s", err, stderr.String())
	}

	golden.AssertBytes(t, []byte(stdout.String()), "out_login_flags.golden")
}

// \1 performs an operation
func TestMDZLoginIt(t *testing.T) {
	console, err := expect.NewConsole(expect.WithStdout(os.Stdout))

	if err != nil {
		t.Fatalf("Error creating console: %v", err)
	}

	defer console.Close()

	cmd := exec.Command("mdz", "login")

	cmd.Stdin = console.Tty()

	cmd.Stdout = console.Tty()

	cmd.Stderr = console.Tty()

	errChan := make(chan error, 1)

	go func() {
		if err := cmd.Start(); err != nil {
			errChan <- err
			return
		}

		if err := cmd.Wait(); err != nil {
			errChan <- err
			return
		}

		errChan <- nil
	}()

	time.Sleep(1 * time.Second)
	down(t, console)
	enter(t, console)
	sendInput(t, console, "user_john")
	sendInput(t, console, "Lerian@123")
	time.Sleep(1 * time.Second)

	if _, err := console.ExpectString("successfully logged in"); err != nil {
		t.Fatalf("Failed test: %v", err)
	}

	if err := <-errChan; err != nil {
		t.Fatalf("Command execution error: %v", err)
	}
}
