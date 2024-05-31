package http

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/common/gold/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	"github.com/gofiber/fiber/v2"
)

type TransactionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// InputDSL is a struct design to encapsulate payload data.
type InputDSL struct {
	TransactionType     uuid.UUID      `json:"transactionType"`
	TransactionTypeCode string         `json:"transactionTypeCode"`
	Variables           map[string]any `json:"variables,omitempty"`
}

func (handler *TransactionHandler) ValidateTransaction(ctx *fiber.Ctx) error {
	fileHeader, err := ctx.FormFile("dsl")
	if err != nil {
		return err
	}

	if !strings.Contains(fileHeader.Filename, ".gold") {
		return ctx.Status(http.StatusBadRequest).JSON(fiber.Map{
			"code":    -1,
			"message": fmt.Sprintf("This type o file: %s can't be parsed", fileHeader.Filename),
		})
	}

	if fileHeader.Size == 0 {
		return ctx.Status(http.StatusBadRequest).JSON(fiber.Map{
			"code":    -1,
			"message": fmt.Sprintf("This file: %s is empty", fileHeader.Filename),
		})
	}

	file, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer func(file multipart.File) {
		err := file.Close()
		if err != nil {
			panic(0)
		}
	}(file)

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return err
	}

	dsl := buf.String()

	errListener := transaction.Validate(dsl)
	if errListener != nil && len(errListener.Errors) > 0 {
		var err []fiber.Map
		for _, e := range errListener.Errors {
			err = append(err, fiber.Map{
				"line":    e.Line,
				"column":  e.Column,
				"message": e.Message,
				"source":  errListener.Source,
			})
		}

		return ctx.Status(http.StatusBadRequest).JSON(err)
	}

	t := transaction.Parse(dsl)

	return ctx.Status(http.StatusOK).JSON(fiber.Map{
		"transaction": t,
	})
}

func (handler *TransactionHandler) ParserTransactionTemplate(ctx *fiber.Ctx) error {
	fileHeader, err := ctx.FormFile("dsl")
	if err != nil {
		return err
	}

	if !strings.Contains(fileHeader.Filename, ".gold") {
		return ctx.Status(http.StatusBadRequest).JSON(fiber.Map{
			"code":    -1,
			"message": fmt.Sprintf("This type o file: %s can't be parsed", fileHeader.Filename),
		})
	}

	if fileHeader.Size == 0 {
		return ctx.Status(http.StatusBadRequest).JSON(fiber.Map{
			"code":    -1,
			"message": fmt.Sprintf("This file: %s is empty", fileHeader.Filename),
		})
	}

	file, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer func(file multipart.File) {
		err := file.Close()
		if err != nil {
			panic(0)
		}
	}(file)

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return err
	}

	dsl := buf.String()

	if err := ctx.BodyParser(&InputDSL{}); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(fiber.Map{
			"code":    -1,
			"message": fmt.Sprintf("This input: %s can't be parsed", err.Error()),
		})
	}

	t := transaction.Parse(dsl)

	return ctx.Status(http.StatusOK).JSON(fiber.Map{
		"template": t,
	})
}
