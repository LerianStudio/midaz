// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// ErrBalanceEngineMetadataConflict identifies metadata that cannot be confirmed
// without changing the frozen value or replacing an existing document.
var ErrBalanceEngineMetadataConflict = errors.New("balance engine metadata conflict")

type balanceEngineMetadataRepository interface {
	Create(context.Context, string, *mongodb.Metadata) error
	FindByEntity(context.Context, string, string) (*mongodb.Metadata, error)
}

// TransactionCompletionService durably materializes an applied accounting result
// without executing accounting or removing its completion record.
type TransactionCompletionService struct {
	store     TransactionWriteStore
	metadata  balanceEngineMetadataRepository
	publisher BalanceEngineEventPublisher
}

// NewTransactionCompletionService uses the existing metadata repository with the
// authenticated Mongo context supplied by its caller.
func NewTransactionCompletionService(store TransactionWriteStore, metadata balanceEngineMetadataRepository) *TransactionCompletionService {
	return &TransactionCompletionService{store: store, metadata: metadata}
}

// NewTransactionCompletionServiceWithEvents enables best-effort event dispatch after
// SQL and frozen metadata have both been confirmed.
func NewTransactionCompletionServiceWithEvents(
	store TransactionWriteStore,
	metadata balanceEngineMetadataRepository,
	publisher BalanceEngineEventPublisher,
) (*TransactionCompletionService, error) {
	if publisher == nil || (reflect.ValueOf(publisher).Kind() == reflect.Pointer && reflect.ValueOf(publisher).IsNil()) {
		return nil, invalidTransactionCompletionRecord("balance engine event publisher is not configured")
	}

	if _, ok := store.(TransactionWriteStoreWithOutcome); !ok {
		return nil, invalidTransactionCompletionRecord("transaction write store does not report durable transaction status")
	}

	return &TransactionCompletionService{store: store, metadata: metadata, publisher: publisher}, nil
}

// Complete returns the durable status and a caller-owned copy of the
// exact operation records only after SQL commit and metadata verification.
func (service *TransactionCompletionService) Complete(ctx context.Context, record *TransactionCompletionRecord) (TransactionCompletionResult, error) {
	return service.complete(ctx, record)
}

func (service *TransactionCompletionService) complete(ctx context.Context, record *TransactionCompletionRecord) (TransactionCompletionResult, error) {
	if err := ctx.Err(); err != nil {
		return TransactionCompletionResult{}, err
	}

	if service == nil || service.store == nil || service.metadata == nil {
		return TransactionCompletionResult{}, invalidTransactionCompletionRecord("transaction completion dependencies are not configured")
	}

	if record == nil {
		return TransactionCompletionResult{}, invalidTransactionCompletionRecord("transaction completion record is missing")
	}

	if record.TenantID != tmcore.GetTenantIDContext(ctx) {
		return TransactionCompletionResult{}, invalidTransactionCompletionRecord("completion tenant does not match authenticated context")
	}

	if err := validateTransactionCompletionRecord(*record); err != nil {
		return TransactionCompletionResult{}, err
	}

	plan, err := DecodeTransactionCompletionPlan([]byte(record.Payload))
	if err != nil {
		return TransactionCompletionResult{}, err
	}

	writeSet, err := BuildTransactionWriteSet(*plan, record.Result)
	if err != nil {
		return TransactionCompletionResult{}, err
	}

	metadata, err := frozenMetadataRecords(writeSet.Transaction, plan.TransactionDate)
	if err != nil {
		return TransactionCompletionResult{}, err
	}

	callerWriteSet, err := cloneTransactionWriteSet(writeSet)
	if err != nil {
		return TransactionCompletionResult{}, err
	}

	outcome, err := service.persist(ctx, writeSet)
	if err != nil {
		return TransactionCompletionResult{}, err
	}

	for _, entry := range metadata {
		if err := ctx.Err(); err != nil {
			return TransactionCompletionResult{}, err
		}

		if err := service.persistMetadata(ctx, entry); err != nil {
			return TransactionCompletionResult{}, err
		}
	}

	if service.publisher != nil {
		if !validTransactionLifecyclePhase(outcome.LifecyclePhase) {
			return TransactionCompletionResult{}, fmt.Errorf("%w: transaction write store reported unknown lifecycle phase", ErrTransactionCompletionConflict)
		}

		service.publisher.PublishBalanceEngineEvents(ctx, writeSet.Transaction, outcome.LifecyclePhase)
	}

	return TransactionCompletionResult{Record: callerWriteSet, Outcome: outcome}, nil
}

