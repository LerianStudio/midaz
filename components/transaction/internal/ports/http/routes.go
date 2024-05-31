package http

import (
	lib "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/ports"
	"github.com/LerianStudio/midaz/components/transaction/internal/service"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func NewRouter(th *ports.TransactionHandler) *fiber.App {
	f := fiber.New()

	_ = service.NewConfig()

	f.Use(cors.New())
	f.Use(lib.WithCorrelationID())

	// jwt := lib.NewJWTMiddleware(config.JWKAddress)

	// -- Routes --
	f.Post("transaction/v1/validate", th.ValidateTransaction)
	f.Post("transaction/v1/parser", th.ParserTransactionTemplate)

	// Health
	f.Get("/health", lib.Ping)

	// Doc
	lib.DocAPI("transaction", "Transaction API", f)

	return f
}
