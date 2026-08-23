// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
)

// disableScrubForTest turns the deferral on for the duration of one test and
// restores the previous value afterwards. The toggle is process-wide, so every
// test that flips it must restore it or it leaks into the rest of the package.
func disableScrubForTest(t *testing.T) {
	t.Helper()

	previous := highStatusScrubDisabled.Load()

	highStatusScrubDisabled.Store(true)

	t.Cleanup(func() { highStatusScrubDisabled.Store(previous) })
}

const (
	registryInternalTitle   = "Internal Server Error"
	registryInternalMessage = "The server encountered an unexpected error. Please try again later or contact support."
)

func TestHighStatusScrub_OnByDefault(t *testing.T) {
	require.False(t, highStatusScrubDisabled.Load(), "the scrub is on by default")

	body, ok := ProblemDetail(pkg.ValidateInternalError(errors.New("kaboom"), ""))
	require.True(t, ok)

	assert.Equal(t, http.StatusInternalServerError, body.Status)
	assert.Equal(t, http.StatusText(http.StatusInternalServerError), body.Title)
	assert.Equal(t, "internal error", body.Detail.Detail)
	assert.NotContains(t, body.Detail.Detail, "kaboom")
}

func TestHighStatusScrub_DisabledRestoresRegistryText(t *testing.T) {
	disableScrubForTest(t)

	body, ok := ProblemDetail(pkg.ValidateInternalError(errors.New("kaboom"), ""))
	require.True(t, ok)

	assert.Equal(t, http.StatusInternalServerError, body.Status)
	assert.Equal(t, registryInternalTitle, body.Title)
	assert.Equal(t, registryInternalMessage, body.Detail.Detail)

	// The bound on the E9 carve-out: what survives is STATIC catalog text, never
	// the wrapped cause. ValidateInternalError keeps the raw error in the
	// non-serialized Err field, so it cannot reach the wire through this path.
	assert.NotContains(t, body.Detail.Detail, "kaboom")
	assert.NotContains(t, body.Title, "kaboom")
}

func TestHighStatusScrub_DisabledKeepsCodeAndStatus(t *testing.T) {
	scrubbed, ok := ProblemDetail(pkg.ValidateInternalError(errors.New("boom"), ""))
	require.True(t, ok)

	disableScrubForTest(t)

	restored, ok := ProblemDetail(pkg.ValidateInternalError(errors.New("boom"), ""))
	require.True(t, ok)

	// The money path is (code, status): the toggle moves prose only.
	assert.Equal(t, scrubbed.Code, restored.Code)
	assert.Equal(t, scrubbed.Status, restored.Status)
	assert.Equal(t, scrubbed.Type, restored.Type)
}

func TestHighStatusScrub_DisabledLeavesUnmappedErrorsGeneric(t *testing.T) {
	disableScrubForTest(t)

	// An error that matches no arm of the cascade has no captured registry text,
	// so MapError's fallback stands and the generic 500 stays generic.
	body, ok := ProblemDetail(errors.New("something the classifier does not know"))
	require.True(t, ok)

	assert.Equal(t, http.StatusInternalServerError, body.Status)
	assert.Equal(t, http.StatusText(http.StatusInternalServerError), body.Title)
	assert.Equal(t, "internal error", body.Detail.Detail)
	assert.NotContains(t, body.Detail.Detail, "does not know")
}

func TestHighStatusScrub_DisabledRestoresServiceUnavailable(t *testing.T) {
	disableScrubForTest(t)

	body, ok := ProblemDetail(pkg.ServiceUnavailableError{
		Code:    "0999",
		Title:   "Dependency Unavailable",
		Message: "The upstream dependency is unavailable.",
	})
	require.True(t, ok)

	assert.Equal(t, http.StatusServiceUnavailable, body.Status)
	assert.Equal(t, "Dependency Unavailable", body.Title)
	assert.Equal(t, "The upstream dependency is unavailable.", body.Detail.Detail)
}

func TestHighStatusScrub_BelowFiveHundredIsUnaffected(t *testing.T) {
	notFound := pkg.EntityNotFoundError{
		Code:       "0007",
		Title:      "Entity Not Found",
		Message:    "No entity was found for the given ID.",
		EntityType: "Ledger",
	}

	before, ok := ProblemDetail(notFound)
	require.True(t, ok)

	disableScrubForTest(t)

	after, ok := ProblemDetail(notFound)
	require.True(t, ok)

	assert.Equal(t, before, after, "the toggle must not touch anything below 500")
	assert.Equal(t, http.StatusNotFound, after.Status)
	assert.Equal(t, "Entity Not Found", after.Title)
}

func TestHighStatusScrub_WithProblemStatusFollowsTheToggle(t *testing.T) {
	internal := pkg.InternalServerError{
		Code:    "0046",
		Title:   registryInternalTitle,
		Message: registryInternalMessage,
	}

	t.Run("enabled scrubs", func(t *testing.T) {
		body := driveWithProblemStatus(t, http.StatusInternalServerError, internal)

		assert.Equal(t, http.StatusText(http.StatusInternalServerError), body["title"])
		assert.Equal(t, "internal error", body["detail"])
	})

	t.Run("disabled restores", func(t *testing.T) {
		disableScrubForTest(t)

		body := driveWithProblemStatus(t, http.StatusInternalServerError, internal)

		assert.Equal(t, registryInternalTitle, body["title"])
		assert.Equal(t, registryInternalMessage, body["detail"])
	})
}

// driveWithProblemStatus renders err through withProblemStatus at an explicit
// status and returns the decoded body. withProblemStatus builds its Detail by
// hand rather than through ProblemDetail, so it needs its own harness to prove
// it follows the same toggle.
func driveWithProblemStatus(t *testing.T, status int, err error) map[string]any {
	t.Helper()

	app := fiber.New(fiber.Config{ErrorHandler: CanonicalFiberErrorHandler})
	app.Get("/probe", func(c fiber.Ctx) error { return withProblemStatus(c, status, err) })

	resp, testErr := app.Test(httptest.NewRequest(fiber.MethodGet, "/probe", nil))
	require.NoError(t, testErr)

	defer func() { _ = resp.Body.Close() }()

	raw, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body), "body must be JSON, got: %s", string(raw))

	return body
}
