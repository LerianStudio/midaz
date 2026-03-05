// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mgrpc

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// Peer authentication header constants used by both the authorizer server-side
// verifier and all outbound gRPC clients that sign requests with HMAC.
const (
	PeerAuthTimestampHeader = "x-midaz-peer-ts"
	PeerAuthNonceHeader     = "x-midaz-peer-nonce"
	PeerAuthMethodHeader    = "x-midaz-peer-method"
	PeerAuthBodyHashHeader  = "x-midaz-peer-body-sha256"
	PeerAuthSignatureHeader = "x-midaz-peer-signature"
)

// WithPeerAuth attaches HMAC-based peer authentication metadata to an outgoing
// gRPC context. When token is empty the context is returned unmodified.
func WithPeerAuth(ctx context.Context, token, method string, body proto.Message) (context.Context, error) {
	if token == "" {
		return ctx, nil
	}

	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)

	nonce, err := GeneratePeerNonce()
	if err != nil {
		return nil, fmt.Errorf("generate peer nonce: %w", err)
	}

	bodyHash, err := HashPeerAuthBody(body)
	if err != nil {
		return nil, fmt.Errorf("hash peer auth body: %w", err)
	}

	signature := SignPeerAuth(token, timestamp, nonce, method, bodyHash)

	return metadata.AppendToOutgoingContext(
		ctx,
		PeerAuthTimestampHeader, timestamp,
		PeerAuthNonceHeader, nonce,
		PeerAuthMethodHeader, method,
		PeerAuthBodyHashHeader, bodyHash,
		PeerAuthSignatureHeader, signature,
	), nil
}

// SignPeerAuth computes the HMAC-SHA256 signature over the canonical
// representation: timestamp\nnonce\nmethod\nbodyHash.
func SignPeerAuth(token, timestamp, nonce, method, bodyHash string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(nonce))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(method))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(bodyHash))

	return hex.EncodeToString(mac.Sum(nil))
}

// HashPeerAuthBody returns the hex-encoded SHA-256 digest of the deterministic
// protobuf serialization of body. A nil message hashes to SHA-256 of the empty
// byte slice. Returns an error if proto marshalling fails.
func HashPeerAuthBody(body proto.Message) (string, error) {
	if body == nil {
		digest := sha256.Sum256(nil)
		return hex.EncodeToString(digest[:]), nil
	}

	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal peer auth body: %w", err)
	}

	digest := sha256.Sum256(payload)

	return hex.EncodeToString(digest[:]), nil
}

// nonceSize is the number of random bytes used for peer authentication nonces (128-bit).
const nonceSize = 16

// GeneratePeerNonce returns a cryptographically random 128-bit nonce encoded as
// a 32-character hex string.
func GeneratePeerNonce() (string, error) {
	raw := make([]byte, nonceSize)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("crypto/rand read failed: %w", err)
	}

	return hex.EncodeToString(raw), nil
}