func validTransactionLifecyclePhase(phase string) bool {
	switch phase {
	case TransactionLifecyclePhaseCreated, TransactionLifecyclePhaseUpdated, TransactionLifecyclePhaseNoop:
		return true
	default:
		return false
	}
}

func (service *TransactionCompletionService) persist(ctx context.Context, writeSet TransactionWriteSet) (TransactionPersistenceOutcome, error) {
	outcomeStore, ok := service.store.(TransactionWriteStoreWithOutcome)
	if !ok {
		return TransactionPersistenceOutcome{}, invalidTransactionCompletionRecord("transaction write store does not report durable transaction status")
	}

	outcome, err := outcomeStore.PersistWithOutcome(ctx, writeSet)
	if err != nil {
		return TransactionPersistenceOutcome{}, fmt.Errorf("persist transaction write set: %w", err)
	}

	if !validTransactionPersistenceOutcome(outcome) {
		return TransactionPersistenceOutcome{}, fmt.Errorf("%w: transaction write store reported unknown transaction status", ErrTransactionCompletionConflict)
	}

	return outcome, nil
}

func validTransactionPersistenceOutcome(outcome TransactionPersistenceOutcome) bool {
	switch outcome.TransactionStatus {
	case constant.PENDING, constant.APPROVED, constant.CANCELED:
		return true
	default:
		return false
	}
}

// BuildTransactionWriteSet is the single final composition seam
// from frozen transaction context plus an authoritative engine result to the
// deterministic SQL transaction and operation rows used by normal completion
// and recovery.
func BuildTransactionWriteSet(payload TransactionCompletionPlan, result engine.Result) (TransactionWriteSet, error) {
	rows, err := BuildOperationRecordsFromMovements(payload, result)
	if err != nil {
		return TransactionWriteSet{}, err
	}

	status := payload.TransactionStatus
	if status == constant.CREATED {
		status = constant.APPROVED
	}

	amount := payload.TransactionInput.Send.Value

	tran := &transaction.Transaction{
		ID: payload.TransactionID.String(), OrganizationID: payload.OrganizationID.String(), LedgerID: payload.LedgerID.String(),
		Amount: &amount, AssetCode: payload.TransactionInput.Send.Asset, Description: payload.TransactionInput.Description,
		ChartOfAccountsGroupName: payload.TransactionInput.ChartOfAccountsGroupName,
		Status:                   transaction.Status{Code: status, Description: &status},
		CreatedAt:                payload.TransactionCreatedAt, UpdatedAt: payload.TransactionUpdatedAt,
		Route: payload.TransactionInput.Route, RouteID: payload.TransactionInput.RouteID, //nolint:staticcheck // Preserve the frozen legacy route column alongside its canonical ID.
		FeesSkipped: payload.FeesSkipped, TracerSkipped: payload.TracerSkipped,
		Metadata: payload.TransactionInput.Metadata, Operations: rows,
	}
	if payload.ParentTransactionID != nil {
		parentID := payload.ParentTransactionID.String()
		tran.ParentTransactionID = &parentID
	}

	if status == constant.PENDING {
		tran.Body = payload.TransactionInput
	}

	if payload.Validate != nil {
		tran.Source = getAliasWithoutKey(filterCompanionAliases(payload.Validate.Sources))
		tran.Destination = getAliasWithoutKey(filterCompanionAliases(payload.Validate.Destinations))
	} else {
		tran.Source = frozenAccountAliases(payload.TransactionInput.Send.Source.From)
		tran.Destination = frozenAccountAliases(payload.TransactionInput.Send.Distribute.To)
	}

	expectedStatus := ""
	if payload.Action == constant.ActionCommit || payload.Action == constant.ActionCancel {
		expectedStatus = constant.PENDING
	}

	return TransactionWriteSet{Transaction: tran, Action: payload.Action, ExpectedStatus: expectedStatus}, nil
}

