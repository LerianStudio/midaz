package in

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	goldTransaction "github.com/LerianStudio/midaz/v3/pkg/gold/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/trace"
)

const (
	pendingTransactionLockTTLSeconds = 300
)

// TransactionHandler struct that handle transaction
type TransactionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateTransactionJSON method that create transaction using JSON
//
//	@Summary		Create a Transaction using JSON
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json [post]
func (handler *TransactionHandler) CreateTransactionJSON(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	c.SetUserContext(ctx)

	input := http.Payload[*transaction.CreateTransactionInput](c, p)
	parserDSL := input.FromDSL()
	logger.Infof("Request to create an transaction with details: %#v", parserDSL)

	transactionStatus := constant.CREATED
	if parserDSL.Pending {
		transactionStatus = constant.PENDING
	}

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", parserDSL)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	return handler.createTransaction(c, *parserDSL, transactionStatus)
}

// CreateTransactionAnnotation method that create transaction using JSON
//
//	@Summary		Create a Transaction Annotation using JSON
//	@Description	Create a Transaction Annotation with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string										true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string										false	"Request ID"
//	@Param			organization_id	path		string										true	"Organization ID"
//	@Param			ledger_id		path		string								        true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/annotation [post]
func (handler *TransactionHandler) CreateTransactionAnnotation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	c.SetUserContext(ctx)

	input := http.Payload[*transaction.CreateTransactionInput](c, p)
	parserDSL := input.FromDSL()
	logger.Infof("Create an transaction annotation without an affected balance: %#v", parserDSL)

	return handler.createTransaction(c, *parserDSL, constant.NOTED)
}

// CreateTransactionInflow method that creates a transaction without specifying a source
//
//	@Summary		Create a Transaction without passing from source
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionInflowSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/inflow [post]
func (handler *TransactionHandler) CreateTransactionInflow(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_inflow")
	defer span.End()

	c.SetUserContext(ctx)

	input := http.Payload[*transaction.CreateTransactionInflowInput](c, p)
	parserDSL := input.InflowFromDSL()
	logger.Infof("Request to create an transaction inflow with details: %#v", parserDSL)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", parserDSL)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	response := handler.createTransaction(c, *parserDSL, constant.CREATED)

	return response
}

// CreateTransactionOutflow method that creates a transaction without specifying a distribution
//
//	@Summary		Create a Transaction without passing to distribution
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionOutflowSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/outflow [post]
func (handler *TransactionHandler) CreateTransactionOutflow(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_outflow")
	defer span.End()

	c.SetUserContext(ctx)

	input := http.Payload[*transaction.CreateTransactionOutflowInput](c, p)
	parserDSL := input.OutflowFromDSL()
	logger.Infof("Request to create an transaction outflow with details: %#v", parserDSL)

	transactionStatus := constant.CREATED
	if parserDSL.Pending {
		transactionStatus = constant.PENDING
	}

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", parserDSL)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	return handler.createTransaction(c, *parserDSL, transactionStatus)
}

// CreateTransactionDSL method that create transaction using DSL
//
//	@Summary		Create a Transaction using DSL
//	@Description	Create a Transaction with the input DSL file
//	@Tags			Transactions
//	@Accept			mpfd
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction		formData	file	true	"Transaction DSL file"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid DSL file format or validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/dsl [post]
func (handler *TransactionHandler) CreateTransactionDSL(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_dsl")
	defer span.End()

	c.SetUserContext(ctx)

	if err := handler.validateDSLQueryParams(c, &span, logger); err != nil {
		return err
	}

	dsl, err := handler.getDSLFile(c, &span, logger)
	if err != nil {
		return err
	}

	parserDSL, err := handler.validateAndParseDSL(c, &span, dsl)
	if err != nil {
		return err
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.transaction.send", parserDSL.Send)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	return handler.createTransaction(c, parserDSL, constant.CREATED)
}

// validateDSLQueryParams validates query parameters for DSL requests.
func (handler *TransactionHandler) validateDSLQueryParams(c *fiber.Ctx, span *trace.Span, logger libLog.Logger) error {
	_, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)
		logger.Error("Failed to validate query parameters: ", err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	return nil
}

// getDSLFile retrieves and returns the DSL file from the request header.
func (handler *TransactionHandler) getDSLFile(c *fiber.Ctx, span *trace.Span, logger libLog.Logger) (string, error) {
	dsl, err := http.GetFileFromHeader(c)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get file from Header", err)
		logger.Error("Failed to get file from Header: ", err.Error())

		if err := http.WithError(c, err); err != nil {
			return "", fmt.Errorf("failed to send error response: %w", err)
		}

		return "", nil
	}

	return dsl, nil
}

