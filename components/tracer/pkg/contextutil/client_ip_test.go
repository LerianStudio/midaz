// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package contextutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetClientIP_NotSet(t *testing.T) {
	ctx := context.Background()
	ip := GetClientIP(ctx)
	assert.Equal(t, "0.0.0.0", ip, "Should return default when not set")
}

func TestGetClientIP_EmptyString(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyClientIP{}, "")
	ip := GetClientIP(ctx)
	assert.Equal(t, "0.0.0.0", ip, "Should return default when empty string")
}

func TestGetClientIP_ValidIPv4(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyClientIP{}, "192.168.1.50")
	ip := GetClientIP(ctx)
	assert.Equal(t, "192.168.1.50", ip, "Should return stored IPv4")
}

func TestGetClientIP_ValidIPv6(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyClientIP{}, "2001:db8::1")
	ip := GetClientIP(ctx)
	assert.Equal(t, "2001:db8::1", ip, "Should return stored IPv6")
}

func TestGetClientIP_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyClientIP{}, 12345)
	ip := GetClientIP(ctx)
	assert.Equal(t, "0.0.0.0", ip, "Should return default when value is not string")
}

func TestGetClientIP_WrongKey(t *testing.T) {
	ctx := context.WithValue(context.Background(), "other_key", "192.168.1.1")
	ip := GetClientIP(ctx)
	assert.Equal(t, "0.0.0.0", ip, "Should return default when key is different")
}
