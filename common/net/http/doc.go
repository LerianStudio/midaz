package http

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
)

// DocAPI adds the default documentation route to the API.
// Ex: /{serviceName}/docs
// And adds the swagger route too.
// Ex: /{serviceName}/swagger.yaml
func DocAPI(serviceName, title string, app *fiber.App) {
	docURL := fmt.Sprintf("/%s/docs", serviceName)

	app.Get(docURL, func(c *fiber.Ctx) error {
		return c.SendFile("./components/ledger/api/v1.yml")
	})

	app.Get("/v1/swagger/*", swagger.New(swagger.Config{
		URL:   docURL,
		Title: title,
	}))
}
