// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
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

// BalanceEngineFinalizer confirms SQL rows and their frozen metadata without
// executing accounting or removing the durable recovery envelope.
type BalanceEngineFinalizer struct {
	store    BalanceEngineRecoveryStore
	metadata balanceEngineMetadataRepository
}

// NewBalanceEngineFinalizer uses the existing metadata repository with the
// authenticated Mongo context supplied by its caller.
func NewBalanceEngineFinalizer(store BalanceEngineRecoveryStore, metadata balanceEngineMetadataRepository) *BalanceEngineFinalizer {
	return &BalanceEngineFinalizer{store: store, metadata: metadata}
}

// Finalize returns nil only after SQL commit and metadata verification succeed.
// Any error leaves the caller responsible for retaining the recovery envelope.
func (finalizer *BalanceEngineFinalizer) Finalize(ctx context.Context, envelope *BalanceEngineRecoveryEnvelope) error {
	_, err := finalizer.finalize(ctx, envelope, false)

	return err
}

// FinalizeWithOutcome returns the status observed by the durable SQL store only
// after SQL commit and all frozen metadata verification succeed.
func (finalizer *BalanceEngineFinalizer) FinalizeWithOutcome(ctx context.Context, envelope *BalanceEngineRecoveryEnvelope) (BalanceEngineRecoveryOutcome, error) {
	return finalizer.finalize(ctx, envelope, true)
}

func (finalizer *BalanceEngineFinalizer) finalize(ctx context.Context, envelope *BalanceEngineRecoveryEnvelope, requireOutcome bool) (BalanceEngineRecoveryOutcome, error) {
	if err := ctx.Err(); err != nil {
		return BalanceEngineRecoveryOutcome{}, err
	}

	if finalizer == nil || finalizer.store == nil || finalizer.metadata == nil {
		return BalanceEngineRecoveryOutcome{}, invalidRecovery("recovery finalizer dependencies are not configured")
	}

	if envelope == nil {
		return BalanceEngineRecoveryOutcome{}, invalidRecovery("recovery envelope is missing")
	}

	if envelope.TenantID != tmcore.GetTenantIDContext(ctx) {
		return BalanceEngineRecoveryOutcome{}, invalidRecovery("recovery tenant does not match authenticated context")
	}

	if err := validateRecoveryEnvelope(*envelope); err != nil {
		return BalanceEngineRecoveryOutcome{}, err
	}

	payload, err := DecodeBalanceEngineRecoveryPayload([]byte(envelope.Payload))
	if err != nil {
		return BalanceEngineRecoveryOutcome{}, err
	}

	record, err := frozenPersistenceRecord(*payload, envelope)
	if err != nil {
		return BalanceEngineRecoveryOutcome{}, err
	}

	metadata, err := frozenMetadataRecords(record.Transaction, payload.TransactionDate)
	if err != nil {
		return BalanceEngineRecoveryOutcome{}, err
	}

	outcome, err := finalizer.persist(ctx, record, requireOutcome)
	if err != nil {
		return BalanceEngineRecoveryOutcome{}, err
	}

	for _, entry := range metadata {
		if err := ctx.Err(); err != nil {
			return BalanceEngineRecoveryOutcome{}, err
		}

		if err := finalizer.persistMetadata(ctx, entry); err != nil {
			return BalanceEngineRecoveryOutcome{}, err
		}
	}

	return outcome, nil
}

func (finalizer *BalanceEngineFinalizer) persist(ctx context.Context, record BalanceEnginePersistenceRecord, requireOutcome bool) (BalanceEngineRecoveryOutcome, error) {
	if !requireOutcome {
		if err := finalizer.store.Persist(ctx, record); err != nil {
			return BalanceEngineRecoveryOutcome{}, fmt.Errorf("persist recovered SQL rows: %w", err)
		}

		return BalanceEngineRecoveryOutcome{}, nil
	}

	outcomeStore, ok := finalizer.store.(BalanceEngineRecoveryStoreWithOutcome)
	if !ok {
		return BalanceEngineRecoveryOutcome{}, invalidRecovery("recovery SQL store does not report durable transaction status")
	}

	outcome, err := outcomeStore.PersistWithOutcome(ctx, record)
	if err != nil {
		return BalanceEngineRecoveryOutcome{}, fmt.Errorf("persist recovered SQL rows: %w", err)
	}

	if !validRecoveryOutcome(outcome) {
		return BalanceEngineRecoveryOutcome{}, fmt.Errorf("%w: recovery SQL store reported unknown transaction status", ErrBalanceEnginePersistenceConflict)
	}

	return outcome, nil
}

func validRecoveryOutcome(outcome BalanceEngineRecoveryOutcome) bool {
	switch outcome.TransactionStatus {
	case constant.PENDING, constant.APPROVED, constant.CANCELED:
		return true
	default:
		return false
	}
}

func frozenPersistenceRecord(payload BalanceEngineRecoveryPayload, envelope *BalanceEngineRecoveryEnvelope) (BalanceEnginePersistenceRecord, error) {
	rows, err := ProjectBalanceEngineOperations(payload, envelope.Result)
	if err != nil {
		return BalanceEnginePersistenceRecord{}, err
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
		tran.Source = append([]string(nil), payload.Validate.Sources...)
		tran.Destination = append([]string(nil), payload.Validate.Destinations...)
	} else {
		tran.Source = frozenAccountAliases(payload.TransactionInput.Send.Source.From)
		tran.Destination = frozenAccountAliases(payload.TransactionInput.Send.Distribute.To)
	}

	expectedStatus := ""
	if payload.Action == constant.ActionCommit || payload.Action == constant.ActionCancel {
		expectedStatus = constant.PENDING
	}

	return BalanceEnginePersistenceRecord{Transaction: tran, Action: payload.Action, ExpectedStatus: expectedStatus}, nil
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

func (finalizer *BalanceEngineFinalizer) persistMetadata(ctx context.Context, expected *mongodb.Metadata) error {
	if err := finalizer.metadata.Create(ctx, expected.EntityName, expected); err != nil {
		return fmt.Errorf("create recovered metadata: %w", err)
	}

	actual, err := finalizer.metadata.FindByEntity(ctx, expected.EntityName, expected.EntityID)
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