func cloneTransactionWriteSet(record TransactionWriteSet) (TransactionWriteSet, error) {
	if record.Transaction == nil {
		return TransactionWriteSet{}, invalidTransactionCompletionRecord("projected persistence record is missing its transaction")
	}

	tran := *record.Transaction
	tran.ParentTransactionID = cloneTextPointer(record.Transaction.ParentTransactionID)
	tran.Status.Description = cloneTextPointer(record.Transaction.Status.Description)
	tran.Amount = cloneDecimalPointer(record.Transaction.Amount)
	tran.Source = append([]string(nil), record.Transaction.Source...)
	tran.Destination = append([]string(nil), record.Transaction.Destination...)
	tran.RouteID = cloneTextPointer(record.Transaction.RouteID)
	tran.DeletedAt = cloneTimePointer(record.Transaction.DeletedAt)
	tran.Metadata = maps.Clone(record.Transaction.Metadata)

	if !record.Transaction.Body.IsEmpty() {
		body, err := clonePendingTransactionInput(record.Transaction.Body)
		if err != nil {
			return TransactionWriteSet{}, fmt.Errorf("clone materialized transaction body: %w", err)
		}

		tran.Body = body
	}

	tran.Operations = make([]*operation.Operation, len(record.Transaction.Operations))
	for index, row := range record.Transaction.Operations {
		if row == nil {
			return TransactionWriteSet{}, invalidTransactionCompletionRecord("projected persistence record contains a nil operation")
		}

		cloned := *row
		cloned.Amount.Value = cloneDecimalPointer(row.Amount.Value)
		cloned.Balance.Available = cloneDecimalPointer(row.Balance.Available)
		cloned.Balance.OnHold = cloneDecimalPointer(row.Balance.OnHold)
		cloned.Balance.Version = cloneInt64Pointer(row.Balance.Version)
		cloned.Balance.OverdraftUsed = cloneDecimal(row.Balance.OverdraftUsed)
		cloned.BalanceAfter.Available = cloneDecimalPointer(row.BalanceAfter.Available)
		cloned.BalanceAfter.OnHold = cloneDecimalPointer(row.BalanceAfter.OnHold)
		cloned.BalanceAfter.Version = cloneInt64Pointer(row.BalanceAfter.Version)
		cloned.BalanceAfter.OverdraftUsed = cloneDecimal(row.BalanceAfter.OverdraftUsed)
		cloned.Status.Description = cloneTextPointer(row.Status.Description)
		cloned.RouteID = cloneTextPointer(row.RouteID)
		cloned.RouteCode = cloneTextPointer(row.RouteCode)
		cloned.RouteDescription = cloneTextPointer(row.RouteDescription)
		cloned.DeletedAt = cloneTimePointer(row.DeletedAt)
		cloned.Metadata = maps.Clone(row.Metadata)
		tran.Operations[index] = &cloned
	}

	return TransactionWriteSet{Transaction: &tran, Action: record.Action, ExpectedStatus: record.ExpectedStatus}, nil
}

func cloneTextPointer(value *string) *string {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}

func cloneDecimalPointer(value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return nil
	}

	cloned := cloneDecimal(*value)

	return &cloned
}

func cloneDecimal(value decimal.Decimal) decimal.Decimal {
	if value.IsZero() {
		return value
	}

	return decimal.NewFromBigInt(value.Coefficient(), value.Exponent())
}

func frozenAccountAliases(entries []mtransaction.FromTo) []string {
	aliases := make([]string, 0, len(entries))
	for _, entry := range entries {
		aliases = append(aliases, mtransaction.SplitAlias(entry.AccountAlias))
	}

	return aliases
}

func frozenMetadataRecords(tran *transaction.Transaction, date time.Time) ([]*mongodb.Metadata, error) {
	metadata := make([]*mongodb.Metadata, 0, len(tran.Operations)+1)
	appendMetadata := func(entity, id string, data map[string]any) error {
		if data == nil {
			return nil
		}

		normalized, err := normalizeFrozenMetadata(data)
		if err != nil {
			return err
		}

		metadata = append(metadata, &mongodb.Metadata{EntityID: id, EntityName: entity, Data: normalized, CreatedAt: date, UpdatedAt: date})

		return nil
	}

	if err := appendMetadata(constant.EntityTransaction, tran.ID, tran.Metadata); err != nil {
		return nil, err
	}

	for _, row := range tran.Operations {
		if err := appendMetadata(constant.EntityOperation, row.ID, row.Metadata); err != nil {
			return nil, err
		}
	}

	return metadata, nil
}

func (service *TransactionCompletionService) persistMetadata(ctx context.Context, expected *mongodb.Metadata) error {
	if err := service.metadata.Create(ctx, expected.EntityName, expected); err != nil {
		return fmt.Errorf("create recovered metadata: %w", err)
	}

	actual, err := service.metadata.FindByEntity(ctx, expected.EntityName, expected.EntityID)
	if err != nil {
		return fmt.Errorf("verify recovered metadata: %w", err)
	}

	if actual == nil || actual.EntityID != expected.EntityID || actual.EntityName != expected.EntityName {
		return metadataConflict("metadata identity is not confirmed")
	}

	return compareFrozenMetadata(expected.Data, actual.Data)
}

func normalizeFrozenMetadata(data map[string]any) (mongodb.JSON, error) {
	normalized := make(mongodb.JSON, len(data))
	for key, value := range data {
		number, numeric, err := canonicalMetadataNumber(value)
		if err != nil {
			return nil, err
		}

		if !numeric {
			if !metadataScalar(value) {
				return nil, metadataConflict("metadata values must be flat JSON scalars")
			}

			normalized[key] = value

			continue
		}

		encoded, err := number.bsonNumber()
		if err != nil {
			return nil, err
		}

		normalized[key] = encoded
	}

	return normalized, nil
}

