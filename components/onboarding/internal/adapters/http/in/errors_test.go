package in

import (
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

func TestLegacyFiberErrorHandler_PreservesLegacyErrorEnvelope(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{ErrorHandler: legacyFiberErrorHandler})
	app.Get("/bad-request", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request")
	})
	app.Get("/boom", func(c *fiber.Ctx) error {
		return errors.New("boom")
	})

	t.Run("fiber error uses raw message", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest("GET", "/bad-request", nil))
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"error":"invalid request"}`, string(body))
	})

	t.Run("generic error uses internal server error text", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest("GET", "/boom", nil))
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"error":"Internal Server Error"}`, string(body))
	})
}

func TestLegacyErrorBoundary_PreservesLegacyErrorEnvelopeInGroupMiddleware(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	group := app.Group("", LegacyErrorBoundary())
	group.Get("/bad-request", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/bad-request", nil))
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"error":"invalid request"}`, string(body))
}
