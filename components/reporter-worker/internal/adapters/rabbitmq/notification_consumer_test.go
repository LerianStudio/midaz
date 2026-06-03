// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubNotificationHandler is a minimal test double for NotificationHandler.
type stubNotificationHandler struct {
	returnErr error
	called    bool
	lastBody  []byte
}

func (s *stubNotificationHandler) ProcessFetcherNotification(_ context.Context, body []byte) error {
	s.called = true
	s.lastBody = body

	return s.returnErr
}

func TestNewNotificationConsumerHandler(t *testing.T) {
	t.Parallel()

	logger := log.NewNop()
	handler := &stubNotificationHandler{}

	h := NewNotificationConsumerHandler(handler, logger)

	require.NotNil(t, h)
	assert.Equal(t, handler, h.handler)
	assert.Equal(t, logger, h.logger)
}

func TestNewNotificationConsumerHandler_NilHandler(t *testing.T) {
	t.Parallel()

	h := NewNotificationConsumerHandler(nil, log.NewNop())

	require.NotNil(t, h)
	assert.Nil(t, h.handler)
}

func TestNotificationConsumerHandler_Handle_Success(t *testing.T) {
	t.Parallel()

	handler := &stubNotificationHandler{}
	h := NewNotificationConsumerHandler(handler, log.NewNop())

	body := []byte(`{"status":"completed"}`)

	err := h.Handle(context.Background(), body)

	require.NoError(t, err)
	assert.True(t, handler.called)
	assert.Equal(t, body, handler.lastBody)
}

func TestNotificationConsumerHandler_Handle_Error(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("processing failed")
	handler := &stubNotificationHandler{returnErr: expectedErr}
	h := NewNotificationConsumerHandler(handler, log.NewNop())

	err := h.Handle(context.Background(), []byte(`{}`))

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.True(t, handler.called)
}

func TestNotificationConsumerHandler_Handle_NilBody(t *testing.T) {
	t.Parallel()

	handler := &stubNotificationHandler{}
	h := NewNotificationConsumerHandler(handler, log.NewNop())

	err := h.Handle(context.Background(), nil)

	require.NoError(t, err)
	assert.True(t, handler.called)
	assert.Nil(t, handler.lastBody)
}

func TestNotificationConsumerHandler_Handle_EmptyBody(t *testing.T) {
	t.Parallel()

	handler := &stubNotificationHandler{}
	h := NewNotificationConsumerHandler(handler, log.NewNop())

	err := h.Handle(context.Background(), []byte{})

	require.NoError(t, err)
	assert.True(t, handler.called)
	assert.Empty(t, handler.lastBody)
}
