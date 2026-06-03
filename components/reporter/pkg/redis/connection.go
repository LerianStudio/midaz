// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	base "github.com/LerianStudio/lib-commons/v5/commons/redis"
	"github.com/LerianStudio/lib-observability/log"
	goRedis "github.com/redis/go-redis/v9"
)

type RedisConnection struct {
	Address                      []string
	Password                     string
	DB                           int
	Protocol                     int
	MasterName                   string
	UseTLS                       bool
	CACert                       string
	UseGCPIAMAuth                bool
	ServiceAccount               string
	GoogleApplicationCredentials string
	TokenLifeTime                time.Duration
	RefreshDuration              time.Duration
	Logger                       log.Logger

	mu     sync.Mutex
	client *base.Client
}

func (r *RedisConnection) GetClient(ctx context.Context) (goRedis.UniversalClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client == nil {
		if len(r.Address) == 0 {
			return nil, fmt.Errorf("redis address list is empty; at least one address is required")
		}

		client, err := base.New(ctx, r.toConfig())
		if err != nil {
			return nil, err
		}

		r.client = client
	}

	return r.client.GetClient(ctx)
}

func (r *RedisConnection) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client == nil {
		return nil
	}

	err := r.client.Close()
	r.client = nil

	return err
}

func (r *RedisConnection) toConfig() base.Config {
	addresses := make([]string, 0, len(r.Address))
	for _, addr := range r.Address {
		if trimmed := strings.TrimSpace(addr); trimmed != "" {
			addresses = append(addresses, trimmed)
		}
	}

	topology := base.Topology{}

	if strings.TrimSpace(r.MasterName) != "" {
		topology.Sentinel = &base.SentinelTopology{
			Addresses:  addresses,
			MasterName: r.MasterName,
		}
	} else if len(addresses) <= 1 {
		addr := ""
		if len(addresses) == 1 {
			addr = addresses[0]
		}

		topology.Standalone = &base.StandaloneTopology{Address: addr}
	} else {
		topology.Cluster = &base.ClusterTopology{Addresses: addresses}
	}

	auth := base.Auth{}
	if r.UseGCPIAMAuth {
		auth.GCPIAM = &base.GCPIAMAuth{
			CredentialsBase64: r.GoogleApplicationCredentials,
			ServiceAccount:    r.ServiceAccount,
			TokenLifetime:     r.TokenLifeTime,
			RefreshEvery:      r.RefreshDuration,
		}
	} else if r.Password != "" {
		auth.StaticPassword = &base.StaticPasswordAuth{Password: r.Password}
	}

	connOpts := base.ConnectionOptions{
		DB:       r.DB,
		Protocol: r.Protocol,
	}

	cfg := base.Config{
		Topology: topology,
		Auth:     auth,
		Options:  connOpts,
		Logger:   r.Logger,
	}

	if r.UseTLS {
		cfg.TLS = &base.TLSConfig{CACertBase64: r.CACert}
	}

	return cfg
}
