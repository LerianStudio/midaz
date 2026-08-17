// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestStreamingBrokerRequiredModeFailsInsteadOfSkipping(t *testing.T) {
	if os.Getenv("E2E_STREAMING_BROKER_CHILD") == "1" {
		t.Setenv("E2E_REQUIRED", "1")
		t.Setenv("STREAMING_BROKERS", "127.0.0.1:1")
		strmRequireBroker(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestStreamingBrokerRequiredModeFailsInsteadOfSkipping$", "-test.v")
	cmd.Env = append(os.Environ(), "E2E_STREAMING_BROKER_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("required streaming broker subprocess passed; output:\n%s", out)
	}
	if !strings.Contains(string(out), "required streaming broker unavailable") {
		t.Fatalf("required streaming broker failure was not actionable; output:\n%s", out)
	}
}

func TestStreamingBrokerLocalModeStillSkips(t *testing.T) {
	if os.Getenv("E2E_STREAMING_BROKER_LOCAL_CHILD") == "1" {
		t.Setenv("E2E_REQUIRED", "0")
		t.Setenv("STREAMING_BROKERS", "127.0.0.1:1")
		strmRequireBroker(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestStreamingBrokerLocalModeStillSkips$", "-test.v")
	cmd.Env = append(os.Environ(), "E2E_STREAMING_BROKER_LOCAL_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("local streaming broker subprocess failed instead of skipping: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "SKIP") {
		t.Fatalf("local streaming broker subprocess did not skip; output:\n%s", out)
	}
}

func TestStreamingTopicAdminFailureFailsRequiredMode(t *testing.T) {
	if os.Getenv("E2E_STREAMING_ADMIN_CHILD") == "1" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen for streaming admin failure probe: %v", err)
		}
		defer func() { _ = listener.Close() }()

		t.Setenv("E2E_REQUIRED", "1")
		t.Setenv("STREAMING_BROKERS", listener.Addr().String())
		strmProvisionTopics = func(context.Context, []string) error {
			return errors.New("topic admin denied")
		}
		strmRequireBroker(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestStreamingTopicAdminFailureFailsRequiredMode$", "-test.v")
	cmd.Env = append(os.Environ(), "E2E_STREAMING_ADMIN_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("required streaming admin subprocess passed; output:\n%s", out)
	}
	if !strings.Contains(string(out), "topic admin denied") {
		t.Fatalf("required streaming admin failure was not surfaced; output:\n%s", out)
	}
}

func TestStreamingTopicProvisioningReturnsAdminFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := strmEnsureTopics(ctx, []string{"127.0.0.1:1"})
	if err == nil {
		t.Fatal("topic provisioning returned nil after its admin context was canceled")
	}
	if !strings.Contains(err.Error(), "create streaming topics") {
		t.Fatalf("topic provisioning error = %v, want create streaming topics context", err)
	}
}
