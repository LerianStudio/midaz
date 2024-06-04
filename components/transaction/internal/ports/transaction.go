package ports

import (
	"bytes"
	"fmt"
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/LerianStudio/midaz/common/gold/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	"github.com/gofiber/fiber/v2"
)

type TransactionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateTransaction method that create transaction
func (handler *TransactionHandler) CreateTransaction(c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	dsl, err := getFileFromHeader(c)
	if err != nil {
		logger.Error("Failed to validate and parse transaction", err.Error())
		return commonHTTP.WithError(c, err)
	}

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

		return c.Status(http.StatusBadRequest).JSON(err)
	}

	tran := transaction.Parse(dsl)

	logger.Infof("Transaction parsed and validated")

	return commonHTTP.Created(c, tran)
}

// CreateTransactionTemplate method that create transaction template
func (handler *TransactionHandler) CreateTransactionTemplate(p any, c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	payload := p.(*t.InputDSL)
	logger.Infof("Request to create an transaction with details: %#v", payload)

	return commonHTTP.Created(c, payload)
}

// CommitTransaction method that commit transaction created before
func (handler *TransactionHandler) CommitTransaction(c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	return commonHTTP.Created(c, logger)
}

// RevertTransaction method that revert transaction created before
func (handler *TransactionHandler) RevertTransaction(c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	return commonHTTP.Created(c, logger)
}

// getFileFromHeader method that get file from header and give a string fom this dsl gold file
func getFileFromHeader(ctx *fiber.Ctx) (string, error) {
	fileHeader, err := ctx.FormFile("dsl")
	if err != nil {
		return "", err
	}

	if !strings.Contains(fileHeader.Filename, ".gold") {
		return "", common.ValidationError{
			Code:    "0001",
			Message: fmt.Sprintf("This type o file: %s can't be parsed", fileHeader.Filename),
		}
	}

	if fileHeader.Size == 0 {
		return "", common.ValidationError{
			Code:    "0001",
			Message: fmt.Sprintf("This file: %s is empty", fileHeader.Filename),
		}
	}

	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer func(file multipart.File) {
		err := file.Close()
		if err != nil {
			panic(0)
		}
	}(file)

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return "", err
	}

	dsl := buf.String()

	return dsl, nil
}