// validateAndParseDSL validates and parses the DSL content.
func (handler *TransactionHandler) validateAndParseDSL(c *fiber.Ctx, span *trace.Span, dsl string) (libTransaction.Transaction, error) {
	errListener := goldTransaction.Validate(dsl)
	if errListener != nil && len(errListener.Errors) > 0 {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(transaction.Transaction{}).Name(), errListener.Errors)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate script in DSL", err)

		if err := http.WithError(c, err); err != nil {
			return libTransaction.Transaction{}, fmt.Errorf("failed to send error response: %w", err)
		}

		return libTransaction.Transaction{}, nil
	}

	parsed := goldTransaction.Parse(dsl)

	parserDSL, ok := parsed.(libTransaction.Transaction)
	if !ok {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(transaction.Transaction{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse script in DSL", err)

		if err := http.WithError(c, err); err != nil {
			return libTransaction.Transaction{}, fmt.Errorf("failed to send error response: %w", err)
		}

		return libTransaction.Transaction{}, nil
	}

	return parserDSL, nil
}

// CommitTransaction method that commit transaction created before
//
//	@Summary		Commit a Transaction
//	@Description	Commit a previously created transaction
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit [Post]
func (handler *TransactionHandler) CommitTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	transactionID := http.LocalUUID(c, "transaction_id")

	_, span := tracer.Start(ctx, "handler.commit_transaction")
	defer span.End()

	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	return handler.commitOrCancelTransaction(c, tran, constant.APPROVED)
}

// CancelTransaction method that cancel pre transaction created before
//
//	@Summary		Cancel a pre transaction
//	@Description	Cancel a previously created pre transaction
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/cancel [Post]
func (handler *TransactionHandler) CancelTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	transactionID := http.LocalUUID(c, "transaction_id")

	_, span := tracer.Start(ctx, "handler.cancel_transaction")
	defer span.End()

	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	return handler.commitOrCancelTransaction(c, tran, constant.CANCELED)
}

// RevertTransaction method that revert transaction created before
//
//	@Summary		Revert a Transaction
//	@Description	Revert a Transaction with Transaction ID only
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction_id	path		string								true	"Transaction ID"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		422				{object}	mmodel.Error	"Unprocessable Entity, validation errors"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert [post]
func (handler *TransactionHandler) RevertTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	transactionID := http.LocalUUID(c, "transaction_id")

	_, span := tracer.Start(ctx, "handler.revert_transaction")
	defer span.End()

	if err := handler.validateNoParentTransaction(ctx, c, &span, logger, organizationID, ledgerID, transactionID); err != nil {
		return err
	}

	tran, err := handler.fetchAndValidateTransactionForRevert(ctx, c, &span, logger, organizationID, ledgerID, transactionID)
	if err != nil {
		return err
	}

	if tran == nil {
		return nil
	}

	transactionReverted := tran.TransactionRevert()
	if transactionReverted.IsEmpty() {
		err = pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, "RevertTransaction")
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction can't be reverted", err)
		logger.Errorf("Parent Transaction can't be reverted with ID: %s, Error: %s", transactionID.String(), err)

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	return handler.createTransaction(c, transactionReverted, constant.CREATED)
}

// validateNoParentTransaction checks if the transaction already has a parent.
func (handler *TransactionHandler) validateNoParentTransaction(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID, transactionID uuid.UUID) error {
	parent, err := handler.Query.GetParentByTransactionID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Parent Transaction on query", err)
		logger.Errorf("Failed to retrieve Parent Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	if parent != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, "RevertTransaction")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)
		logger.Errorf("Transaction Has Already Parent Transaction with ID: %s, Error: %s", transactionID.String(), err)

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	return nil
}

// fetchAndValidateTransactionForRevert retrieves and validates a transaction before reverting it.
func (handler *TransactionHandler) fetchAndValidateTransactionForRevert(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	tran, err := handler.Query.GetTransactionWithOperationsByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve transaction on query", err)
		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		if err := http.WithError(c, err); err != nil {
			return nil, fmt.Errorf("failed to send error response: %w", err)
		}

		return nil, nil
	}

	if tran.ParentTransactionID != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDIsAlreadyARevert, "RevertTransaction")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)
		logger.Errorf("Transaction Has Already Parent Transaction with ID: %s, Error: %s", transactionID.String(), err)

		if err := http.WithError(c, err); err != nil {
			return nil, fmt.Errorf("failed to send error response: %w", err)
		}

		return nil, nil
	}

	if tran.Status.Code != constant.APPROVED {
		err = pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "RevertTransaction")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction CantRevert Transaction", err)
		logger.Errorf("Transaction CantRevert Transaction with ID: %s, Error: %s", transactionID.String(), err)

		if err := http.WithError(c, err); err != nil {
			return nil, fmt.Errorf("failed to send error response: %w", err)
		}

		return nil, nil
	}

	return tran, nil
}

