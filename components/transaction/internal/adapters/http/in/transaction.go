package in

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	goldTransaction "github.com/LerianStudio/midaz/v3/pkg/gold/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mlog"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/trace"
)

const (
	pendingTransactionLockTTLSeconds = 300
	// asyncModeValue is the value used to check if async mode is enabled via environment variable.
	asyncModeValue = "true"
	// revertWaitTimeoutSeconds is the timeout in seconds for waiting for operations to be ready for revert.
	revertWaitTimeoutSeconds = 5
	// revertPollIntervalMillis is the polling interval in milliseconds when waiting for operations.
	revertPollIntervalMillis = 75
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
//	@Success		201				{object}	mmodel.Transaction
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

	input := http.Payload[*mmodel.CreateTransactionInput](c, p)
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
//	@Success		201				{object}	mmodel.Transaction
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

	input := http.Payload[*mmodel.CreateTransactionInput](c, p)
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
//	@Success		201				{object}	mmodel.Transaction
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

	input := http.Payload[*mmodel.CreateTransactionInflowInput](c, p)
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
//	@Success		201				{object}	mmodel.Transaction
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

	input := http.Payload[*mmodel.CreateTransactionOutflowInput](c, p)
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
//	@Success		200				{object}	mmodel.Transaction
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
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	dsl, err := handler.getDSLFile(c, &span, logger)
	if err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	parserDSL, err := handler.validateAndParseDSL(&span, dsl)
	if err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
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

		//nolint:wrapcheck // Error is validated and written by CreateTransactionDSL.
		return err
	}

	return nil
}

// getDSLFile retrieves and returns the DSL file from the request header.
func (handler *TransactionHandler) getDSLFile(c *fiber.Ctx, span *trace.Span, logger libLog.Logger) (string, error) {
	dsl, err := http.GetFileFromHeader(c)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get file from Header", err)
		logger.Error("Failed to get file from Header: ", err.Error())

		//nolint:wrapcheck // Error is validated and written by CreateTransactionDSL.
		return "", err
	}

	return dsl, nil
}

// validateAndParseDSL validates and parses the DSL content.
func (handler *TransactionHandler) validateAndParseDSL(span *trace.Span, dsl string) (pkgTransaction.Transaction, error) {
	errListener := goldTransaction.Validate(dsl)
	if errListener != nil && len(errListener.Errors) > 0 {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(mmodel.Transaction{}).Name(), errListener.Errors)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate script in DSL", err)

		return pkgTransaction.Transaction{}, err
	}

	parsed := goldTransaction.Parse(dsl)

	// The Gold parser returns lib-commons Transaction types, but the handler
	// expects pkg Transaction types. Convert between the two type scopes.
	//nolint:staticcheck // SA1019: libTransaction.Transaction is required here as goldTransaction.Parse returns this type
	libTran, ok := parsed.(libTransaction.Transaction)
	if !ok {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(mmodel.Transaction{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse script in DSL: unexpected type", err)

		return pkgTransaction.Transaction{}, err
	}

	parserDSL := goldTransaction.ConvertLibToPkgTransaction(libTran)

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
//	@Success		201				{object}	mmodel.Transaction
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

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	assert.That(tran.OrganizationID == organizationID.String(),
		"transaction organization_id must match request",
		"transaction_id", tran.ID,
		"expected_organization_id", organizationID.String(),
		"actual_organization_id", tran.OrganizationID)
	assert.That(tran.LedgerID == ledgerID.String(),
		"transaction ledger_id must match request",
		"transaction_id", tran.ID,
		"expected_ledger_id", ledgerID.String(),
		"actual_ledger_id", tran.LedgerID)

	assert.That(!tran.UpdatedAt.Before(tran.CreatedAt),
		"transaction updated_at must be >= created_at",
		"transaction_id", tran.ID,
		"created_at", tran.CreatedAt,
		"updated_at", tran.UpdatedAt)

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
//	@Success		201				{object}	mmodel.Transaction
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

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	assert.That(tran.OrganizationID == organizationID.String(),
		"transaction organization_id must match request",
		"transaction_id", tran.ID,
		"expected_organization_id", organizationID.String(),
		"actual_organization_id", tran.OrganizationID)
	assert.That(tran.LedgerID == ledgerID.String(),
		"transaction ledger_id must match request",
		"transaction_id", tran.ID,
		"expected_ledger_id", ledgerID.String(),
		"actual_ledger_id", tran.LedgerID)

	assert.That(!tran.UpdatedAt.Before(tran.CreatedAt),
		"transaction updated_at must be >= created_at",
		"transaction_id", tran.ID,
		"created_at", tran.CreatedAt,
		"updated_at", tran.UpdatedAt)

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
//	@Success		200				{object}	mmodel.Transaction
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

	if err := handler.validateNoParentTransaction(ctx, &span, logger, organizationID, ledgerID, transactionID); err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	tran, err := handler.fetchAndValidateTransactionForRevert(ctx, &span, logger, organizationID, ledgerID, transactionID)
	if err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if tran == nil {
		return nil
	}

	assert.That(tran.OrganizationID == organizationID.String(),
		"transaction organization_id must match request",
		"transaction_id", tran.ID,
		"expected_organization_id", organizationID.String(),
		"actual_organization_id", tran.OrganizationID)
	assert.That(tran.LedgerID == ledgerID.String(),
		"transaction ledger_id must match request",
		"transaction_id", tran.ID,
		"expected_ledger_id", ledgerID.String(),
		"actual_ledger_id", tran.LedgerID)

	assert.That(!tran.UpdatedAt.Before(tran.CreatedAt),
		"transaction updated_at must be >= created_at",
		"transaction_id", tran.ID,
		"created_at", tran.CreatedAt,
		"updated_at", tran.UpdatedAt)

	transactionReverted := handler.buildRevertTransaction(logger, tran)
	if transactionReverted.IsEmpty() {
		err = pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, "RevertTransaction")
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction can't be reverted", err)
		logger.Errorf("Parent Transaction can't be reverted with ID: %s, Error: %s", transactionID.String(), err)

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	return handler.createTransaction(c, transactionReverted, constant.CREATED)
}

// validateNoParentTransaction checks if the transaction already has a parent.
func (handler *TransactionHandler) validateNoParentTransaction(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID, transactionID uuid.UUID) error {
	parent, err := handler.Query.GetParentByTransactionID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Parent Transaction on query", err)
		logger.Errorf("Failed to retrieve Parent Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		//nolint:wrapcheck // Error is already validated in the query layer and mapped to HTTP at the handler boundary.
		return err
	}

	if parent != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, "RevertTransaction")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)
		logger.Errorf("Transaction Has Already Parent Transaction with ID: %s, Error: %s", transactionID.String(), err)

		return err
	}

	return nil
}

