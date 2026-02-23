// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package out

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

// AuthorizerConfig configures the outbound authorizer gRPC client.
type AuthorizerConfig struct {
	Enabled    bool
	Host       string
	Port       string
	Timeout    time.Duration
	Streaming  bool
	TLSEnabled bool
}

// AuthorizerGRPCRepository is a gRPC client for the authorizer service.
type AuthorizerGRPCRepository struct {
	enabled bool
	timeout time.Duration

	conn   *grpc.ClientConn
	client authorizerv1.BalanceAuthorizerClient
	logger libLog.Logger
}

func NewAuthorizerGRPC(cfg AuthorizerConfig, logger libLog.Logger) (*AuthorizerGRPCRepository, error) {
	repo := &AuthorizerGRPCRepository{
		enabled: cfg.Enabled,
		timeout: cfg.Timeout,
		logger:  logger,
	}

	if !cfg.Enabled {
		return repo, nil
	}

	if repo.timeout <= 0 {
		repo.timeout = 100 * time.Millisecond
	}

	addr := net.JoinHostPort(cfg.Host, cfg.Port)

	var transportCredentials grpc.DialOption
	if cfg.TLSEnabled {
		transportCredentials = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12}))
	} else {
		logger.Warn("gRPC authorizer connection using insecure transport - not recommended for production")

		transportCredentials = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	conn, err := grpc.NewClient(
		addr,
		transportCredentials,
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to authorizer gRPC: %w", err)
	}

	repo.conn = conn
	repo.client = authorizerv1.NewBalanceAuthorizerClient(conn)

	logger.Infof("Connected to authorizer gRPC at %s", addr)

	return repo, nil
}

func (a *AuthorizerGRPCRepository) Enabled() bool {
	return a != nil && a.enabled && a.client != nil
}

func (a *AuthorizerGRPCRepository) Authorize(ctx context.Context, req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error) {
	if !a.Enabled() {
		return nil, fmt.Errorf("authorizer gRPC client is disabled")
	}

	var lastErr error

	for attempt := 1; attempt <= 2; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, a.timeout)
		resp, err := a.client.Authorize(callCtx, req, grpc.WaitForReady(true))

		cancel()

		if err == nil {
			return resp, nil
		}

		lastErr = err
		if !isRetryableAuthorizerError(err) || attempt == 2 {
			break
		}

		if sleepErr := sleepWithContext(ctx, 5*time.Millisecond); sleepErr != nil {
			return nil, sleepErr
		}
	}

	return nil, lastErr
}

func (a *AuthorizerGRPCRepository) LoadBalances(ctx context.Context, req *authorizerv1.LoadBalancesRequest) (*authorizerv1.LoadBalancesResponse, error) {
	if !a.Enabled() {
		return nil, fmt.Errorf("authorizer gRPC client is disabled")
	}

	var lastErr error

	for attempt := 1; attempt <= 2; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, a.timeout)
		resp, err := a.client.LoadBalances(callCtx, req, grpc.WaitForReady(true))

		cancel()

		if err == nil {
			return resp, nil
		}

		lastErr = err
		if !isRetryableAuthorizerError(err) || attempt == 2 {
			break
		}

		if sleepErr := sleepWithContext(ctx, 5*time.Millisecond); sleepErr != nil {
			return nil, sleepErr
		}
	}

	return nil, lastErr
}

func (a *AuthorizerGRPCRepository) PublishBalanceOperations(ctx context.Context, topic, partitionKey string, payload []byte, headers map[string]string) error {
	if !a.Enabled() {
		return fmt.Errorf("authorizer gRPC client is disabled")
	}

	req := &authorizerv1.PublishBalanceOperationsRequest{
		Topic:        topic,
		PartitionKey: partitionKey,
		Payload:      payload,
		ContentType:  "application/msgpack",
		Headers:      headers,
	}

	var lastErr error

	for attempt := 1; attempt <= 2; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, a.timeout)
		_, err := a.client.PublishBalanceOperations(callCtx, req, grpc.WaitForReady(true))

		cancel()

		if err == nil {
			return nil
		}

		lastErr = err
		if !isRetryableAuthorizerError(err) || attempt == 2 {
			break
		}

		if sleepErr := sleepWithContext(ctx, 5*time.Millisecond); sleepErr != nil {
			return sleepErr
		}
	}

	return lastErr
}

func isRetryableAuthorizerError(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (a *AuthorizerGRPCRepository) Close() error {
	if a == nil || a.conn == nil {
		return nil
	}

	return a.conn.Close()
}