// UpdateTransaction method that patch transaction created before
//
//	@Summary		Update a Transaction
//	@Description	Update a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction_id	path		string								true	"Transaction ID"
//	@Param			transaction		body		transaction.UpdateTransactionInput	true	"Transaction Input"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [patch]
func (handler *TransactionHandler) UpdateTransaction(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	transactionID := http.LocalUUID(c, "transaction_id")

	logger.Infof("Initiating update of Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String())

	payload := http.Payload[*transaction.UpdateTransactionInput](c, p)
	logger.Infof("Request to update an Transaction with details: %#v", payload)

	if err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	_, err := handler.Command.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction on command", err)

		logger.Errorf("Failed to update Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	trans, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	logger.Infof("Successfully updated Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String())

	if err := http.OK(c, trans); err != nil {
		return fmt.Errorf("failed to send transaction response: %w", err)
	}

	return nil
}

// GetTransaction method that get transaction created before
//
//	@Summary		Get a Transaction by ID
//	@Description	Get a Transaction with the input ID
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [get]
func (handler *TransactionHandler) GetTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_transaction")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	transactionID := http.LocalUUID(c, "transaction_id")

	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	ctxGetTransaction, spanGetTransaction := tracer.Start(ctx, "handler.get_transaction.get_operations")

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	headerParams.Metadata = &bson.M{}

	tran, err = handler.Query.GetOperationsByTransaction(ctxGetTransaction, organizationID, ledgerID, tran, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanGetTransaction, "Failed to retrieve Operations", err)

		logger.Errorf("Failed to retrieve Operations with ID: %s, Error: %s", tran.ID, err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	spanGetTransaction.End()

	logger.Infof("Successfully retrieved Transaction with ID: %s", transactionID.String())

	if err := http.OK(c, tran); err != nil {
		return fmt.Errorf("failed to send transaction response: %w", err)
	}

	return nil
}

// GetAllTransactions method that get all transactions created before
//
//	@Summary		Get all Transactions
//	@Description	Get all Transactions with the input metadata or without metadata
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example "2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example "2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	libPostgres.Pagination{items=[]transaction.Transaction,next_cursor=string,prev_cursor=string,limit=int,page=nil}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions [get]
func (handler *TransactionHandler) GetAllTransactions(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_transactions")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert metadata headerParams to JSON string", err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		return handler.getTransactionsWithMetadata(ctx, c, &span, logger, organizationID, ledgerID, headerParams, &pagination)
	}

	return handler.getTransactionsWithoutMetadata(ctx, c, &span, logger, organizationID, ledgerID, headerParams, &pagination)
}