// fetchAndValidateTransactionForRevert retrieves and validates a transaction before reverting it.
func (handler *TransactionHandler) fetchAndValidateTransactionForRevert(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID, transactionID uuid.UUID) (*mmodel.Transaction, error) {
	tran, err := handler.retrieveTransactionForRevert(ctx, span, logger, organizationID, ledgerID, transactionID)
	if err != nil || tran == nil {
		return tran, err
	}

	if err := handler.waitForOperationsIfNeeded(ctx, span, logger, organizationID, ledgerID, transactionID, tran); err != nil {
		return nil, err
	}

	if err := handler.validateTransactionCanBeReverted(span, logger, transactionID, tran); err != nil {
		return nil, err
	}

	for _, op := range tran.Operations {
		assert.NotNil(op, "operation must not be nil", "transaction_id", tran.ID)
		assert.That(op.TransactionID == tran.ID, "operation transaction_id must match transaction",
			"transaction_id", tran.ID, "operation_id", op.ID, "operation_transaction_id", op.TransactionID)
		assert.That(op.OrganizationID == tran.OrganizationID, "operation organization_id must match transaction",
			"transaction_id", tran.ID, "operation_id", op.ID, "operation_organization_id", op.OrganizationID)
		assert.That(op.LedgerID == tran.LedgerID, "operation ledger_id must match transaction",
			"transaction_id", tran.ID, "operation_id", op.ID, "operation_ledger_id", op.LedgerID)
		assert.That(op.AssetCode == tran.AssetCode, "operation asset_code must match transaction",
			"transaction_id", tran.ID, "operation_id", op.ID, "operation_asset_code", op.AssetCode)
	}

	return tran, nil
}

// retrieveTransactionForRevert fetches transaction data and merges operations.
func (handler *TransactionHandler) retrieveTransactionForRevert(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID, transactionID uuid.UUID) (*mmodel.Transaction, error) {
	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve base transaction on query", err)
		logger.Errorf("Failed to retrieve base Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())
		//nolint:wrapcheck // Error is already validated in the query layer and mapped to HTTP at the handler boundary.
		return nil, err
	}

	tranWithOps, err := handler.Query.GetTransactionWithOperationsByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve transaction operations on query", err)
		logger.Errorf("Failed to retrieve Transaction operations with ID: %s, Error: %s", transactionID.String(), err.Error())
		//nolint:wrapcheck // Error is already validated in the query layer and mapped to HTTP at the handler boundary.
		return nil, err
	}

	if tran == nil || tran.ID == "" {
		tran = tranWithOps
	} else if tranWithOps != nil && tranWithOps.ID != "" {
		tran.Operations = tranWithOps.Operations
	}

	return tran, nil
}

// waitForOperationsIfNeeded polls for operations readiness when async mode is enabled.
func (handler *TransactionHandler) waitForOperationsIfNeeded(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID, transactionID uuid.UUID, tran *mmodel.Transaction) error {
	needsOperations := tran != nil && tran.Body.IsEmpty()
	if !handler.isAsyncModeEnabled() || !needsOperations || operationsReadyForRevert(tran) {
		return nil
	}

	waitDeadline := handler.calculateWaitDeadline(ctx)

	for time.Now().Before(waitDeadline) {
		time.Sleep(revertPollIntervalMillis * time.Millisecond)

		refreshed, refreshErr := handler.Query.GetTransactionWithOperationsByID(ctx, organizationID, ledgerID, transactionID)
		if refreshErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to refresh transaction operations", refreshErr)
			logger.Errorf("Failed to refresh transaction operations for ID %s: %s", transactionID.String(), refreshErr.Error())

			return pkg.ValidateInternalError(refreshErr, "RefreshTransactionOperations")
		}

		if operationsReadyForRevert(refreshed) {
			if tran != nil {
				tran.Operations = refreshed.Operations
			}

			break
		}
	}

	return nil
}

// calculateWaitDeadline computes the deadline for waiting, respecting context deadline.
func (handler *TransactionHandler) calculateWaitDeadline(ctx context.Context) time.Time {
	waitDeadline := time.Now().Add(revertWaitTimeoutSeconds * time.Second)
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining < revertWaitTimeoutSeconds*time.Second {
			waitDeadline = time.Now().Add(remaining)
		}
	}

	return waitDeadline
}

// isAsyncModeEnabled checks if async transaction processing is enabled.
func (handler *TransactionHandler) isAsyncModeEnabled() bool {
	return strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == asyncModeValue
}

