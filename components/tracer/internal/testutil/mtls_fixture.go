// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// MTLSFixture holds PEM-encoded certificate material for exercising the
// reservation seam's mutual-TLS handshake in tests. The CA signs both the
// server and the client leaf, so a server configured with CACertPEM as its
// ClientCAs accepts ClientCertPEM/ClientKeyPEM and rejects anything not chained
// to this CA.
type MTLSFixture struct {
	CACertPEM     []byte
	ServerCertPEM []byte
	ServerKeyPEM  []byte
	ClientCertPEM []byte
	ClientKeyPEM  []byte
}

// mtlsFixtureNotBefore / mtlsFixtureNotAfter bound the validity window of every
// fixture certificate. They are FIXED constants (no time.Now in tests) chosen
// to straddle any realistic test wall-clock: NotBefore sits in 2020 and
// NotAfter in 2100, so the certs are valid during the real TLS handshake that
// crypto/tls performs against the system clock while staying deterministic.
var (
	mtlsFixtureNotBefore = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	mtlsFixtureNotAfter  = time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)
)

// GenerateMTLSFixture builds a self-signed CA and a server + client leaf signed
// by it, returning their PEM encodings. The server leaf carries
// localhost/127.0.0.1 SANs so a client verifying ServerName="localhost" against
// CACertPEM succeeds. ECDSA P-256 keys keep generation fast and deterministic
// in shape; the validity window is fixed (no time.Now).
func GenerateMTLSFixture(t *testing.T) MTLSFixture {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "midaz-seam-test-ca"},
		NotBefore:             mtlsFixtureNotBefore,
		NotAfter:              mtlsFixtureNotAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	serverCertPEM, serverKeyPEM := signLeaf(t, caCert, caKey, leafSpec{
		commonName:  "tracer-seam-server",
		serial:      2,
		serverAuth:  true,
		dnsNames:    []string{"localhost"},
		ipAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	})

	clientCertPEM, clientKeyPEM := signLeaf(t, caCert, caKey, leafSpec{
		commonName: "ledger-seam-client",
		serial:     3,
		clientAuth: true,
	})

	return MTLSFixture{
		CACertPEM:     pemEncode("CERTIFICATE", caDER),
		ServerCertPEM: serverCertPEM,
		ServerKeyPEM:  serverKeyPEM,
		ClientCertPEM: clientCertPEM,
		ClientKeyPEM:  clientKeyPEM,
	}
}

type leafSpec struct {
	commonName  string
	serial      int64
	serverAuth  bool
	clientAuth  bool
	dnsNames    []string
	ipAddresses []net.IP
}

// signLeaf creates an ECDSA leaf certificate signed by the given CA and returns
// the cert and key PEM blocks.
func signLeaf(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, spec leafSpec) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(spec.serial),
		Subject:      pkix.Name{CommonName: spec.commonName},
		NotBefore:    mtlsFixtureNotBefore,
		NotAfter:     mtlsFixtureNotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		DNSNames:     spec.dnsNames,
		IPAddresses:  spec.ipAddresses,
	}

	if spec.serverAuth {
		template.ExtKeyUsage = append(template.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
	}

	if spec.clientAuth {
		template.ExtKeyUsage = append(template.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
	}

	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	return pemEncode("CERTIFICATE", der), pemEncode("EC PRIVATE KEY", keyDER)
}

// pemEncode wraps DER bytes in a PEM block of the given type.
func pemEncode(blockType string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
}
