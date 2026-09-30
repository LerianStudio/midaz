// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package reservationmap maps the reservation wire contract onto the tracer's
// domain model. It sits outside the tracer's internal tree so that the ledger's
// cross-component contract test runs the same mapping the tracer's reservation
// server runs, and it is kept apart from pkg/model so the domain model stays
// free of any transport dependency.
package reservationmap

import (
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// ValidationRequestFromReserveProto builds the model.ValidationRequest the
// reserve path validates, from the proto request: requestId, amount
// (decimal-as-string), asset, account id and type, optional
// segment/portfolio/merchant ids, transactionType, transactionTimestamp
// (RFC3339, fractional seconds accepted) and flat metadata. A parse failure
// returns the validation sentinel for the offending field. An empty timestamp
// maps to the zero time and an empty account id to the nil account, leaving the
// decision to the reserve validation rules
// (model.ValidationRequest.NormalizeAndValidateForReserve).
func ValidationRequestFromReserveProto(req *reservationv1.ReserveRequest) (*model.ValidationRequest, error) {
	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return nil, constant.ErrValidationRequestIDRequired
	}

	amount, err := safe.ParseDecimal(req.GetAmount())
	if err != nil {
		return nil, constant.ErrValidationAmountNonPositive
	}

	var transactionTimestamp time.Time
	if ts := req.GetTransactionTimestamp(); ts != "" {
		transactionTimestamp, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, constant.ErrValidationTimestampRequired
		}
	}

	var accountID uuid.UUID
	if acc := req.GetAccount(); acc != nil && acc.GetAccountId() != "" {
		accountID, err = uuid.Parse(acc.GetAccountId())
		if err != nil {
			return nil, constant.ErrInvalidPathParameter
		}
	}

	validationReq := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionType(req.GetTransactionType()),
		Amount:               amount,
		Asset:                req.GetAsset(),
		TransactionTimestamp: transactionTimestamp,
		Account:              model.AccountContext{ID: accountID, Type: req.GetAccount().GetType()},
		Metadata:             metadataFromProto(req.GetMetadata()),
	}

	if segment, err := optionalContextID(req.GetSegmentId()); err != nil {
		return nil, err
	} else if segment != nil {
		validationReq.Segment = &model.SegmentContext{ID: *segment}
	}

	if portfolio, err := optionalContextID(req.GetPortfolioId()); err != nil {
		return nil, err
	} else if portfolio != nil {
		validationReq.Portfolio = &model.PortfolioContext{ID: *portfolio}
	}

	if merchant, err := optionalContextID(req.GetMerchantId()); err != nil {
		return nil, err
	} else if merchant != nil {
		validationReq.Merchant = &model.MerchantContext{ID: *merchant}
	}

	return validationReq, nil
}

// optionalContextID parses an optional uuid-bearing context id (segment /
// portfolio / merchant). An empty string means the field is absent (nil);
// a present-but-malformed value is rejected.
func optionalContextID(raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, constant.ErrInvalidPathParameter
	}

	return &id, nil
}

// metadataFromProto widens the proto string map into the model's metadata map.
// An empty map yields nil so an absent field and an empty one validate alike.
func metadataFromProto(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}

	return out
}