// validateTransactionCanBeReverted checks if the transaction is eligible for revert.
func (handler *TransactionHandler) validateTransactionCanBeReverted(span *trace.Span, logger libLog.Logger, transactionID uuid.UUID, tran *mmodel.Transaction) error {
	if tran.ParentTransactionID != nil {
		err := pkg.ValidateBusinessError(constant.ErrTransactionIDIsAlreadyARevert, "RevertTransaction")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction Has Already Parent Transaction", err)
		logger.Errorf("Transaction Has Already Parent Transaction with ID: %s, Error: %s", transactionID.String(), err)

		return err
	}

	assert.That(assert.DateNotInFuture(tran.CreatedAt),
		"transaction created_at must not be in the future for revert",
		"transaction_id", transactionID.String(),
		"created_at", tran.CreatedAt)

	if tran.Status.Code != constant.APPROVED {
		err := pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, "RevertTransaction")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction CantRevert Transaction", err)
		logger.Errorf("Transaction CantRevert Transaction with ID: %s, Error: %s", transactionID.String(), err)

		return err
	}

	return nil
}

func operationsReadyForRevert(tran *mmodel.Transaction) bool {
	if !transactionHasRequiredFields(tran) {
		return false
	}

	debitTotal, creditTotal, valid := computeOperationTotals(tran.Operations)
	if !valid {
		return false
	}

	return totalsMatchAmount(debitTotal, creditTotal, *tran.Amount)
}

// transactionHasRequiredFields checks if a transaction has the basic fields required for revert validation.
func transactionHasRequiredFields(tran *mmodel.Transaction) bool {
	return tran != nil && tran.Amount != nil && assert.TransactionHasOperations(tran.Operations)
}

// computeOperationTotals computes debit and credit totals from operations.
// Returns false if any operation is invalid.
func computeOperationTotals(operations []*mmodel.Operation) (debitTotal, creditTotal decimal.Decimal, valid bool) {
	debitTotal = decimal.Zero
	creditTotal = decimal.Zero

	for _, op := range operations {
		if op == nil || op.Amount.Value == nil {
			return decimal.Zero, decimal.Zero, false
		}

		switch op.Type {
		case constant.DEBIT:
			debitTotal = debitTotal.Add(*op.Amount.Value)
		case constant.CREDIT:
			creditTotal = creditTotal.Add(*op.Amount.Value)
		}
	}

	if len(operations) > 0 {
		assert.That(assert.NonZeroTotals(debitTotal, creditTotal),
			"double-entry violation: transaction totals must be non-zero",
			"debitTotal", debitTotal, "creditTotal", creditTotal)

		assert.That(assert.DebitsEqualCredits(debitTotal, creditTotal),
			"double-entry violation: debits must equal credits",
			"debitTotal", debitTotal, "creditTotal", creditTotal,
			"difference", debitTotal.Sub(creditTotal))
	}

	return debitTotal, creditTotal, true
}

// totalsMatchAmount checks if both debit and credit totals are non-zero and equal to the transaction amount.
func totalsMatchAmount(debitTotal, creditTotal, amount decimal.Decimal) bool {
	if debitTotal.IsZero() || creditTotal.IsZero() {
		return false
	}

	return debitTotal.Equal(amount) && creditTotal.Equal(amount)
}

// validateDoubleEntry enforces the double-entry accounting invariant.
// Panics if debits != credits or if totals are zero.
// This is a programming bug assertion - user input errors should be handled before this point.
func validateDoubleEntry(operations []*mmodel.Operation) {
	debitTotal := decimal.Zero
	creditTotal := decimal.Zero

	for _, op := range operations {
		if op == nil || op.Amount.Value == nil {
			continue
		}
		switch op.Type {
		case constant.DEBIT:
			debitTotal = debitTotal.Add(*op.Amount.Value)
		case constant.CREDIT:
			creditTotal = creditTotal.Add(*op.Amount.Value)
		}
	}

	assert.That(assert.NonZeroTotals(debitTotal, creditTotal),
		"double-entry violation: transaction totals must be non-zero",
		"debitTotal", debitTotal, "creditTotal", creditTotal)

	assert.That(assert.DebitsEqualCredits(debitTotal, creditTotal),
		"double-entry violation: debits must equal credits",
		"debitTotal", debitTotal, "creditTotal", creditTotal,
		"difference", debitTotal.Sub(creditTotal))
}

// validateTransactionStateTransition enforces the transaction state machine.
// Panics if the transition from current to target is not allowed.
// Valid transitions: PENDING -> APPROVED, PENDING -> CANCELED
func validateTransactionStateTransition(current, target string) {
	assert.That(assert.ValidTransactionStatus(current),
		"current transaction status must be valid",
		"current", current)

	assert.That(assert.ValidTransactionStatus(target),
		"target transaction status must be valid",
		"target", target)

	assert.That(assert.TransactionCanTransitionTo(current, target),
		"invalid transaction state transition",
		"current", current, "target", target)
}

func (handler *TransactionHandler) buildRevertTransaction(logger libLog.Logger, tran *mmodel.Transaction) pkgTransaction.Transaction {
	if operationsReadyForRevert(tran) {
		return tran.TransactionRevert()
	}

	if tran == nil || tran.Body.IsEmpty() {
		return pkgTransaction.Transaction{}
	}

	logger.Infof("Using transaction body to build revert for transaction %s due to incomplete operations", tran.ID)

	return reverseTransactionBody(tran.Body)
}