// getTransactionsWithMetadata retrieves transactions with metadata filtering.
func (handler *TransactionHandler) getTransactionsWithMetadata(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, headerParams *http.QueryHeader, pagination *libPostgres.Pagination) error {
	logger.Infof("Initiating retrieval of all Transactions by metadata")

	trans, cur, err := handler.Query.GetAllMetadataTransactions(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Transactions by metadata", err)
		logger.Errorf("Failed to retrieve all Transactions, Error: %s", err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	logger.Infof("Successfully retrieved all Transactions by metadata")

	pagination.SetItems(trans)
	pagination.SetCursor(cur.Next, cur.Prev)

	if err := http.OK(c, pagination); err != nil {
		return fmt.Errorf("failed to send transactions pagination response: %w", err)
	}

	return nil
}

// getTransactionsWithoutMetadata retrieves transactions without metadata filtering.
func (handler *TransactionHandler) getTransactionsWithoutMetadata(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, headerParams *http.QueryHeader, pagination *libPostgres.Pagination) error {
	logger.Infof("Initiating retrieval of all Transactions ")

	headerParams.Metadata = &bson.M{}

	trans, cur, err := handler.Query.GetAllTransactions(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Transactions", err)
		logger.Errorf("Failed to retrieve all Transactions, Error: %s", err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	logger.Infof("Successfully retrieved all Transactions")

	pagination.SetItems(trans)
	pagination.SetCursor(cur.Next, cur.Prev)

	if err := http.OK(c, pagination); err != nil {
		return fmt.Errorf("failed to send transactions pagination response: %w", err)
	}

	return nil
}

// HandleAccountFields processes account and accountAlias fields for transaction entries
// accountAlias is deprecated but still needs to be handled for backward compatibility
func (handler *TransactionHandler) HandleAccountFields(entries []libTransaction.FromTo, isConcat bool) []libTransaction.FromTo {
	result := make([]libTransaction.FromTo, 0, len(entries))

	for i := range entries {
		var newAlias string
		if isConcat {
			newAlias = entries[i].ConcatAlias(i)
		} else {
			newAlias = entries[i].SplitAlias()
		}

		entries[i].AccountAlias = newAlias

		result = append(result, entries[i])
	}

	return result
}

func (handler *TransactionHandler) checkTransactionDate(logger libLog.Logger, parserDSL libTransaction.Transaction, transactionStatus string) (time.Time, error) {
	now := time.Now()
	transactionDate := now

	if !parserDSL.TransactionDate.IsZero() {
		switch {
		case parserDSL.TransactionDate.After(now):
			err := pkg.ValidateBusinessError(constant.ErrInvalidFutureTransactionDate, "validateTransactionDate")

			logger.Warnf("transaction date cannot be a future date: %v", err.Error())

			return time.Time{}, fmt.Errorf("invalid future transaction date: %w", err)
		case transactionStatus == constant.PENDING:
			err := pkg.ValidateBusinessError(constant.ErrInvalidPendingFutureTransactionDate, "validateTransactionDate")

			logger.Warnf("pending transaction cannot be used together a transaction date: %v", err.Error())

			return time.Time{}, fmt.Errorf("invalid pending transaction with future date: %w", err)
		default:
			transactionDate = parserDSL.TransactionDate
		}
	}

	return transactionDate, nil
}

// BuildOperations builds the operations for the transaction
func (handler *TransactionHandler) BuildOperations(
	ctx context.Context,
	balances []*mmodel.Balance,
	fromTo []libTransaction.FromTo,
	parserDSL libTransaction.Transaction,
	tran transaction.Transaction,
	validate *libTransaction.Responses,
	transactionDate time.Time,
	isAnnotation bool,
) ([]*operation.Operation, []*mmodel.Balance, error) {
	var operations []*operation.Operation

	var preBalances []*mmodel.Balance

	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction_operations")
	defer span.End()

	for _, blc := range balances {
		for i := range fromTo {
			if blc.Alias == fromTo[i].AccountAlias {
				logger.Infof("Creating operation for account id: %s and account alias: %s", blc.ID, blc.Alias)

				preBalances = append(preBalances, blc)

				amt, bat, err := libTransaction.ValidateFromToOperation(fromTo[i], *validate, blc.ConvertToLibBalance())
				if err != nil {
					libOpentelemetry.HandleSpanError(&span, "Failed to validate balances", err)

					logger.Errorf("Failed to validate balance: %v", err.Error())

					return nil, nil, fmt.Errorf("failed to validate from/to operation: %w", err)
				}

				amount := operation.Amount{
					Value: &amt.Value,
				}

				balance := operation.Balance{
					Available: &blc.Available,
					OnHold:    &blc.OnHold,
					Version:   &blc.Version,
				}

				balanceAfter := operation.Balance{
					Available: &bat.Available,
					OnHold:    &bat.OnHold,
					Version:   &bat.Version,
				}

				if isAnnotation {
					a := decimal.NewFromInt(0)
					balance.Available = &a
					balanceAfter.Available = &a

					o := decimal.NewFromInt(0)
					balance.OnHold = &o
					balanceAfter.OnHold = &o

					vBefore := int64(0)
					balance.Version = &vBefore
					vAfter := int64(0)
					balanceAfter.Version = &vAfter
				}

				description := fromTo[i].Description
				if libCommons.IsNilOrEmpty(&fromTo[i].Description) {
					description = parserDSL.Description
				}

				operations = append(operations, &operation.Operation{
					ID:              libCommons.GenerateUUIDv7().String(),
					TransactionID:   tran.ID,
					Description:     description,
					Type:            amt.Operation,
					AssetCode:       parserDSL.Send.Asset,
					ChartOfAccounts: fromTo[i].ChartOfAccounts,
					Amount:          amount,
					Balance:         balance,
					BalanceAfter:    balanceAfter,
					BalanceID:       blc.ID,
					AccountID:       blc.AccountID,
					AccountAlias:    libTransaction.SplitAlias(blc.Alias),
					BalanceKey:      blc.Key,
					OrganizationID:  blc.OrganizationID,
					LedgerID:        blc.LedgerID,
					CreatedAt:       transactionDate,
					UpdatedAt:       time.Now(),
					Route:           fromTo[i].Route,
					Metadata:        fromTo[i].Metadata,
					BalanceAffected: !isAnnotation,
				})

				metricFactory.RecordTransactionProcessed(ctx, tran.OrganizationID, tran.LedgerID)
			}
		}
	}

	return operations, preBalances, nil
}

// createTransaction func that received struct from DSL parsed and create Transaction
func (handler *TransactionHandler) createTransaction(c *fiber.Ctx, parserDSL libTransaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	parentID := http.LocalUUIDOptional(c, "transaction_id")
	transactionID := libCommons.GenerateUUIDv7()

	c.Set(libConstants.IdempotencyReplayed, "false")

	transactionDate, err := handler.checkTransactionDate(logger, parserDSL, transactionStatus)
	if err != nil {
		return handler.handleTransactionDateError(c, &span, logger, err)
	}

	if err := handler.validateTransactionInput(&span, logger, parserDSL); err != nil {
		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	handler.ApplyDefaultBalanceKeys(parserDSL.Send.Source.From)
	handler.ApplyDefaultBalanceKeys(parserDSL.Send.Distribute.To)

	fromTo := handler.buildFromToList(parserDSL, transactionStatus)

	key, ttl := http.GetIdempotencyKeyAndTTL(c)
	ts, _ := libCommons.StructToJSONString(parserDSL)
	hash := libCommons.HashSHA256(ts)

	existingTran, err := handler.handleIdempotency(ctx, c, tracer, logger, organizationID, ledgerID, key, hash, ttl)
	if err != nil || existingTran != nil {
		return handler.handleIdempotencyResult(c, existingTran, err)
	}

	validate, balances, err := handler.validateAndGetBalances(ctx, tracer, &span, logger, organizationID, ledgerID, transactionID, parserDSL, transactionStatus, transactionDate, key)
	if err != nil {
		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	fromTo = handler.appendAccountFields(fromTo, parserDSL, transactionStatus)

	tran := handler.buildTransaction(transactionID, parentID, organizationID, ledgerID, parserDSL, transactionStatus, transactionDate)

	operations, _, err := handler.BuildOperations(ctx, balances, fromTo, parserDSL, *tran, validate, transactionDate, transactionStatus == constant.NOTED)
	if err != nil {
		return handler.handleBuildOperationsError(c, &span, logger, err)
	}

	return handler.executeAndRespondTransaction(ctx, c, &span, logger, organizationID, ledgerID, key, hash, ttl, tran, operations, validate, balances, parserDSL)
}

// validateTransactionInput validates the transaction input data.
func (handler *TransactionHandler) validateTransactionInput(span *trace.Span, logger libLog.Logger, parserDSL libTransaction.Transaction) error {
	err := libOpentelemetry.SetSpanAttributesFromStruct(span, "app.request.payload", parserDSL)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert transaction to JSON string", err)
		logger.Errorf("Failed to convert transaction to JSON string, Error: %s", err.Error())
	}

	if parserDSL.Send.Value.LessThanOrEqual(decimal.Zero) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, reflect.TypeOf(transaction.Transaction{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction with non-positive value", err)
		logger.Warnf("Transaction value must be greater than zero")

		return fmt.Errorf("invalid transaction with non-positive value: %w", err)
	}

	return nil
}

// buildFromToList builds the initial fromTo list based on transaction status.
func (handler *TransactionHandler) buildFromToList(parserDSL libTransaction.Transaction, transactionStatus string) []libTransaction.FromTo {
	var fromTo []libTransaction.FromTo

	fromTo = append(fromTo, handler.HandleAccountFields(parserDSL.Send.Source.From, true)...)
	to := handler.HandleAccountFields(parserDSL.Send.Distribute.To, true)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	return fromTo
}

// handleIdempotency handles idempotency checking and returns existing transaction if found.
func (handler *TransactionHandler) handleIdempotency(ctx context.Context, c *fiber.Ctx, tracer trace.Tracer, logger libLog.Logger, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*transaction.Transaction, error) {
	ctxIdempotency, spanIdempotency := tracer.Start(ctx, "handler.create_transaction_idempotency")
	defer spanIdempotency.End()

	value, err := handler.Command.CreateOrCheckIdempotencyKey(ctxIdempotency, organizationID, ledgerID, key, hash, ttl)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanIdempotency, "Error on create or check redis idempotency key", err)
		logger.Infof("Error on create or check redis idempotency key: %v", err.Error())

		return nil, fmt.Errorf("failed to create or check idempotency key: %w", err)
	}

	if !libCommons.IsNilOrEmpty(value) {
		t := transaction.Transaction{}
		if err = json.Unmarshal([]byte(*value), &t); err != nil {
			libOpentelemetry.HandleSpanError(&spanIdempotency, "Error to deserialization idempotency transaction json on redis", err)
			logger.Errorf("Error to deserialization idempotency transaction json on redis: %v", err)

			return nil, fmt.Errorf("failed to unmarshal idempotency transaction: %w", err)
		}

		c.Set(libConstants.IdempotencyReplayed, "true")

		return &t, nil
	}

	return nil, nil
}

// validateAndGetBalances validates transaction and retrieves balances.
func (handler *TransactionHandler) validateAndGetBalances(ctx context.Context, tracer trace.Tracer, span *trace.Span, logger libLog.Logger, organizationID, ledgerID, transactionID uuid.UUID, parserDSL libTransaction.Transaction, transactionStatus string, transactionDate time.Time, key string) (*libTransaction.Responses, []*mmodel.Balance, error) {
	validate, err := libTransaction.ValidateSendSourceAndDistribute(ctx, parserDSL, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Warnf("Failed to validate send source and distribute: %v", err.Error())
		err = pkg.HandleKnownBusinessValidationErrors(err)
		_ = handler.Command.RedisRepo.Del(ctx, key)

		return nil, nil, fmt.Errorf("failed to validate send source and distribute: %w", err)
	}

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")
	defer spanGetBalances.End()

	handler.Command.SendTransactionToRedisQueue(ctx, organizationID, ledgerID, transactionID, parserDSL, validate, transactionStatus, transactionDate)

	balances, err := handler.Query.GetBalances(ctx, organizationID, ledgerID, transactionID, &parserDSL, validate, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanGetBalances, "Failed to get balances", err)
		logger.Errorf("Failed to get balances: %v", err.Error())

		_ = handler.Command.RedisRepo.Del(ctx, key)
		handler.Command.RemoveTransactionFromRedisQueue(ctx, logger, organizationID, ledgerID, transactionID.String())

		return nil, nil, fmt.Errorf("failed to get balances: %w", err)
	}

	return validate, balances, nil
}

// handleTransactionDateError handles errors related to transaction date validation.
func (handler *TransactionHandler) handleTransactionDateError(c *fiber.Ctx, span *trace.Span, logger libLog.Logger, err error) error {
	libOpentelemetry.HandleSpanError(span, "Failed to check transaction date", err)
	logger.Errorf("Failed to check transaction date: %v", err)

	if err := http.WithError(c, err); err != nil {
		return fmt.Errorf("failed to send error response: %w", err)
	}

	return nil
}

// handleIdempotencyResult handles the result of idempotency checks.
func (handler *TransactionHandler) handleIdempotencyResult(c *fiber.Ctx, existingTran *transaction.Transaction, err error) error {
	if err != nil {
		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	if existingTran != nil {
		if err := http.Created(c, *existingTran); err != nil {
			return fmt.Errorf("failed to send created transaction response: %w", err)
		}

		return nil
	}

	return nil
}

// appendAccountFields appends account fields to the fromTo list based on transaction status.
func (handler *TransactionHandler) appendAccountFields(fromTo []libTransaction.FromTo, parserDSL libTransaction.Transaction, transactionStatus string) []libTransaction.FromTo {
	fromTo = append(fromTo, handler.HandleAccountFields(parserDSL.Send.Source.From, false)...)

	to := handler.HandleAccountFields(parserDSL.Send.Distribute.To, false)
	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	return fromTo
}

// handleBuildOperationsError handles errors during operation building.
func (handler *TransactionHandler) handleBuildOperationsError(c *fiber.Ctx, span *trace.Span, logger libLog.Logger, err error) error {
	libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)
	logger.Errorf("Failed to validate balance: %v", err.Error())

	if err := http.WithError(c, err); err != nil {
		return fmt.Errorf("failed to send error response: %w", err)
	}

	return nil
}

// executeAndRespondTransaction executes the transaction and sends the response.
func (handler *TransactionHandler) executeAndRespondTransaction(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration, tran *transaction.Transaction, operations []*operation.Operation, validate *libTransaction.Responses, balances []*mmodel.Balance, parserDSL libTransaction.Transaction) error {
	tran.Source = getAliasWithoutKey(validate.Sources)
	tran.Destination = getAliasWithoutKey(validate.Destinations)
	tran.Operations = operations

	if err := handler.Command.TransactionExecute(ctx, organizationID, ledgerID, &parserDSL, validate, balances, tran); err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "failed to update BTO", err)
		logger.Errorf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err)

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	// Idempotency key update MUST be synchronous to ensure subsequent requests
	// can detect replays immediately. Previously async (via goroutine), which created
	// a race condition where Request 2 could arrive before the cached response was stored.
	// This is critical for financial transaction safety.
	handler.Command.SetValueOnExistingIdempotencyKey(ctx, organizationID, ledgerID, key, hash, *tran, ttl)
	mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "transaction_audit_log", mruntime.KeepRunning, func(ctx context.Context) {
		handler.Command.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, tran.IDtoUUID())
	})

	if err := http.Created(c, tran); err != nil {
		return fmt.Errorf("failed to send created transaction response: %w", err)
	}

	return nil
}

