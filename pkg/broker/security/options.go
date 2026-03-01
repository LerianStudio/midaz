// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package security

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// Sentinel errors for TLS/SASL configuration validation.
var (
	ErrTLSCAFileNotAFile        = errors.New("redpanda tls ca file must be a file path")
	ErrTLSCANoCertificates      = errors.New("parse redpanda tls ca file: no valid certificates found")
	ErrSASLCredentialsMissing   = errors.New("redpanda sasl enabled but username/password are empty")
	ErrUnsupportedSASLMechanism = errors.New("unsupported redpanda sasl mechanism")
)

// Config controls optional TLS/SASL settings for Redpanda franz-go clients.
type Config struct {
	TLSEnabled            bool
	TLSInsecureSkipVerify bool
	TLSCAFile             string
	SASLEnabled           bool
	SASLMechanism         string
	SASLUsername          string
	SASLPassword          string
}

// BuildFranzGoOptions returns franz-go options for TLS/SASL authentication.
func BuildFranzGoOptions(cfg Config) ([]kgo.Opt, error) {
	const initialCap = 2

	options := make([]kgo.Opt, 0, initialCap)

	if cfg.TLSEnabled {
		tlsConfig, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, err
		}

		options = append(options, kgo.DialTLSConfig(tlsConfig))
	}

	if cfg.SASLEnabled {
		mechanism, err := buildSASLMechanism(cfg)
		if err != nil {
			return nil, err
		}

		options = append(options, kgo.SASL(mechanism))
	}

	return options, nil
}

func buildTLSConfig(cfg Config) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify, //nolint:gosec // controlled by operator configuration
	}

	if strings.TrimSpace(cfg.TLSCAFile) == "" {
		return tlsConfig, nil
	}

	caPath := filepath.Clean(cfg.TLSCAFile)

	info, err := os.Stat(caPath)
	if err != nil {
		return nil, fmt.Errorf("stat redpanda tls ca file: %w", err)
	}

	if info.IsDir() {
		return nil, ErrTLSCAFileNotAFile
	}

	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read redpanda tls ca file: %w", err)
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system cert pool: %w", err)
	}

	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	if ok := rootCAs.AppendCertsFromPEM(caPEM); !ok {
		return nil, ErrTLSCANoCertificates
	}

	tlsConfig.RootCAs = rootCAs

	return tlsConfig, nil
}

func buildSASLMechanism(cfg Config) (sasl.Mechanism, error) {
	if strings.TrimSpace(cfg.SASLUsername) == "" || strings.TrimSpace(cfg.SASLPassword) == "" {
		return nil, ErrSASLCredentialsMissing
	}

	mechanism := strings.ToUpper(strings.TrimSpace(cfg.SASLMechanism))
	if mechanism == "" {
		mechanism = "SCRAM-SHA-256"
	}

	auth := scram.Auth{User: cfg.SASLUsername, Pass: cfg.SASLPassword}

	switch mechanism {
	case "PLAIN":
		return plain.Auth{User: cfg.SASLUsername, Pass: cfg.SASLPassword}.AsMechanism(), nil
	case "SCRAM-SHA-256":
		return auth.AsSha256Mechanism(), nil
	case "SCRAM-SHA-512":
		return auth.AsSha512Mechanism(), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedSASLMechanism, cfg.SASLMechanism)
	}
}