func reverseTransactionBody(body pkgTransaction.Transaction) pkgTransaction.Transaction {
	froms := make([]pkgTransaction.FromTo, 0, len(body.Send.Distribute.To))
	for _, entry := range body.Send.Distribute.To {
		reversed := entry
		reversed.IsFrom = true
		froms = append(froms, reversed)
	}

	tos := make([]pkgTransaction.FromTo, 0, len(body.Send.Source.From))
	for _, entry := range body.Send.Source.From {
		reversed := entry
		reversed.IsFrom = false
		tos = append(tos, reversed)
	}

	return pkgTransaction.Transaction{
		ChartOfAccountsGroupName: body.ChartOfAccountsGroupName,
		Description:              body.Description,
		Pending:                  false,
		Metadata:                 body.Metadata,
		Route:                    body.Route,
		Send: pkgTransaction.Send{
			Asset: body.Send.Asset,
			Value: body.Send.Value,
			Source: pkgTransaction.Source{
				Remaining: body.Send.Distribute.Remaining,
				From:      froms,
			},
			Distribute: pkgTransaction.Distribute{
				Remaining: body.Send.Source.Remaining,
				To:        tos,
			},
		},
	}
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
//	@Param			transaction		body		mmodel.UpdateTransactionInput	true	"Transaction Input"
//	@Success		200				{object}	mmodel.Transaction
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

	payload := http.Payload[*mmodel.UpdateTransactionInput](c, p)
	logger.Infof("Request to update an Transaction with details: %#v", payload)

	if err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	_, err := handler.Command.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction on command", err)

		logger.Errorf("Failed to update Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	trans, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	assert.That(trans.OrganizationID == organizationID.String(),
		"transaction organization_id must match request",
		"transaction_id", trans.ID,
		"expected_organization_id", organizationID.String(),
		"actual_organization_id", trans.OrganizationID)
	assert.That(trans.LedgerID == ledgerID.String(),
		"transaction ledger_id must match request",
		"transaction_id", trans.ID,
		"expected_ledger_id", ledgerID.String(),
		"actual_ledger_id", trans.LedgerID)

	assert.That(!trans.UpdatedAt.Before(trans.CreatedAt),
		"transaction updated_at must be >= created_at",
		"transaction_id", trans.ID,
		"created_at", trans.CreatedAt,
		"updated_at", trans.UpdatedAt)

	logger.Infof("Successfully updated Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String())

	ensureTransactionDefaults(trans)

	if err := http.OK(c, trans); err != nil {
		return err
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
//	@Success		200				{object}	mmodel.Transaction
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

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	assert.That(tran.OrganizationID == organizationID.String(),
		"transaction organization_id must match request",
		"transaction_id", tran.ID,
		"expected_organization_id", organizationID.String(),
		"actual_organization_id", tran.OrganizationID)
	assert.That(tran.LedgerID == ledgerID.String(),
		"transaction ledger_id must match request",
		"transaction_id", tran.ID,
		"expected_ledger_id", ledgerID.String(),
		"actual_ledger_id", tran.LedgerID)

	assert.That(!tran.UpdatedAt.Before(tran.CreatedAt),
		"transaction updated_at must be >= created_at",
		"transaction_id", tran.ID,
		"created_at", tran.CreatedAt,
		"updated_at", tran.UpdatedAt)

	ctxGetTransaction, spanGetTransaction := tracer.Start(ctx, "handler.get_transaction.get_operations")

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	headerParams.Metadata = &bson.M{}

	tran, err = handler.Query.GetOperationsByTransaction(ctxGetTransaction, organizationID, ledgerID, tran, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanGetTransaction, "Failed to retrieve Operations", err)

		logger.Errorf("Failed to retrieve Operations with ID: %s, Error: %s", tran.ID, err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	for _, op := range tran.Operations {
		assert.NotNil(op, "operation must not be nil", "transaction_id", tran.ID)
		assert.That(op.TransactionID == tran.ID, "operation transaction_id must match transaction",
			"transaction_id", tran.ID, "operation_id", op.ID, "operation_transaction_id", op.TransactionID)
		assert.That(op.OrganizationID == tran.OrganizationID, "operation organization_id must match transaction",
			"transaction_id", tran.ID, "operation_id", op.ID, "operation_organization_id", op.OrganizationID)
		assert.That(op.LedgerID == tran.LedgerID, "operation ledger_id must match transaction",
			"transaction_id", tran.ID, "operation_id", op.ID, "operation_ledger_id", op.LedgerID)
		assert.That(op.AssetCode == tran.AssetCode, "operation asset_code must match transaction",
			"transaction_id", tran.ID, "operation_id", op.ID, "operation_asset_code", op.AssetCode)
	}

	spanGetTransaction.End()

	logger.Infof("Successfully retrieved Transaction with ID: %s", transactionID.String())

	ensureTransactionDefaults(tran)

	if err := http.OK(c, tran); err != nil {
		return err
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
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Transaction,next_cursor=string,prev_cursor=string,limit=int,page=nil}
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

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
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

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	for _, tran := range trans {
		assert.NotNil(tran, "transaction must not be nil",
			"organization_id", organizationID.String(),
			"ledger_id", ledgerID.String())
		assert.That(tran.OrganizationID == organizationID.String(),
			"transaction organization_id must match request",
			"transaction_id", tran.ID,
			"expected_organization_id", organizationID.String(),
			"actual_organization_id", tran.OrganizationID)
		assert.That(tran.LedgerID == ledgerID.String(),
			"transaction ledger_id must match request",
			"transaction_id", tran.ID,
			"expected_ledger_id", ledgerID.String(),
			"actual_ledger_id", tran.LedgerID)
	}

	logger.Infof("Successfully retrieved all Transactions by metadata")

	pagination.SetItems(trans)
	pagination.SetCursor(cur.Next, cur.Prev)

	if err := http.OK(c, pagination); err != nil {
		return err
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

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	for _, tran := range trans {
		assert.NotNil(tran, "transaction must not be nil",
			"organization_id", organizationID.String(),
			"ledger_id", ledgerID.String())
		assert.That(tran.OrganizationID == organizationID.String(),
			"transaction organization_id must match request",
			"transaction_id", tran.ID,
			"expected_organization_id", organizationID.String(),
			"actual_organization_id", tran.OrganizationID)
		assert.That(tran.LedgerID == ledgerID.String(),
			"transaction ledger_id must match request",
			"transaction_id", tran.ID,
			"expected_ledger_id", ledgerID.String(),
			"actual_ledger_id", tran.LedgerID)
	}

	logger.Infof("Successfully retrieved all Transactions")

	pagination.SetItems(trans)
	pagination.SetCursor(cur.Next, cur.Prev)

	if err := http.OK(c, pagination); err != nil {
		return err
	}

	return nil
}

// HandleAccountFields processes account and accountAlias fields for transaction entries
// accountAlias is deprecated but still needs to be handled for backward compatibility
func (handler *TransactionHandler) HandleAccountFields(entries []pkgTransaction.FromTo, isConcat bool) []pkgTransaction.FromTo {
	result := make([]pkgTransaction.FromTo, 0, len(entries))

	for i := range entries {
		entry := entries[i] // Make a copy to avoid mutating the original

		var newAlias string
		if isConcat {
			newAlias = entry.ConcatAlias(i)
		} else {
			newAlias = entry.SplitAlias()
		}

		entry.AccountAlias = newAlias

		result = append(result, entry)
	}

	return result
}

func (handler *TransactionHandler) checkTransactionDate(logger libLog.Logger, parserDSL pkgTransaction.Transaction, transactionStatus string) (time.Time, error) {
	now := time.Now()
	transactionDate := now

	if parserDSL.TransactionDate != nil && !parserDSL.TransactionDate.IsZero() {
		switch {
		case parserDSL.TransactionDate.After(now):
			err := pkg.ValidateBusinessError(constant.ErrInvalidFutureTransactionDate, "validateTransactionDate")

			logger.Warnf("transaction date cannot be a future date: %v", err.Error())

			return time.Time{}, err
		case transactionStatus == constant.PENDING:
			err := pkg.ValidateBusinessError(constant.ErrInvalidPendingFutureTransactionDate, "validateTransactionDate")

			logger.Warnf("pending transaction cannot be used together a transaction date: %v", err.Error())

			return time.Time{}, err
		default:
			transactionDate = parserDSL.TransactionDate.Time()
		}
	}

	return transactionDate, nil
}

// BuildOperations builds the operations for the transaction
func (handler *TransactionHandler) BuildOperations(
	ctx context.Context,
	balances []*mmodel.Balance,
	fromTo []pkgTransaction.FromTo,
	parserDSL pkgTransaction.Transaction,
	tran mmodel.Transaction,
	validate *pkgTransaction.Responses,
	transactionDate time.Time,
	isAnnotation bool,
) ([]*mmodel.Operation, []*mmodel.Balance, error) {
	var operations []*mmodel.Operation

	var preBalances []*mmodel.Balance

	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction_operations")
	defer span.End()

	for _, blc := range balances {
		for i := range fromTo {
			if blc.Alias == fromTo[i].AccountAlias {
				logger.Infof("Creating operation for account id: %s and account alias: %s", blc.ID, blc.Alias)

				preBalances = append(preBalances, blc)

				amt, bat, err := pkgTransaction.ValidateFromToOperation(fromTo[i], *validate, blc.ToTransactionBalance())
				if err != nil {
					libOpentelemetry.HandleSpanError(&span, "Failed to validate balances", err)

					logger.Errorf("Failed to validate balance: %v", err.Error())

					//nolint:wrapcheck // Returning typed business error directly to preserve error type for errors.As() matching
					return nil, nil, err
				}

				amount := mmodel.OperationAmount{
					Value: &amt.Value,
				}

				balance := mmodel.OperationBalance{
					Available: &blc.Available,
					OnHold:    &blc.OnHold,
					Version:   &blc.Version,
				}

				balanceAfter := mmodel.OperationBalance{
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

				operations = append(operations, &mmodel.Operation{
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
					AccountAlias:    pkgTransaction.SplitAlias(blc.Alias),
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
func (handler *TransactionHandler) createTransaction(c *fiber.Ctx, parserDSL pkgTransaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	parentID := http.LocalUUIDOptional(c, "transaction_id")
	transactionID := libCommons.GenerateUUIDv7()

	// Enrich wide event with business context
	mlog.EnrichFromLocals(c)
	mlog.EnrichTransaction(c, organizationID, ledgerID, transactionStatus)
	mlog.SetHandler(c, "create_transaction")

	c.Set(libConstants.IdempotencyReplayed, "false")

	transactionDate, err := handler.checkTransactionDate(logger, parserDSL, transactionStatus)
	if err != nil {
		return handler.handleTransactionDateError(c, &span, logger, err)
	}

	if err := handler.validateTransactionInput(&span, logger, parserDSL); err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
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

	// Create a validation copy of parserDSL with concatenated aliases.
	// This must happen AFTER idempotency hash (to preserve client input format for deduplication)
	// but BEFORE validateAndGetBalances (which needs unique keys in From/To maps).
	// Both Source.From and Distribute.To need concatenated aliases because ValidateSendSourceAndDistribute
	// always builds both From and To maps, and ValidateBalancesRules checks the total count.
	// IMPORTANT: We keep the original parserDSL unchanged for storage in transaction Body (used by revert).
	validationDSL := parserDSL
	validationDSL.Send.Source.From = handler.HandleAccountFields(parserDSL.Send.Source.From, true)
	validationDSL.Send.Distribute.To = handler.HandleAccountFields(parserDSL.Send.Distribute.To, true)

	validate, balances, err := handler.validateAndGetBalances(ctx, tracer, &span, logger, organizationID, ledgerID, transactionID, validationDSL, transactionStatus, transactionDate, key)
	if err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	fromTo = handler.appendAccountFields(fromTo, parserDSL, transactionStatus)

	tran := handler.buildTransaction(transactionID, parentID, organizationID, ledgerID, parserDSL, transactionStatus, transactionDate)
	assert.That(!transactionDate.After(tran.CreatedAt),
		"transaction_date must be <= created_at",
		"transaction_id", tran.ID,
		"transaction_date", transactionDate,
		"created_at", tran.CreatedAt)

	operations, preBalances, err := handler.BuildOperations(ctx, balances, fromTo, parserDSL, *tran, validate, transactionDate, transactionStatus == constant.NOTED)
	if err != nil {
		return handler.handleBuildOperationsError(c, &span, logger, err)
	}

	// Use preBalances (only balances with matching operations) to avoid updating
	// balances that don't have corresponding operations (e.g., destination accounts
	// during PENDING transactions shouldn't have their version incremented)
	return handler.executeAndRespondTransaction(ctx, c, &span, logger, organizationID, ledgerID, key, hash, ttl, tran, operations, validate, preBalances, parserDSL)
}

// validateTransactionInput validates the transaction input data.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func (handler *TransactionHandler) validateTransactionInput(span *trace.Span, logger libLog.Logger, parserDSL pkgTransaction.Transaction) error {
	err := libOpentelemetry.SetSpanAttributesFromStruct(span, "app.request.payload", parserDSL)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert transaction to JSON string", err)
		logger.Errorf("Failed to convert transaction to JSON string, Error: %s", err.Error())
	}

	if parserDSL.Send.Value.LessThanOrEqual(decimal.Zero) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, reflect.TypeOf(mmodel.Transaction{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid transaction with non-positive value", err)
		logger.Warnf("Transaction value must be greater than zero - received value: %s, asset: %s, description: %s",
			parserDSL.Send.Value.String(), parserDSL.Send.Asset, parserDSL.Description)

		return err
	}

	return nil
}

// buildFromToList builds the initial fromTo list based on transaction status.
func (handler *TransactionHandler) buildFromToList(parserDSL pkgTransaction.Transaction, transactionStatus string) []pkgTransaction.FromTo {
	var fromTo []pkgTransaction.FromTo

	fromTo = append(fromTo, handler.HandleAccountFields(parserDSL.Send.Source.From, true)...)
	to := handler.HandleAccountFields(parserDSL.Send.Distribute.To, true)

	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, to...)
	}

	return fromTo
}

// handleIdempotency handles idempotency checking and returns existing transaction if found.
func (handler *TransactionHandler) handleIdempotency(ctx context.Context, c *fiber.Ctx, tracer trace.Tracer, logger libLog.Logger, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*mmodel.Transaction, error) {
	ctxIdempotency, spanIdempotency := tracer.Start(ctx, "handler.create_transaction_idempotency")
	defer spanIdempotency.End()

	value, err := handler.Command.CreateOrCheckIdempotencyKey(ctxIdempotency, organizationID, ledgerID, key, hash, ttl)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanIdempotency, "Error on create or check redis idempotency key", err)
		logger.Infof("Error on create or check redis idempotency key: %v", err.Error())

		return nil, pkg.ValidateInternalError(err, "Idempotency")
	}

	if !libCommons.IsNilOrEmpty(value) {
		t := mmodel.Transaction{}
		if err = json.Unmarshal([]byte(*value), &t); err != nil {
			libOpentelemetry.HandleSpanError(&spanIdempotency, "Error to deserialization idempotency transaction json on redis", err)
			logger.Errorf("Error to deserialization idempotency transaction json on redis: %v", err)

			return nil, pkg.ValidateInternalError(err, "Transaction")
		}

		c.Set(libConstants.IdempotencyReplayed, "true")

		// Enrich wide event with idempotency info (cache hit)
		mlog.SetIdempotency(c, key, true)

		return &t, nil
	}

	return nil, nil
}

// validateAndGetBalances validates transaction and retrieves balances.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func (handler *TransactionHandler) validateAndGetBalances(ctx context.Context, tracer trace.Tracer, span *trace.Span, logger libLog.Logger, organizationID, ledgerID, transactionID uuid.UUID, parserDSL pkgTransaction.Transaction, transactionStatus string, transactionDate time.Time, key string) (*pkgTransaction.Responses, []*mmodel.Balance, error) {
	validate, err := pkgTransaction.ValidateSendSourceAndDistribute(ctx, parserDSL, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Warnf("Failed to validate send source and distribute: %v", err.Error())
		// HandleKnownBusinessValidationErrors returns typed business errors
		// Return directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
		businessErr := pkg.HandleKnownBusinessValidationErrors(err)
		_ = handler.Command.RedisRepo.Del(ctx, key)

		//nolint:wrapcheck // Returning typed business error directly to preserve error type for errors.As() matching
		return nil, nil, businessErr
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

		// Return error directly - may be typed business error that errors.As() needs to match
		//nolint:wrapcheck // Returning error directly to preserve error type for errors.As() matching
		return nil, nil, err
	}

	return validate, balances, nil
}

// handleTransactionDateError handles errors related to transaction date validation.
func (handler *TransactionHandler) handleTransactionDateError(c *fiber.Ctx, span *trace.Span, logger libLog.Logger, err error) error {
	libOpentelemetry.HandleSpanError(span, "Failed to check transaction date", err)
	logger.Errorf("Failed to check transaction date: %v", err)

	if httpErr := http.WithError(c, err); httpErr != nil {
		return httpErr
	}

	return nil
}

// handleIdempotencyResult handles the result of idempotency checks.
func (handler *TransactionHandler) handleIdempotencyResult(c *fiber.Ctx, existingTran *mmodel.Transaction, err error) error {
	if err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if existingTran != nil {
		ensureTransactionDefaults(existingTran)

		if err := http.Created(c, *existingTran); err != nil {
			return err
		}

		return nil
	}

	return nil
}

// appendAccountFields appends account fields to the fromTo list based on transaction status.
func (handler *TransactionHandler) appendAccountFields(fromTo []pkgTransaction.FromTo, parserDSL pkgTransaction.Transaction, transactionStatus string) []pkgTransaction.FromTo {
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

	if httpErr := http.WithError(c, err); httpErr != nil {
		return httpErr
	}

	return nil
}

// executeAndRespondTransaction executes the transaction and sends the response.
func (handler *TransactionHandler) executeAndRespondTransaction(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration, tran *mmodel.Transaction, operations []*mmodel.Operation, validate *pkgTransaction.Responses, balances []*mmodel.Balance, parserDSL pkgTransaction.Transaction) error {
	tran.Source = getAliasWithoutKey(validate.Sources)
	tran.Destination = getAliasWithoutKey(validate.Destinations)
	tran.Operations = operations

	if err := handler.Command.TransactionExecute(ctx, organizationID, ledgerID, &parserDSL, validate, balances, tran); err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "failed to update BTO", err)
		logger.Errorf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err)

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if err := handler.ensureAsyncTransactionVisibility(ctx, span, logger, tran, parserDSL); err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
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

	ensureTransactionDefaults(tran)

	if err := http.Created(c, tran); err != nil {
		return err
	}

	return nil
}

// ensureAsyncTransactionVisibility persists the transaction record synchronously when async processing is enabled.
// This keeps API semantics (e.g., immediate retrieval, commit on non-pending) consistent even when BTO runs async.
func (handler *TransactionHandler) ensureAsyncTransactionVisibility(ctx context.Context, span *trace.Span, logger libLog.Logger, tran *mmodel.Transaction, parserDSL pkgTransaction.Transaction) error {
	if !handler.isAsyncModeEnabled() {
		return nil
	}

	tranCopy := *tran

	switch tranCopy.Status.Code {
	case constant.CREATED:
		approved := constant.APPROVED
		tranCopy.Status = mmodel.Status{
			Code:        approved,
			Description: &approved,
		}
	case constant.PENDING:
		tranCopy.Body = parserDSL
	}

	if _, err := handler.Command.TransactionRepo.Create(ctx, &tranCopy); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			logger.Warnf("Transaction already exists while ensuring async visibility: %s", tran.ID)
			return nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to persist async transaction record", err)
		logger.Errorf("Failed to persist async transaction record: %v", err)

		return pkg.ValidateInternalError(err, "PersistAsyncTransaction")
	}

	return nil
}

// buildTransaction creates a transaction object with the provided data.
func (handler *TransactionHandler) buildTransaction(transactionID, parentID, organizationID, ledgerID uuid.UUID, parserDSL pkgTransaction.Transaction, transactionStatus string, transactionDate time.Time) *mmodel.Transaction {
	var parentTransactionID *string

	if parentID != uuid.Nil {
		str := parentID.String()
		parentTransactionID = &str
	}

	return &mmodel.Transaction{
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
		Status: mmodel.Status{
			Code:        transactionStatus,
			Description: &transactionStatus,
		},
	}
}

// commitOrCancelTransaction func that is responsible to commit or cancel transaction.
func (handler *TransactionHandler) commitOrCancelTransaction(c *fiber.Ctx, tran *mmodel.Transaction, transactionStatus string) error {
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
	// Validate state machine: only PENDING transactions can be committed/canceled
	validateTransactionStateTransition(tran.Status.Code, transactionStatus)

	organizationID := uuid.MustParse(tran.OrganizationID)
	ledgerID := uuid.MustParse(tran.LedgerID)
	txnID := uuid.MustParse(tran.ID)

	// Enrich wide event with transaction action context
	mlog.EnrichTransactionAction(c, organizationID, ledgerID, txnID, transactionStatus)
	mlog.SetHandler(c, "commit_or_cancel_transaction")

	deleteLockOnError, err := handler.acquireTransactionLock(ctx, &span, logger, organizationID, ledgerID, tran.ID)
	if err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if err := handler.validatePendingTransaction(&span, logger, tran, deleteLockOnError); err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	parserDSL := tran.Body
	handler.ApplyDefaultBalanceKeys(parserDSL.Send.Source.From)
	handler.ApplyDefaultBalanceKeys(parserDSL.Send.Distribute.To)

	fromTo := handler.buildCommitFromToList(parserDSL, transactionStatus)

	// Create a validation copy with concatenated aliases for unique map keys.
	// ValidateSendSourceAndDistribute needs unique keys in From/To maps.
	validationDSL := parserDSL
	validationDSL.Send.Source.From = handler.HandleAccountFields(parserDSL.Send.Source.From, true)
	validationDSL.Send.Distribute.To = handler.HandleAccountFields(parserDSL.Send.Distribute.To, true)

	validate, balances, err := handler.validateAndGetBalancesForCommit(ctx, tracer, &span, logger, organizationID, ledgerID, tran, validationDSL, transactionStatus, deleteLockOnError)
	if err != nil {
		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	return handler.executeCommitOrCancel(ctx, c, &span, logger, organizationID, ledgerID, tran, parserDSL, validate, balances, fromTo, transactionStatus)
}

// executeCommitOrCancel builds operations and executes the commit or cancel transaction.
// Note: fromTo contains concatenated aliases from buildCommitFromToList (for validation maps).
// We also add split aliases below (for matching balance.Alias in BuildOperations).
// HandleAccountFields returns copies, so calling it multiple times is safe.
func (handler *TransactionHandler) executeCommitOrCancel(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, tran *mmodel.Transaction, parserDSL pkgTransaction.Transaction, validate *pkgTransaction.Responses, balances []*mmodel.Balance, fromTo []pkgTransaction.FromTo, transactionStatus string) error {
	// Add split-alias entries for matching balance.Alias in BuildOperations
	// Note: HandleAccountFields now returns copies, so this won't corrupt the original parserDSL
	fromTo = append(fromTo, handler.HandleAccountFields(parserDSL.Send.Source.From, false)...)

	to := handler.HandleAccountFields(parserDSL.Send.Distribute.To, false)
	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	tran.UpdatedAt = time.Now()
	tran.Status = mmodel.Status{
		Code:        transactionStatus,
		Description: &transactionStatus,
	}

	operations, preBalances, err := handler.BuildOperations(ctx, balances, fromTo, parserDSL, *tran, validate, time.Now(), false)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate balances", err)
		logger.Errorf("Failed to validate balance: %v", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
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

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "commit_cancel_audit_log", mruntime.KeepRunning, func(ctx context.Context) {
		handler.Command.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, tran.IDtoUUID())
	})

	ensureTransactionDefaults(tran)

	if err := http.Created(c, tran); err != nil {
		return err
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

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	if !success {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is locked", err)
		logger.Errorf("Failed, Transaction: %s is locked, Error: %s", tranID, err.Error())

		return nil, err
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
func (handler *TransactionHandler) validatePendingTransaction(span *trace.Span, logger libLog.Logger, tran *mmodel.Transaction, deleteLockOnError func()) error {
	if tran.Status.Code != constant.PENDING {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction is not pending", err)
		logger.Errorf("Failed, Transaction: %s is not pending, Error: %s", tran.ID, err.Error())
		deleteLockOnError()

		return err
	}

	return nil
}

// buildCommitFromToList builds the initial fromTo list for commit/cancel operations.
func (handler *TransactionHandler) buildCommitFromToList(parserDSL pkgTransaction.Transaction, transactionStatus string) []pkgTransaction.FromTo {
	var fromTo []pkgTransaction.FromTo

	fromTo = append(fromTo, handler.HandleAccountFields(parserDSL.Send.Source.From, true)...)
	to := handler.HandleAccountFields(parserDSL.Send.Distribute.To, true)

	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, to...)
	}

	return fromTo
}

// validateAndGetBalancesForCommit validates transaction and retrieves balances for commit/cancel.
func (handler *TransactionHandler) validateAndGetBalancesForCommit(ctx context.Context, tracer trace.Tracer, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, tran *mmodel.Transaction, parserDSL pkgTransaction.Transaction, transactionStatus string, deleteLockOnError func()) (*pkgTransaction.Responses, []*mmodel.Balance, error) {
	validate, err := pkgTransaction.ValidateSendSourceAndDistribute(ctx, parserDSL, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send source and distribute", err)
		logger.Error("Failed to validate send source and distribute: %v", err.Error())
		businessErr := pkg.HandleKnownBusinessValidationErrors(err)

		deleteLockOnError()

		//nolint:wrapcheck // Returning typed business error directly to preserve error type for errors.As() matching
		return nil, nil, businessErr
	}

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")
	defer spanGetBalances.End()

	balances, err := handler.Query.GetBalances(ctx, organizationID, ledgerID, tran.IDtoUUID(), nil, validate, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanGetBalances, "Failed to get balances", err)
		logger.Errorf("Failed to get balances: %v", err.Error())
		deleteLockOnError()

		//nolint:wrapcheck // Returning error directly to preserve error type for errors.As() matching
		return nil, nil, err
	}

	return validate, balances, nil
}

// updateTransactionStatusIfAsync updates transaction status synchronously if async mode is enabled.
func (handler *TransactionHandler) updateTransactionStatusIfAsync(ctx context.Context, span *trace.Span, logger libLog.Logger, tran *mmodel.Transaction) {
	if handler.isAsyncModeEnabled() {
		if _, err := handler.Command.UpdateTransactionStatus(ctx, tran); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction status synchronously", err)
			logger.Errorf("Failed to update transaction status synchronously for Transaction: %s, Error: %s", tran.ID, err.Error())
		}
	}
}

// ApplyDefaultBalanceKeys sets the BalanceKey to "default" for any entries where the BalanceKey is empty.
func (handler *TransactionHandler) ApplyDefaultBalanceKeys(entries []pkgTransaction.FromTo) {
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

func ensureTransactionDefaults(tran *mmodel.Transaction) {
	if tran == nil {
		return
	}

	if tran.Metadata == nil {
		tran.Metadata = map[string]any{}
	}

	if tran.Operations == nil {
		tran.Operations = make([]*mmodel.Operation, 0)
	}
}
