// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// OfficialContextLoader enriches prepared, fee-inclusive entries in one batch.
// Invoke only after the authorized off/skip gates and within the total deadline.
type OfficialContextLoader struct {
	reader OfficialRecordsReader
	bounds tracercontract.Limits
}

func NewOfficialContextLoader(reader OfficialRecordsReader, bounds tracercontract.Limits) (*OfficialContextLoader, error) {
	if err := bounds.Validate(); err != nil {
		return nil, err
	}

	if reader == nil {
		return nil, constant.ErrInvalidRequestBody
	}

	return &OfficialContextLoader{reader: reader, bounds: bounds}, nil
}

func (l *OfficialContextLoader) EvaluationContext(ctx context.Context, org, ledger uuid.UUID, entries []PreparedEntry) (_ tracercontract.Context, retErr error) {
	if err := ctx.Err(); err != nil {
		return tracercontract.Context{}, err
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.load_tracer_context")
	defer span.End()
	defer func() { recordOfficialContextError(span, retErr) }()

	ids, err := l.entryReferences(ctx, entries)
	if err != nil {
		return tracercontract.Context{}, err
	}

	accounts, err := l.read(ctx, org, ledger, ids)
	if err != nil {
		return tracercontract.Context{}, err
	}

	return BuildEvaluationContext(ctx, ContextInput{OrganizationID: org, LedgerID: ledger, Accounts: accounts, Entries: entries}, l.bounds)
}

func (l *OfficialContextLoader) entryReferences(ctx context.Context, entries []PreparedEntry) ([]uuid.UUID, error) {
	if len(entries) == 0 || len(entries) > l.bounds.MaxEntries {
		return nil, constant.ErrInvalidRequestBody
	}

	accounts := make(map[uuid.UUID]struct{})

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if !tracercontract.ValidAssetCodeFact(entry.AssetCode) ||
			(entry.Direction != tracercontract.Debit && entry.Direction != tracercontract.Credit) ||
			(entry.External != (entry.AccountID == uuid.Nil)) || !entry.Amount.IsPositive() {
			return nil, constant.ErrInvalidRequestBody
		}

		if _, err := tracercontract.AmountFromDecimal(ctx, entry.Amount, l.bounds); err != nil {
			return nil, err
		}

		if !entry.External {
			accounts[entry.AccountID] = struct{}{}
		}
	}

	if len(accounts) > l.bounds.MaxAccounts {
		return nil, constant.ErrInvalidRequestBody
	}

	ids := make([]uuid.UUID, 0, len(accounts))
	for id := range accounts {
		ids = append(ids, id)
	}

	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

	return ids, nil
}

func (l *OfficialContextLoader) read(ctx context.Context, org, ledger uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	if org == uuid.Nil || ledger == uuid.Nil || len(ids) > l.bounds.MaxAccounts {
		return nil, constant.ErrInvalidRequestBody
	}

	expected := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || expected[id] {
			return nil, constant.ErrInvalidRequestBody
		}

		expected[id] = true
	}

	if len(ids) == 0 {
		return []*mmodel.Account{}, nil
	}

	accounts, err := l.reader.Read(ctx, org, ledger, ids)
	if err != nil {
		return nil, fmt.Errorf("load official tracer records: %w", err)
	}

	for _, account := range accounts {
		if account == nil {
			return nil, constant.ErrTracerFactsUnavailable
		}

		id, err := uuid.Parse(account.ID)
		if err != nil || !expected[id] {
			return nil, constant.ErrTracerFactsUnavailable
		}

		delete(expected, id)
	}

	if len(expected) != 0 {
		return nil, constant.ErrTracerFactsUnavailable
	}

	return accounts, nil
}

func recordOfficialContextError(span trace.Span, err error) {
	if err == nil {
		return
	}

	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		if pkg.IsBusinessError(pkg.ValidateBusinessError(cause, constant.EntityAccount)) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Official context rejected", err)
			return
		}
	}

	libOtel.HandleSpanError(span, "Official context load failed", err)
}
