// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package protobuf translates the shared reservation contract without importing
// either producer or Tracer components. It never applies business policy.
package protobuf

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func DecodeReserve(ctx context.Context, p *reservationv1.ReserveRequest, namespace string, bounds tracercontract.Limits, maxBodyBytes int) (tracercontract.ReserveRequest, error) {
	if err := ctx.Err(); err != nil {
		return tracercontract.ReserveRequest{}, err
	}

	if err := bounds.Validate(); err != nil {
		return tracercontract.ReserveRequest{}, err
	}

	if unknown(p) || unknown(p.GetContext()) || maxBodyBytes <= 0 || len(p.Context.Accounts) > bounds.MaxAccounts || len(p.Context.Entries) > bounds.MaxEntries || proto.Size(p) > maxBodyBytes {
		return tracercontract.ReserveRequest{}, constant.ErrInvalidRequestBody
	}

	r, err := decodeEnvelope(p)
	if err != nil {
		return tracercontract.ReserveRequest{}, err
	}

	r.Context.Accounts = make([]tracercontract.Account, 0, len(p.Context.Accounts))

	r.Context.Entries = make([]tracercontract.Entry, 0, len(p.Context.Entries))
	for _, account := range p.Context.Accounts {
		if err := ctx.Err(); err != nil {
			return tracercontract.ReserveRequest{}, err
		}

		decoded, err := decodeAccount(account)
		if err != nil {
			return tracercontract.ReserveRequest{}, err
		}

		r.Context.Accounts = append(r.Context.Accounts, decoded)
	}

	for _, entry := range p.Context.Entries {
		if err := ctx.Err(); err != nil {
			return tracercontract.ReserveRequest{}, err
		}

		decoded, err := decodeEntry(entry)
		if err != nil {
			return tracercontract.ReserveRequest{}, err
		}

		r.Context.Entries = append(r.Context.Entries, decoded)
	}

	if err := r.Validate(ctx, namespace, bounds); err != nil {
		return tracercontract.ReserveRequest{}, err
	}

	return r, nil
}

func decodeAccount(account *reservationv1.ContextAccount) (tracercontract.Account, error) {
	if unknown(account) {
		return tracercontract.Account{}, constant.ErrInvalidRequestBody
	}

	id, err := uuid.Parse(account.Id)
	if err != nil {
		return tracercontract.Account{}, constant.ErrInvalidRequestBody
	}

	asset, err := decodeAsset(account.Asset)
	if err != nil {
		return tracercontract.Account{}, err
	}

	return tracercontract.Account{ID: id, Type: account.Type, Status: account.Status, Blocked: copyBool(account.Blocked), Asset: asset}, nil
}

func decodeEnvelope(p *reservationv1.ReserveRequest) (tracercontract.ReserveRequest, error) {
	transaction, err := uuid.Parse(p.TransactionId)
	if err != nil {
		return tracercontract.ReserveRequest{}, constant.ErrInvalidRequestBody
	}

	request, err := uuid.Parse(p.RequestId)
	if err != nil {
		return tracercontract.ReserveRequest{}, constant.ErrInvalidRequestBody
	}

	at, err := time.Parse(time.RFC3339Nano, p.TransactionTimestamp)
	if err != nil {
		return tracercontract.ReserveRequest{}, constant.ErrInvalidRequestBody
	}

	asset, err := decodeAsset(p.Asset)
	if err != nil {
		return tracercontract.ReserveRequest{}, err
	}

	return tracercontract.ReserveRequest{ContractRevision: p.ContractRevision, TransactionID: transaction, RequestID: request, ContextID: p.ContextId, ValidationMode: tracercontract.ValidationMode(p.ValidationMode), TransactionTimestamp: at, LongLived: copyBool(p.LongLived), Amount: tracercontract.Amount(p.Amount), Asset: asset}, nil
}

func decodeEntry(p *reservationv1.ContextEntry) (tracercontract.Entry, error) {
	if unknown(p) {
		return tracercontract.Entry{}, constant.ErrInvalidRequestBody
	}

	var id uuid.UUID

	if p.AccountId != "" {
		parsed, err := uuid.Parse(p.AccountId)
		if err != nil {
			return tracercontract.Entry{}, constant.ErrInvalidRequestBody
		}

		id = parsed
	}

	asset, err := decodeAsset(p.Asset)
	if err != nil {
		return tracercontract.Entry{}, err
	}

	return tracercontract.Entry{AccountID: id, External: p.External, Direction: tracercontract.Direction(p.Direction), Amount: tracercontract.Amount(p.Amount), Asset: asset}, nil
}

