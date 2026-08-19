//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"net/http"
	"net/url"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestReusableContainerIsolatesVirtualHosts(t *testing.T) {
	first := SetupReusableContainer(t)
	second := SetupReusableContainer(t)

	require.Equal(t, first.Container.GetContainerID(), second.Container.GetContainerID())
	require.NotEqual(t, first.VHost, second.VHost)

	_, err := first.Channel.QueueDeclare("owned", false, false, false, false, nil)
	require.NoError(t, err)
	_, err = second.Channel.QueueDeclare("owned", false, false, false, false, nil)
	require.NoError(t, err)
	require.NoError(t, first.Channel.Publish("", "owned", false, false, amqp.Publishing{Body: []byte("first")}))

	message, ok, err := first.Channel.Get("owned", true)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "first", string(message.Body))

	_, ok, err = second.Channel.Get("owned", true)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestReusableContainerDropsTheOwningTestsVirtualHost(t *testing.T) {
	observer := SetupReusableContainer(t)

	var ownedVHost string
	require.True(t, t.Run("owner", func(t *testing.T) {
		owned := SetupReusableContainer(t)
		ownedVHost = owned.VHost
	}))

	status, err := rabbitManagementRequest(
		t.Context(),
		DefaultContainerConfig(),
		observer.Host,
		observer.MgmtPort,
		http.MethodGet,
		"/api/vhosts/"+url.PathEscape(ownedVHost),
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, status)
}
