package itestkit

import (
	"context"
	"time"
)

type ChaosConfig struct {
	Enabled bool
	Image   string // opcional
}

type ProxyRef struct {
	Name       string
	ListenAddr string // host:port para o app usar
	Upstream   string // host:port real
}

type ChaosInterface interface {
	CreateProxy(ctx context.Context, name string, upstream string) (ProxyRef, error)
	AddLatency(ctx context.Context, proxyName string, latency, jitter time.Duration) error
	AddTimeout(ctx context.Context, proxyName string, timeout time.Duration) error
	AddBandwidth(ctx context.Context, proxyName string, rateKBps int64) error
	CutConnection(ctx context.Context, proxyName string) error
	RemoveToxic(ctx context.Context, proxyName, toxicName string) error
	RemoveAllToxics(ctx context.Context, proxyName string) error
	Close(ctx context.Context) error
}