// buildTransaction creates a transaction object with the provided data.
func (handler *TransactionHandler) buildTransaction(transactionID, parentID, organizationID, ledgerID uuid.UUID, parserDSL libTransaction.Transaction, transactionStatus string, transactionDate time.Time) *transaction.Transaction {
	var parentTransactionID *string

	if parentID != uuid.Nil {
		str := parentID.String()
		parentTransactionID = &str
	}

	return &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      parentTransactionID,
		OrganizationID:           organizationID.String(),
		LedgerID:                 ledgerID.String(),
		Description:              parserDSL.Description,
		Amount:                   &parserDSL.Send.Value,
		AssetCode:                parserDSL.Send.Asset,
		ChartOfAccountsGroupName: parserDSL.ChartOfAccountsGroupName,
		CreatedAt:                transactionDate,
		UpdatedAt:                time.Now(),
		Route:                    parserDSL.Route,
		Metadata:                 parserDSL.Metadata,
		Status: transaction.Status{
			Code:        transactionStatus,
			Description: &transactionStatus,
		},
	}
}

// commitOrCancelTransaction func that is responsible to commit or cancel transaction.
func (handler *TransactionHandler) commitOrCancelTransaction(c *fiber.Ctx, tran *transaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.commit_or_cancel_transaction")
	defer span.End()

	assert.That(assert.ValidUUID(tran.OrganizationID),
		"transaction organization ID must be valid UUID",
		"organizationID", tran.OrganizationID)
	assert.That(assert.ValidUUID(tran.LedgerID),
		"transaction ledger ID must be valid UUID",
		"ledgerID", tran.LedgerID)

	organizationID := uuid.MustParse(tran.OrganizationID)
	ledgerID := uuid.MustParse(tran.LedgerID)

	deleteLockOnError, err := handler.acquireTransactionLock(ctx, &span, logger, organizationID, ledgerID, tran.ID)
	if err != nil {
		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	if err := handler.validatePendingTransaction(&span, logger, tran, deleteLockOnError); err != nil {
		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	parserDSL := tran.Body
	handler.ApplyDefaultBalanceKeys(parserDSL.Send.Source.From)
	handler.ApplyDefaultBalanceKeys(parserDSL.Send.Distribute.To)

	fromTo := handler.buildCommitFromToList(parserDSL, transactionStatus)

	validate, balances, err := handler.validateAndGetBalancesForCommit(ctx, tracer, &span, logger, organizationID, ledgerID, tran, parserDSL, transactionStatus, deleteLockOnError)
	if err != nil {
		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	return handler.executeCommitOrCancel(ctx, c, &span, logger, organizationID, ledgerID, tran, parserDSL, validate, balances, fromTo, transactionStatus)
}

// executeCommitOrCancel builds operations and executes the commit or cancel transaction.
func (handler *TransactionHandler) executeCommitOrCancel(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, tran *transaction.Transaction, parserDSL libTransaction.Transaction, validate *libTransaction.Responses, balances []*mmodel.Balance, fromTo []libTransaction.FromTo, transactionStatus string) error {
	fromTo = append(fromTo, handler.HandleAccountFields(parserDSL.Send.Source.From, false)...)

	to := handler.HandleAccountFields(parserDSL.Send.Distribute.To, false)
	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	tran.UpdatedAt = time.Now()
	tran.Status = transaction.Status{
		Code:        transactionStatus,
		Description: &transactionStatus,
	}

	operations, preBalances, err := handler.BuildOperations(ctx, balances, fromTo, parserDSL, *tran, validate, time.Now(), false)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)
		logger.Errorf("Failed to validate balance: %v", err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	tran.Source = getAliasWithoutKey(validate.Sources)
	tran.Destination = getAliasWithoutKey(validate.Destinations)
	tran.Operations = operations

	handler.updateTransactionStatusIfAsync(ctx, span, logger, tran)

	if err := handler.Command.TransactionExecute(ctx, organizationID, ledgerID, &parserDSL, validate, preBalances, tran); err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "failed to update BTO", err)
		logger.Errorf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err)

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "commit_cancel_audit_log", mruntime.KeepRunning, func(ctx context.Context) {
		handler.Command.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, tran.IDtoUUID())
	})

	if err := http.Created(c, tran); err != nil {
		return fmt.Errorf("failed to send created transaction response: %w", err)
	}

	return nil
}

