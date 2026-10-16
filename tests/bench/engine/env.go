//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	benchUser      = "bench"
	benchDatabase  = "bench_transaction"
	connectTimeout = 10 * time.Second
	// lockTimeout turns a lock held by an unexpected session into a bounded failure instead of a hang.
	lockTimeout = "30s"
)

var errBenchEnvUnset = errors.New("bench environment not configured")

// benchEnv is the disposable compose stack. The configs only allow loopback; the SQL guard proves the rest.
type benchEnv struct {
	primary     *pgx.ConnConfig
	clientProxy *pgx.ConnConfig
	standby     *pgx.ConnConfig
	toxiproxy   *url.URL
}

// loadBenchEnv returns errBenchEnvUnset only when all four variables are empty.
func loadBenchEnv() (*benchEnv, error) {
	names := [...]string{"BENCH_PG_PRIMARY_DSN", "BENCH_PG_CLIENT_PROXY_DSN", "BENCH_PG_STANDBY_DSN", "BENCH_TOXIPROXY_URL"}

	var (
		values  [len(names)]string
		missing []string
	)

	for i, name := range names {
		values[i] = os.Getenv(name)
		if values[i] == "" {
			missing = append(missing, name)
		}
	}

	switch len(missing) {
	case 0:
	case len(names):
		return nil, errBenchEnvUnset
	default:
		return nil, fmt.Errorf("partial bench environment, missing %s", strings.Join(missing, ", "))
	}

	env := &benchEnv{}
	for i, target := range []**pgx.ConnConfig{&env.primary, &env.clientProxy, &env.standby} {
		cfg, err := parseBenchDSN(names[i], values[i])
		if err != nil {
			return nil, err
		}

		*target = cfg
	}

	toxiproxy, err := url.Parse(values[3])
	// Neither the URL nor url.Error is echoed: both may carry credentials.
	if err != nil || toxiproxy.Scheme != "http" || toxiproxy.User != nil || toxiproxy.Port() == "" || !isLoopbackIP(toxiproxy.Hostname()) {
		return nil, fmt.Errorf("%s: want http://<loopback ip>:<port> without credentials", names[3])
	}

	env.toxiproxy = toxiproxy

	return env, nil
}

// parseBenchDSN validates the config pgx will actually dial, after query overrides and PG* env defaults.
func parseBenchDSN(name, dsn string) (*pgx.ConnConfig, error) {
	cfg, err := pgx.ParseConfig(dsn) // ParseConfigError redacts the password
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	if !isLoopbackIP(cfg.Host) {
		return nil, fmt.Errorf("%s: host %q is not a loopback IP literal", name, cfg.Host)
	}

	// sslmode prefer/allow adds TLS variants of the same endpoint; any other fallback dials elsewhere.
	for _, fallback := range cfg.Fallbacks {
		if fallback.Host != cfg.Host || fallback.Port != cfg.Port {
			return nil, fmt.Errorf("%s: fallback %s:%d leaves %s:%d", name, fallback.Host, fallback.Port, cfg.Host, cfg.Port)
		}
	}

	if cfg.User != benchUser || cfg.Database != benchDatabase {
		return nil, fmt.Errorf("%s: user %q database %q, want %q %q", name, cfg.User, cfg.Database, benchUser, benchDatabase)
	}

	cfg.ConnectTimeout = connectTimeout
	cfg.RuntimeParams["lock_timeout"] = lockTimeout

	return cfg, nil
}

func isLoopbackIP(host string) bool {
	addr, err := netip.ParseAddr(host)

	return err == nil && addr.IsLoopback()
}
