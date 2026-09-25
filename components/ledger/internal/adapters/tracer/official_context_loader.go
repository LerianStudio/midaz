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
	"strings"
	"unicode/utf8"

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
// Its namespace is trusted configuration, never a field selected by the caller.
// Invoke only after the authorized off/skip gates and within the total deadline.
type OfficialContextLoader struct {
	reader    OfficialRecordsReader
	namespace string
	bounds    tracercontract.Limits
}

func NewOfficialContextLoader(reader OfficialRecordsReader, namespace string, bounds tracercontract.Limits) (*OfficialContextLoader, error) {
	if err := bounds.Validate(); err != nil {
		return nil, err
	}

	if reader == nil || !officialText(namespace, bounds.MaxTextBytes) {
		return nil, constant.ErrInvalidRequestBody
	}

	return &OfficialContextLoader{reader: reader, namespace: namespace, bounds: bounds}, nil
}

func (l *OfficialContextLoader) EvaluationContext(ctx context.Context, org, ledger uuid.UUID, entries []PreparedEntry) (_ tracercontract.Context, retErr error) {
	if err := ctx.Err(); err != nil {
		return tracercontract.Context{}, err
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.load_tracer_context")
	defer span.End()
	defer func() { recordOfficialContextError(span, retErr) }()

	ids, codes, err := l.entryReferences(ctx, entries)
	if err != nil {
		return tracercontract.Context{}, err
	}

	accounts, assets, err := l.read(ctx, org, ledger, ids, codes)
	if err != nil {
		return tracercontract.Context{}, err
	}

	return BuildEvaluationContext(ctx, ContextInput{Namespace: l.namespace, OrganizationID: org, LedgerID: ledger, Accounts: accounts, Assets: assets, Entries: entries}, l.bounds)
}

// AccountAssets loads official facts for migration/administration without
// fabricating transaction entries or transferring limit policy into Ledger.
func (l *OfficialContextLoader) AccountAssets(ctx context.Context, org, ledger uuid.UUID, ids []uuid.UUID) (_ []tracercontract.AccountAsset, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.load_account_assets")
	defer span.End()
	defer func() { recordOfficialContextError(span, retErr) }()

	if len(ids) == 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	ordered := slices.Clone(ids)
	slices.SortFunc(ordered, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

	accounts, assets, err := l.read(ctx, org, ledger, ordered, nil)
	if err != nil {
		return nil, err
	}

	return BuildAccountAssets(ctx, AccountAssetsInput{Namespace: l.namespace, OrganizationID: org, LedgerID: ledger, Accounts: accounts, Assets: assets}, l.bounds)
}

func (l *OfficialContextLoader) entryReferences(ctx context.Context, entries []PreparedEntry) ([]uuid.UUID, []string, error) {
	if len(entries) == 0 || len(entries) > l.bounds.MaxEntries {
		return nil, nil, constant.ErrInvalidRequestBody
	}

	accounts := make(map[uuid.UUID]struct{})
	assets := make(map[string]struct{})

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		if !officialText(entry.AssetCode, l.bounds.MaxTextBytes) ||
			(entry.Direction != tracercontract.Debit && entry.Direction != tracercontract.Credit) ||
			(entry.External != (entry.AccountID == uuid.Nil)) || !entry.Amount.IsPositive() {
			return nil, nil, constant.ErrInvalidRequestBody
		}

		if _, err := tracercontract.AmountFromDecimal(ctx, entry.Amount, l.bounds); err != nil {
			return nil, nil, err
		}

		if !entry.External {
			accounts[entry.AccountID] = struct{}{}
		}

		assets[entry.AssetCode] = struct{}{}
	}

	if len(accounts) > l.bounds.MaxAccounts {
		return nil, nil, constant.ErrInvalidRequestBody
	}

	ids := make([]uuid.UUID, 0, len(accounts))
	for id := range accounts {
		ids = append(ids, id)
	}

	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

	codes := make([]string, 0, len(assets))
	for code := range assets {
		codes = append(codes, code)
	}

	slices.Sort(codes)

	return ids, codes, nil
}

func (l *OfficialContextLoader) read(ctx context.Context, org, ledger uuid.UUID, ids []uuid.UUID, codes []string) ([]*mmodel.Account, []*mmodel.Asset, error) {
	if org == uuid.Nil || ledger == uuid.Nil || len(ids) > l.bounds.MaxAccounts {
		return nil, nil, constant.ErrInvalidRequestBody
	}

	expected := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || expected[id] {
			return nil, nil, constant.ErrInvalidRequestBody
		}

		expected[id] = true
	}

	accounts, assets, err := l.reader.Read(ctx, org, ledger, ids, codes)
	if err != nil {
		return nil, nil, fmt.Errorf("load official tracer records: %w", err)
	}

	for _, account := range accounts {
		if account == nil {
			return nil, nil, constant.ErrTracerFactsUnavailable
		}

		id, err := uuid.Parse(account.ID)
		if err != nil || !expected[id] {
			return nil, nil, constant.ErrTracerFactsUnavailable
		}

		delete(expected, id)
	}

	if len(expected) != 0 {
		return nil, nil, constant.ErrTracerFactsUnavailable
	}

	return accounts, assets, nil
}

func officialText(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
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
