// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSeamClientTLSConfig_ServerMode(t *testing.T) {
	t.Parallel()

	t.Run("verifies the tracer against the CA and presents no client certificate", func(t *testing.T) {
		t.Parallel()

		files := writeSeamCertFiles(t)

		cfg, err := buildSeamClientTLSConfig(&Config{TracerTLSMode: " Server ", TracerTLSCAFile: files.caFile}, "tracer")
		require.NoError(t, err)
		require.NotNil(t, cfg)

		assert.NotNil(t, cfg.RootCAs)
		assert.Empty(t, cfg.Certificates)
		assert.Nil(t, cfg.GetClientCertificate)
		assert.Equal(t, "tracer", cfg.ServerName)
		assert.GreaterOrEqual(t, cfg.MinVersion, uint16(0x0303), "TLS 1.2 or later")
	})

	t.Run("client certificate material is ignored", func(t *testing.T) {
		t.Parallel()

		files := writeSeamCertFiles(t)

		cfg, err := buildSeamClientTLSConfig(&Config{
			TracerTLSMode:     "server",
			TracerTLSCAFile:   files.caFile,
			TracerTLSCertFile: files.certFile,
			TracerTLSKeyFile:  files.keyFile,
		}, "tracer")
		require.NoError(t, err)

		assert.Empty(t, cfg.Certificates)
		assert.Nil(t, cfg.GetClientCertificate)
	})

	t.Run("refuses without a CA file", func(t *testing.T) {
		t.Parallel()

		cfg, err := buildSeamClientTLSConfig(&Config{TracerTLSMode: "server"}, "tracer")
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "TRACER_TLS_MODE=server requires TRACER_TLS_CA_FILE")
	})

	t.Run("refuses an unreadable CA file", func(t *testing.T) {
		t.Parallel()

		_, err := buildSeamClientTLSConfig(&Config{TracerTLSMode: "server", TracerTLSCAFile: t.TempDir() + "/absent.pem"}, "tracer")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TRACER_TLS_CA_FILE")
	})
}

func TestBuildSeamClientTLSConfig_UnknownModeNamesEveryAcceptedValue(t *testing.T) {
	t.Parallel()

	_, err := buildSeamClientTLSConfig(&Config{TracerTLSMode: "tls"}, "tracer")
	require.Error(t, err)

	for _, mode := range []string{`"mtls"`, `"mesh"`, `"server"`} {
		assert.Contains(t, err.Error(), mode)
	}
}