// acquireTransactionLock acquires a lock for the transaction and returns a cleanup function.
func (handler *TransactionHandler) acquireTransactionLock(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, tranID string) (func(), error) {
	lockPendingTransactionKey := utils.GenericInternalKeyWithContext("pending_transaction", "transaction", organizationID.String(), ledgerID.String(), tranID)
	ttl := time.Duration(pendingTransactionLockTTLSeconds)

	success, err := handler.Command.RedisRepo.SetNX(ctx, lockPendingTransactionKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set on redis", err)
		logger.Errorf("Failed to set on redis: %v", err)

		return nil, fmt.Errorf("failed to set transaction lock in redis: %w", err)
	}

	if !success {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is locked", err)
		logger.Errorf("Failed, Transaction: %s is locked, Error: %s", tranID, err.Error())

		return nil, fmt.Errorf("transaction is locked: %w", err)
	}

	deleteLockOnError := func() {
		if delErr := handler.Command.RedisRepo.Del(ctx, lockPendingTransactionKey); delErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete pending transaction lock", delErr)
			logger.Errorf("Failed to delete pending transaction lock key: %v", delErr)
		}
	}

	return deleteLockOnError, nil
}

// validatePendingTransaction validates that the transaction is in pending state.
func (handler *TransactionHandler) validatePendingTransaction(span *trace.Span, logger libLog.Logger, tran *transaction.Transaction, deleteLockOnError func()) error {
	if tran.Status.Code != constant.PENDING {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is not pending", err)
		logger.Errorf("Failed, Transaction: %s is not pending, Error: %s", tran.ID, err.Error())
		deleteLockOnError()

		return fmt.Errorf("transaction is not pending: %w", err)
	}

	return nil
}

