// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
)

// ReserveContractRevision identifies the replacement content contract, not a
// new route. Receivers must reject missing or unsupported revisions.
const ReserveContractRevision = "context-reserve-1"

// ValidationMode declares requested controls, as selected by trusted producer
// settings. Omission never silently reduces validation to limits alone.
type ValidationMode string

const (
	ValidationLimits         ValidationMode = "limits"
	ValidationRulesAndLimits ValidationMode = "rules-and-limits"
)

// ReserveRequest carries the frozen facts of one operation. Transport adapters
// must reject unknown/duplicate fields and invalid text before decoding, enforce
// a body-size bound, and preserve optional boolean presence. JSON unmarshalling
// alone does not provide those guarantees; DecodeReserveJSON enforces the wire
// shape before this type reaches admission. Protobuf adapters use this same type
// after translation; neither transport owns separate validation semantics.
type ReserveRequest struct {
	ContractRevision     string         `json:"contractRevision"`
	TransactionID        uuid.UUID      `json:"transactionId"`
	RequestID            uuid.UUID      `json:"requestId"`
	ContextID            string         `json:"contextId"`
	ValidationMode       ValidationMode `json:"validationMode"`
	TransactionTimestamp time.Time      `json:"transactionTimestamp"`
	LongLived            *bool          `json:"longLived"`
	Amount               Amount         `json:"amount"`
	Asset                AssetRef       `json:"asset"`
	Context              Context        `json:"context"`
}

// ReserveScope is trusted input supplied after authentication and tenant
// resolution. It must never be populated from the request body or unverified
// headers. SingleTenant explicitly permits an empty tenant identity; the zero
// value cannot accidentally disable tenant isolation.
type ReserveScope struct {
	TenantID       string
	IntegrationID  string
	AssetNamespace string
	SingleTenant   bool
}

const maxReserveIdentityBytes = 256

func (s ReserveScope) validate(limits Limits) error {
	if (s.TenantID != "" || !s.SingleTenant) && !validText(s.TenantID, maxReserveIdentityBytes) {
		return invalid("resolved tenant")
	}

	if !validText(s.IntegrationID, maxReserveIdentityBytes) || !validText(s.AssetNamespace, limits.MaxTextBytes) {
		return invalid("authenticated integration")
	}

	return nil
}

// Validate checks completeness and bounded structure without mutating facts or
// applying business policies. Timestamp freshness belongs only to first-time
// evaluation: a stored decision remains replayable outside that time window.
// Amount is the declared principal, not gross consumption across accounts/assets.
func (r ReserveRequest) Validate(ctx context.Context, authorizedNamespace string, limits Limits) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := validateReserveLimits(limits); err != nil {
		return err
	}

	if r.ContractRevision != ReserveContractRevision || r.TransactionID == uuid.Nil || r.RequestID == uuid.Nil {
		return invalid("reserve identity or revision")
	}

	if !validText(r.ContextID, maxReserveIdentityBytes) || r.LongLived == nil || r.Context.Accounts == nil {
		return invalid("reserve presence")
	}

	if r.ValidationMode != ValidationLimits && r.ValidationMode != ValidationRulesAndLimits {
		return invalid("validation mode")
	}

	stamp := r.TransactionTimestamp.UTC()
	if stamp.IsZero() || stamp.Year() < 1 || stamp.Year() > 9999 {
		return invalid("transaction timestamp")
	}

	amount, err := r.Amount.Decimal(ctx, limits)
	if err != nil {
		return err
	}

	if !amount.IsPositive() {
		return invalid("positive principal amount required")
	}

	assets := make(map[AssetIdentity]string)
	if err := validateAsset(r.Asset, authorizedNamespace, limits, assets); err != nil {
		return err
	}

	return r.Context.validate(ctx, authorizedNamespace, limits, assets)
}

// The canonical format uses uint32 lengths. Reject configurations that could
// overflow framing before allocating maps or parsing decimal coefficients.
func validateReserveLimits(limits Limits) error {
	if err := limits.Validate(); err != nil {
		return err
	}

	if int64(limits.MaxAccounts) > math.MaxUint32 || int64(limits.MaxEntries) > math.MaxUint32 ||
		int64(limits.MaxTextBytes) > math.MaxUint32 || int64(limits.MaxIntegerDigits) > math.MaxUint32-2 ||
		int64(limits.MaxFractionDigits) > math.MaxUint32-2-int64(limits.MaxIntegerDigits) {
		return invalid("reserve framing limits")
	}

	return nil
}
