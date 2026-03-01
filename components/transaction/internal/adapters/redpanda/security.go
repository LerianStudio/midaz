// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
)

// ClientSecurityConfig controls optional TLS/SASL settings for Redpanda clients.
type ClientSecurityConfig = brokersecurity.Config

// BuildSecurityOptions returns franz-go options for TLS/SASL authentication.
func BuildSecurityOptions(cfg ClientSecurityConfig) ([]kgo.Opt, error) {
	options, err := brokersecurity.BuildFranzGoOptions(cfg)
	if err != nil {
		return nil, fmt.Errorf("build redpanda security options: %w", err)
	}

	return options, nil
}