func compareFrozenMetadata(expected, actual mongodb.JSON) error {
	if len(expected) != len(actual) {
		return metadataConflict("stored metadata differs from frozen content")
	}

	for key, value := range expected {
		other, exists := actual[key]
		if !exists {
			return metadataConflict("stored metadata is missing a frozen field")
		}

		left, leftNumeric, err := canonicalMetadataNumber(value)
		if err != nil {
			return err
		}

		right, rightNumeric, err := canonicalMetadataNumber(other)
		if err != nil {
			return err
		}

		if leftNumeric || rightNumeric {
			if !leftNumeric || !rightNumeric || left != right {
				return metadataConflict("stored metadata number differs from frozen content")
			}

			continue
		}

		if !metadataScalar(value) || !metadataScalar(other) || value != other {
			return metadataConflict("stored metadata differs from frozen content")
		}
	}

	return nil
}

func metadataScalar(value any) bool {
	switch value.(type) {
	case nil, bool, string:
		return true
	default:
		return false
	}
}

type metadataNumber struct {
	coefficient string
	exponent    int64
}

func canonicalMetadataNumber(value any) (metadataNumber, bool, error) {
	text, numeric, err := metadataNumberText(value)
	if err != nil || !numeric {
		return metadataNumber{}, numeric, err
	}

	number, err := parseMetadataNumber(text)

	return number, true, err
}

func metadataNumberText(value any) (string, bool, error) {
	switch number := value.(type) {
	case json.Number:
		return number.String(), true, nil
	case int:
		return strconv.FormatInt(int64(number), 10), true, nil
	case int8:
		return strconv.FormatInt(int64(number), 10), true, nil
	case int16:
		return strconv.FormatInt(int64(number), 10), true, nil
	case int32:
		return strconv.FormatInt(int64(number), 10), true, nil
	case int64:
		return strconv.FormatInt(number, 10), true, nil
	case uint:
		return strconv.FormatUint(uint64(number), 10), true, nil
	case uint8:
		return strconv.FormatUint(uint64(number), 10), true, nil
	case uint16:
		return strconv.FormatUint(uint64(number), 10), true, nil
	case uint32:
		return strconv.FormatUint(uint64(number), 10), true, nil
	case uint64:
		return strconv.FormatUint(number, 10), true, nil
	case float32:
		return metadataFloatText(float64(number), 32)
	case float64:
		return metadataFloatText(number, 64)
	case bson.Decimal128:
		return number.String(), true, nil
	default:
		return "", false, nil
	}
}

func metadataFloatText(value float64, bits int) (string, bool, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "", true, metadataConflict("metadata number is not finite")
	}

	return strconv.FormatFloat(value, 'g', -1, bits), true, nil
}

func parseMetadataNumber(text string) (metadataNumber, error) {
	if len(text) > 2000 || !json.Valid([]byte(text)) {
		return metadataNumber{}, metadataConflict("metadata number is not valid JSON")
	}

	value, err := decimal.NewFromString(text)
	if err != nil {
		return metadataNumber{}, metadataConflict("metadata number is not a decimal")
	}

	coefficient := value.Coefficient().String()
	if coefficient == "0" {
		return metadataNumber{coefficient: "0"}, nil
	}

	trimmed := strings.TrimRight(coefficient, "0")

	return metadataNumber{coefficient: trimmed, exponent: int64(value.Exponent()) + int64(len(coefficient)-len(trimmed))}, nil
}

func (number metadataNumber) bsonNumber() (any, error) {
	if number.exponent >= 0 && number.exponent <= 19 && int64(len(number.coefficient))+number.exponent <= 20 {
		integer, err := strconv.ParseInt(number.coefficient+strings.Repeat("0", int(number.exponent)), 10, 64)
		if err == nil {
			return integer, nil
		}
	}

	value, err := strconv.ParseFloat(number.coefficient+"e"+strconv.FormatInt(number.exponent, 10), 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, metadataConflict("metadata number cannot be stored without loss")
	}

	roundTrip, err := parseMetadataNumber(strconv.FormatFloat(value, 'g', -1, 64))
	if err != nil || roundTrip != number {
		return nil, metadataConflict("metadata number cannot be stored without loss")
	}

	return value, nil
}

func metadataConflict(reason string) error {
	return fmt.Errorf("%w: %s", ErrBalanceEngineMetadataConflict, reason)
}
