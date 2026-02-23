//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

const (
	kafkaPort = "9092/tcp"
	adminPort = "9644/tcp"
)

// ContainerResult contains Redpanda container connection information.
type ContainerResult struct {
	Container testcontainers.Container
	Host      string
	KafkaPort string
	AdminPort string
	Brokers   []string
}

func reserveHostPort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)

	defer func() {
		_ = listener.Close()
	}()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	return strconv.Itoa(tcpAddr.Port)
}

// SetupContainer starts a Redpanda container for integration tests.
func SetupContainer(t *testing.T) *ContainerResult {
	t.Helper()

	ctx := context.Background()
	kafkaHostPort := reserveHostPort(t)
	adminHostPort := reserveHostPort(t)

	req := testcontainers.ContainerRequest{
		Image: "docker.redpanda.com/redpandadata/redpanda:latest",
		ExposedPorts: []string{
			fmt.Sprintf("%s:%s", kafkaHostPort, kafkaPort),
			fmt.Sprintf("%s:%s", adminHostPort, adminPort),
		},
		Cmd: []string{
			"redpanda",
			"start",
			"--overprovisioned",
			"--smp", "1",
			"--memory", "1G",
			"--reserve-memory", "0M",
			"--check=false",
			"--node-id", "0",
			"--kafka-addr", "PLAINTEXT://0.0.0.0:9092",
			"--advertise-kafka-addr", fmt.Sprintf("PLAINTEXT://127.0.0.1:%s", kafkaHostPort),
		},
		WaitingFor: wait.ForLog("Successfully started Redpanda!").WithStartupTimeout(180 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	mappedKafka, err := container.MappedPort(ctx, kafkaPort)
	require.NoError(t, err)

	mappedAdmin, err := container.MappedPort(ctx, adminPort)
	require.NoError(t, err)

	result := &ContainerResult{
		Container: container,
		Host:      host,
		KafkaPort: mappedKafka.Port(),
		AdminPort: mappedAdmin.Port(),
		Brokers:   []string{fmt.Sprintf("127.0.0.1:%s", mappedKafka.Port())},
	}

	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate Redpanda container: %v", err)
		}
	})

	return result
}

// SetupTopics creates topics in the test Redpanda container.
func SetupTopics(t *testing.T, result *ContainerResult, topics ...string) {
	t.Helper()

	require.NotNil(t, result)

	client, err := kgo.NewClient(kgo.SeedBrokers(result.Brokers...))
	require.NoError(t, err)

	t.Cleanup(client.Close)

	for _, topic := range topics {
		trimmed := strings.TrimSpace(topic)
		if trimmed == "" {
			continue
		}

		var lastErr error
		ctx := context.Background()

		for attempt := range 20 {
			topicReq := kmsg.NewCreateTopicsRequestTopic()
			topicReq.Topic = trimmed
			topicReq.NumPartitions = 1
			topicReq.ReplicationFactor = 1

			req := kmsg.NewPtrCreateTopicsRequest()
			req.Topics = append(req.Topics, topicReq)
			req.TimeoutMillis = int32((5 * time.Second).Milliseconds())

			resp, requestErr := req.RequestWith(ctx, client)
			if requestErr != nil {
				lastErr = fmt.Errorf("create topic %s request failed: %w", trimmed, requestErr)
			} else if len(resp.Topics) != 1 {
				lastErr = fmt.Errorf("create topic %s returned unexpected topic count: %d", trimmed, len(resp.Topics))
			} else {
				resultTopic := resp.Topics[0]
				if resultTopic.ErrorCode == 0 {
					lastErr = nil
					break
				}

				topicErr := kerr.ErrorForCode(resultTopic.ErrorCode)
				if errors.Is(topicErr, kerr.TopicAlreadyExists) {
					lastErr = nil
					break
				}

				if resultTopic.ErrorMessage != nil {
					lastErr = fmt.Errorf("create topic %s failed: %w (%s)", trimmed, topicErr, *resultTopic.ErrorMessage)
				} else {
					lastErr = fmt.Errorf("create topic %s failed: %w", trimmed, topicErr)
				}
			}

			if attempt < 19 {
				time.Sleep(300 * time.Millisecond)
			}
		}

		require.NoError(t, lastErr)

		var readinessErr error
		for attempt := range 20 {
			topicName := trimmed
			metadataReq := kmsg.NewPtrMetadataRequest()
			metadataTopic := kmsg.NewMetadataRequestTopic()
			metadataTopic.Topic = &topicName
			metadataReq.Topics = append(metadataReq.Topics, metadataTopic)

			metadataResp, err := metadataReq.RequestWith(ctx, client)
			if err != nil {
				readinessErr = fmt.Errorf("metadata request for topic %s failed: %w", trimmed, err)
			} else if len(metadataResp.Topics) != 1 {
				readinessErr = fmt.Errorf("metadata request for topic %s returned unexpected topic count: %d", trimmed, len(metadataResp.Topics))
			} else {
				topicMeta := metadataResp.Topics[0]
				topicErr := kerr.ErrorForCode(topicMeta.ErrorCode)
				if topicMeta.ErrorCode == 0 && len(topicMeta.Partitions) > 0 {
					allLeadersReady := true
					for _, partition := range topicMeta.Partitions {
						if partition.Leader < 0 || partition.ErrorCode != 0 {
							allLeadersReady = false
							break
						}
					}

					if allLeadersReady {
						readinessErr = nil
						break
					}

					readinessErr = fmt.Errorf("topic %s partitions are not leader-ready yet", trimmed)
				} else {
					readinessErr = fmt.Errorf("topic %s metadata not ready: %w", trimmed, topicErr)
				}
			}

			if attempt < 19 {
				time.Sleep(300 * time.Millisecond)
			}
		}

		require.NoError(t, readinessErr)
	}
}