func EncodeReserve(ctx context.Context, r tracercontract.ReserveRequest, namespace string, bounds tracercontract.Limits) (*reservationv1.ReserveRequest, error) {
	if err := r.Validate(ctx, namespace, bounds); err != nil {
		return nil, err
	}

	p := &reservationv1.ReserveRequest{
		ContractRevision: r.ContractRevision, TransactionId: r.TransactionID.String(), RequestId: r.RequestID.String(), ContextId: r.ContextID,
		ValidationMode: string(r.ValidationMode), TransactionTimestamp: r.TransactionTimestamp.UTC().Format(time.RFC3339Nano), LongLived: copyBool(r.LongLived), Amount: string(r.Amount), Asset: encodeAsset(r.Asset),
		Context: &reservationv1.EvaluationContext{Accounts: make([]*reservationv1.ContextAccount, 0, len(r.Context.Accounts)), Entries: make([]*reservationv1.ContextEntry, 0, len(r.Context.Entries))},
	}
	for _, account := range r.Context.Accounts {
		p.Context.Accounts = append(p.Context.Accounts, &reservationv1.ContextAccount{Id: account.ID.String(), Type: account.Type, Status: account.Status, Blocked: copyBool(account.Blocked), Asset: encodeAsset(account.Asset)})
	}

	for _, entry := range r.Context.Entries {
		id := ""
		if entry.AccountID != uuid.Nil {
			id = entry.AccountID.String()
		}

		p.Context.Entries = append(p.Context.Entries, &reservationv1.ContextEntry{AccountId: id, External: entry.External, Direction: string(entry.Direction), Amount: string(entry.Amount), Asset: encodeAsset(entry.Asset)})
	}

	return p, nil
}

func DecodeResult(p *reservationv1.ReserveResult, maxReservations int) (*tracercontract.ReserveResult, error) {
	if unknown(p) || unknown(p.GetControls()) || maxReservations <= 0 || len(p.ReservationIds) > maxReservations || len(p.Reasons) > 7 {
		return nil, constant.ErrInvalidRequestBody
	}

	transaction, err := uuid.Parse(p.TransactionId)
	if err != nil {
		return nil, constant.ErrInvalidRequestBody
	}

	evaluation, err := uuid.Parse(p.EvaluationId)
	if err != nil {
		return nil, constant.ErrInvalidRequestBody
	}

	r := &tracercontract.ReserveResult{ContractRevision: p.ContractRevision, TransactionID: transaction, EvaluationID: evaluation, Decision: tracercontract.Decision(p.Decision), Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesControl(p.Controls.Rules), Limits: tracercontract.LimitsControl(p.Controls.Limits)}, ReservationIDs: make([]uuid.UUID, 0, len(p.ReservationIds)), Reasons: make([]tracercontract.ReserveReason, 0, len(p.Reasons))}
	for _, raw := range p.ReservationIds {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, constant.ErrInvalidRequestBody
		}

		r.ReservationIDs = append(r.ReservationIDs, id)
	}

	for _, reason := range p.Reasons {
		r.Reasons = append(r.Reasons, tracercontract.ReserveReason(reason))
	}

	if err := r.Validate(maxReservations); err != nil {
		return nil, err
	}

	return r, nil
}

func EncodeResult(r *tracercontract.ReserveResult, maxReservations int) (*reservationv1.ReserveResult, error) {
	if r == nil {
		return nil, constant.ErrInvalidRequestBody
	}

	if err := r.Validate(maxReservations); err != nil {
		return nil, err
	}

	p := &reservationv1.ReserveResult{ContractRevision: r.ContractRevision, TransactionId: r.TransactionID.String(), EvaluationId: r.EvaluationID.String(), Decision: string(r.Decision), Controls: &reservationv1.ReserveControls{Rules: string(r.Controls.Rules), Limits: string(r.Controls.Limits)}, ReservationIds: make([]string, 0, len(r.ReservationIDs)), Reasons: make([]string, 0, len(r.Reasons))}
	for _, id := range r.ReservationIDs {
		p.ReservationIds = append(p.ReservationIds, id.String())
	}

	for _, reason := range r.Reasons {
		p.Reasons = append(p.Reasons, string(reason))
	}

	return p, nil
}

func unknown(p proto.Message) bool {
	return p == nil || !p.ProtoReflect().IsValid() || len(p.ProtoReflect().GetUnknown()) != 0
}

func copyBool(value *bool) *bool {
	if value == nil {
		return nil
	}

	copied := *value

	return &copied
}

func decodeAsset(p *reservationv1.AssetRef) (tracercontract.AssetRef, error) {
	if unknown(p) {
		return tracercontract.AssetRef{}, constant.ErrInvalidRequestBody
	}

	return tracercontract.AssetRef{Namespace: p.Namespace, ID: p.Id, Code: p.Code}, nil
}

func encodeAsset(a tracercontract.AssetRef) *reservationv1.AssetRef {
	return &reservationv1.AssetRef{Namespace: a.Namespace, Id: a.ID, Code: a.Code}
}
