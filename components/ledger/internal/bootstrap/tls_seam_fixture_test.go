// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// seamCertFiles holds on-disk paths to a CA-signed client leaf plus the CA
// bundle, suitable for feeding TRACER_TLS_CERT_FILE / TRACER_TLS_KEY_FILE /
// TRACER_TLS_CA_FILE in the boot-guard tests. The guard only needs the material
// to LOAD (lib-commons NewManager validates the key matches the cert; the CA
// pool must parse), not to complete a handshake, so a self-consistent fixture
// is enough.
type seamCertFiles struct {
	certFile string
	keyFile  string
	caFile   string
}

// fixtureNotBefore / fixtureNotAfter bound the validity window of the fixture
// certificates. They are FIXED constants (no time.Now in tests) chosen to
// straddle any realistic test wall-clock.
var (
	fixtureNotBefore = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	fixtureNotAfter  = time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)
)

// writeSeamCertFiles generates a self-signed CA and a client leaf signed by it,
// writing cert/key/CA PEM files into t.TempDir() and returning their paths.
func writeSeamCertFiles(t *testing.T) seamCertFiles {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "midaz-ledger-seam-test-ca"},
		NotBefore:             fixtureNotBefore,
		NotAfter:              fixtureNotAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "ledger-seam-client"},
		NotBefore:    fixtureNotBefore,
		NotAfter:     fixtureNotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	require.NoError(t, err)

	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	require.NoError(t, err)

	dir := t.TempDir()
	files := seamCertFiles{
		certFile: filepath.Join(dir, "client.crt"),
		keyFile:  filepath.Join(dir, "client.key"),
		caFile:   filepath.Join(dir, "ca.crt"),
	}

	writePEM(t, files.certFile, "CERTIFICATE", clientDER)
	writePEM(t, files.keyFile, "EC PRIVATE KEY", clientKeyDER)
	writePEM(t, files.caFile, "CERTIFICATE", caDER)

	return files
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()

	data := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	require.NoError(t, os.WriteFile(path, data, 0o600))
}
