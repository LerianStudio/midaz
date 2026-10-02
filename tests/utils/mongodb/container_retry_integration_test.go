//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const startRetryTestLabel = "studio.lerian.midaz.test.start-retry"

// failureRecorder records the helper's terminal failure instead of stopping the
// test goroutine, so the test can inspect what the failed attempts left behind.
type failureRecorder struct {
	testing.TB
	failures []string
}

func (r *failureRecorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *failureRecorder) FailNow() {}

func TestStartMongoContainerWithRetryTerminatesEveryFailedAttempt(t *testing.T) {
	ctx := context.Background()
	runID := uuid.NewString()

	dockerClient, err := testcontainers.NewDockerClientWithOpts(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dockerClient.Close() })

	labelled := func() []string {
		result, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{
			All:     true,
			Filters: make(client.Filters).Add("label", startRetryTestLabel+"="+runID),
		})
		require.NoError(t, err)

		ids := make([]string, 0, len(result.Items))
		for _, item := range result.Items {
			ids = append(ids, item.ID)
		}

		return ids
	}

	t.Cleanup(func() {
		for _, id := range labelled() {
			_, _ = dockerClient.ContainerRemove(context.Background(), id, client.ContainerRemoveOptions{Force: true})
		}
	})

	req := testcontainers.ContainerRequest{
		Image:      DefaultContainerConfig().Image,
		Labels:     map[string]string{startRetryTestLabel: runID},
		WaitingFor: wait.ForLog("this line is never logged").WithStartupTimeout(2 * time.Second),
	}

	recorder := &failureRecorder{TB: t}
	ctr := startMongoContainerWithRetry(recorder, ctx, req, "failed to start MongoDB container")

	require.Nil(t, ctr)
	require.Len(t, recorder.failures, 1)
	require.Contains(t, recorder.failures[0], "failed to start MongoDB container")
	require.Contains(t, recorder.failures[0], "start container:", "every attempt must have created a container that then failed to start")
	require.Empty(t, labelled(), "containers from failed start attempts must be terminated")
}