// buildCommitFromToList builds the initial fromTo list for commit/cancel operations.
func (handler *TransactionHandler) buildCommitFromToList(parserDSL libTransaction.Transaction, transactionStatus string) []libTransaction.FromTo {
	var fromTo []libTransaction.FromTo

	fromTo = append(fromTo, handler.HandleAccountFields(parserDSL.Send.Source.From, true)...)
	to := handler.HandleAccountFields(parserDSL.Send.Distribute.To, true)

	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	return fromTo
}

// validateAndGetBalancesForCommit validates transaction and retrieves balances for commit/cancel.
func (handler *TransactionHandler) validateAndGetBalancesForCommit(ctx context.Context, tracer trace.Tracer, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, tran *transaction.Transaction, parserDSL libTransaction.Transaction, transactionStatus string, deleteLockOnError func()) (*libTransaction.Responses, []*mmodel.Balance, error) {
	validate, err := libTransaction.ValidateSendSourceAndDistribute(ctx, parserDSL, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Error("Failed to validate send source and distribute: %v", err.Error())
		err = pkg.HandleKnownBusinessValidationErrors(err)

		deleteLockOnError()

		return nil, nil, fmt.Errorf("failed to validate send source and distribute: %w", err)
	}

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")
	defer spanGetBalances.End()

	balances, err := handler.Query.GetBalances(ctx, organizationID, ledgerID, tran.IDtoUUID(), nil, validate, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanGetBalances, "Failed to get balances", err)
		logger.Errorf("Failed to get balances: %v", err.Error())
		deleteLockOnError()

		return nil, nil, fmt.Errorf("failed to get balances: %w", err)
	}

	return validate, balances, nil
}

// updateTransactionStatusIfAsync updates transaction status synchronously if async mode is enabled.
func (handler *TransactionHandler) updateTransactionStatusIfAsync(ctx context.Context, span *trace.Span, logger libLog.Logger, tran *transaction.Transaction) {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		if _, err := handler.Command.UpdateTransactionStatus(ctx, tran); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction status synchronously", err)
			logger.Errorf("Failed to update transaction status synchronously for Transaction: %s, Error: %s", tran.ID, err.Error())
		}
	}
}

// ApplyDefaultBalanceKeys sets the BalanceKey to "default" for any entries where the BalanceKey is empty.
func (handler *TransactionHandler) ApplyDefaultBalanceKeys(entries []libTransaction.FromTo) {
	for i := range entries {
		if entries[i].BalanceKey == "" {
			entries[i].BalanceKey = constant.DefaultBalanceKey
		}
	}
}

// getAliasWithoutKey takes a slice of strings and returns a new slice containing the first part of each string split by '#'.
func getAliasWithoutKey(array []string) []string {
	result := make([]string, len(array))

	for i, str := range array {
		parts := strings.Split(str, "#")
		result[i] = parts[0]
	}

	return result
}
