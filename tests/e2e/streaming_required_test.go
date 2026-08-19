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
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
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

	t.Run("captured offset ignores existing history", func(t *testing.T) {
		const (
			topic       = "captured-offset-history"
			wantSubject = "current-action"
			historySize = 5000
		)

		cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, topic))
		if err != nil {
			t.Fatalf("start fake broker: %v", err)
		}
		t.Cleanup(cluster.Close)
		t.Setenv("STREAMING_BROKERS", strings.Join(cluster.ListenAddrs(), ","))

		producer, err := kgo.NewClient(kgo.SeedBrokers(cluster.ListenAddrs()...))
		if err != nil {
			t.Fatalf("create fake-broker producer: %v", err)
		}
		t.Cleanup(producer.Close)

		history := make([]*kgo.Record, historySize)
		for i := range history {
			history[i] = &kgo.Record{Topic: topic, Value: []byte(`{"history":true}`)}
		}

		produceCtx, produceCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer produceCancel()
		if err := producer.ProduceSync(produceCtx, history...).FirstErr(); err != nil {
			t.Fatalf("seed broker history: %v", err)
		}

		baseline, err := kgo.NewClient(
			kgo.SeedBrokers(cluster.ListenAddrs()...),
			kgo.ConsumeTopics(topic),
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		)
		if err != nil {
			t.Fatalf("create from-start baseline consumer: %v", err)
		}
		t.Cleanup(baseline.Close)

		capture := strmCaptureFromEnd(t, topic)
		target := &kgo.Record{
			Topic: topic,
			Value: []byte(`{"current":true}`),
			Headers: []kgo.RecordHeader{
				{Key: strmHeaderCESubject, Value: []byte(wantSubject)},
				{Key: strmHeaderCEType, Value: []byte(strmCETypePrefix + "test.created")},
			},
		}

		if err := producer.ProduceSync(produceCtx, target).FirstErr(); err != nil {
			t.Fatalf("produce current action: %v", err)
		}

		baselineStarted := time.Now()
		baselineScanned := consumeSubjectForBaseline(t, baseline, wantSubject, 5*time.Second)
		baselineDuration := time.Since(baselineStarted)

		started := time.Now()
		_, _, payload, found := capture.ConsumeMatch(t, wantSubject, 5*time.Second)
		captureDuration := time.Since(started)
		if !found {
			t.Fatal("captured consumer did not find current action")
		}
		if capture.scanned != 1 {
			t.Fatalf("captured consumer scanned %d records, want 1 after %d historical records", capture.scanned, historySize)
		}
		if current, ok := payload["current"].(bool); !ok || !current {
			t.Fatalf("captured payload = %v, want current action", payload)
		}

		t.Logf("streaming history baseline: scanned=%d duration=%s; captured offset: scanned=%d duration=%s",
			baselineScanned, baselineDuration, capture.scanned, captureDuration)
	})
}

func consumeSubjectForBaseline(t *testing.T, client *kgo.Client, wantSubject string, timeout time.Duration) int {
	t.Helper()

	deadline := time.Now().Add(timeout)
	scanned := 0

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		fetches := client.PollFetches(ctx)
		cancel()

		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()
			scanned++

			if subject, ok := strmHeader(record, strmHeaderCESubject); ok && subject == wantSubject {
				return scanned
			}
		}
	}

	t.Fatalf("from-start baseline did not find subject %s", wantSubject)

	return scanned
}
