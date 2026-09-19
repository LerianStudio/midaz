// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
)

// EngineWriteBehindEvidenceCodec adapts the canonical command evidence types
// to query's dependency-inverted lookup seam without making query import the
// command package.
type EngineWriteBehindEvidenceCodec struct{}

func (EngineWriteBehindEvidenceCodec) DecodeEngineTransactionIndex(
	ctx context.Context,
	raw []byte,
	organizationID, ledgerID, transactionID uuid.UUID,
) (uuid.UUID, bool, error) {
	index, err := DecodeTransactionEvidenceIndex(raw)
	if err != nil {
		return uuid.Nil, false, err
	}

	if index.TenantID != tmcore.GetTenantIDContext(ctx) || index.OrganizationID != organizationID || index.LedgerID != ledgerID || index.TransactionID != transactionID {
		return uuid.Nil, false, fmt.Errorf("engine transaction index scope mismatch: %w", ErrInvalidTransactionCompletionRecord)
	}

	return index.ExecutionID, index.DurabilityState == TransactionDurabilityPending, nil
}

func (EngineWriteBehindEvidenceCodec) BuildEngineTransactionLookup(
	ctx context.Context,
	rawIndex, rawEnvelope, rawReceipt []byte,
	organizationID, ledgerID, transactionID uuid.UUID,
) (*transaction.Transaction, error) {
	index, err := DecodeTransactionEvidenceIndex(rawIndex)
	if err != nil {
		return nil, err
	}

	if index.TenantID != tmcore.GetTenantIDContext(ctx) || index.OrganizationID != organizationID || index.LedgerID != ledgerID || index.TransactionID != transactionID {
		return nil, fmt.Errorf("engine transaction index scope mismatch: %w", ErrInvalidTransactionCompletionRecord)
	}

	envelope, err := DecodeTransactionWriteBehindEnvelope(rawEnvelope)
	if err != nil {
		return nil, err
	}

	if err := validateIndexedEnvelopeAndReceipt(*index, *envelope, rawReceipt); err != nil {
		return nil, err
	}

	views, err := BuildTransactionEvidenceViews(envelope.Record)
	if err != nil {
		return nil, err
	}

	return views.Lookup, nil
}

//nolint:gocyclo // every envelope/index/receipt correlation invariant is explicit and fail-closed
func validateIndexedEnvelopeAndReceipt(index TransactionEvidenceIndex, envelope TransactionWriteBehindEnvelope, rawReceipt []byte) error {
	if envelope.Record.TenantID != index.TenantID || envelope.Record.OrganizationID != index.OrganizationID || envelope.Record.LedgerID != index.LedgerID || envelope.Record.TransactionID != index.TransactionID || envelope.Record.ExecutionID != index.ExecutionID || envelope.ApplicationState != index.ApplicationState || envelope.ReplayState != index.ReplayState || envelope.DurabilityState != index.DurabilityState || !slices.Equal(envelope.Dependencies, index.Dependencies) {
		return fmt.Errorf("indexed engine transaction evidence mismatch: %w", ErrInvalidTransactionCompletionRecord)
	}

	plan, err := DecodeTransactionCompletionPlan([]byte(envelope.Record.Payload))
	if err != nil || plan.Action != index.Action {
		return fmt.Errorf("indexed engine transaction action mismatch: %w", ErrInvalidTransactionCompletionRecord)
	}

	var receipt struct {
		FormatVersion     int       `json:"formatVersion"`
		TenantID          string    `json:"tenantId"`
		OrganizationID    uuid.UUID `json:"organizationId"`
		LedgerID          uuid.UUID `json:"ledgerId"`
		ExecutionID       uuid.UUID `json:"executionId"`
		IntentFingerprint string    `json:"intentFingerprint"`
		Response          string    `json:"response"`
		Protection        struct {
			FormatVersion  int              `json:"formatVersion"`
			Transactions   []uuid.UUID      `json:"transactions"`
			RecoveryFields []string         `json:"recoveryFields"`
			IndexFields    []uuid.UUID      `json:"indexFields"`
			Acknowledged   map[string]bool  `json:"acknowledged"`
			Terminal       map[string]int64 `json:"terminalCompletedAtMs"`
		} `json:"protection"`
	}

	decoder := json.NewDecoder(bytes.NewReader(rawReceipt))
	if err := decoder.Decode(&receipt); err != nil {
		return fmt.Errorf("decode engine transaction receipt: %w", err)
	}

	if receipt.FormatVersion != 1 || receipt.Protection.FormatVersion != 2 || receipt.TenantID != index.TenantID || receipt.OrganizationID != index.OrganizationID || receipt.LedgerID != index.LedgerID || receipt.ExecutionID != index.ExecutionID || receipt.IntentFingerprint != envelope.Record.IntentFingerprint || receipt.Response == "" {
		return fmt.Errorf("engine transaction receipt identity mismatch: %w", ErrInvalidTransactionCompletionRecord)
	}

	var response struct {
		ProtocolVersion int               `json:"protocolVersion"`
		Movements       []json.RawMessage `json:"movements"`
		Final           []json.RawMessage `json:"final"`
	}
	if err := json.Unmarshal([]byte(receipt.Response), &response); err != nil || response.ProtocolVersion != 1 || len(response.Movements) == 0 || len(response.Final) == 0 {
		return fmt.Errorf("engine transaction receipt response is invalid: %w", ErrInvalidTransactionCompletionRecord)
	}

	if len(receipt.Protection.Transactions) != len(receipt.Protection.RecoveryFields) || len(receipt.Protection.Transactions) != len(receipt.Protection.IndexFields) {
		return fmt.Errorf("engine transaction receipt protection mismatch: %w", ErrInvalidTransactionCompletionRecord)
	}

	for position, protectedID := range receipt.Protection.Transactions {
		if protectedID == index.TransactionID && receipt.Protection.RecoveryFields[position] == index.RecoveryField && receipt.Protection.IndexFields[position] == index.TransactionID {
			return nil
		}
	}

	return fmt.Errorf("engine transaction receipt does not protect index: %w", ErrInvalidTransactionCompletionRecord)
}
