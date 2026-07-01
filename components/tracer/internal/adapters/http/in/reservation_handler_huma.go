// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// This file migrates the five ReservationHandler operations to Huma, following the
// reference pattern established in rule_handler_huma.go (read that file's header for
// the full rationale). The conventions carried verbatim:
//
//   - Reserve: the body is taken as RawBody []byte + SkipValidateBody:true so
//     malformed JSON and NormalizeAndReserveValidate failures flow through the
//     core's imperative json.Unmarshal + Validate and produce the canonical Midaz
//     error — never a native Huma 422. The payload-size guard (>100KB) also stays
//     imperative in the core (Huma has no Fiber-style body limit).
//   - The four lifecycle ops (confirm/release by id, confirm/release by
//     transaction) carry a single path param with NO format tag — uuid.Parse in the
//     shared core is the sole validator, yielding the canonical 400/0065 rather than
//     a native 422. The by-transaction ops key on transaction_id (not id).
//   - Every handler func delegates to the transport-agnostic core on
//     *ReservationHandler (reserve / terminate / terminateByTransaction); the cores
//     own the span, imperative validation, the service call, the success log, and
//     canonicalize every error. Errors flow through the package-level humaProblem
//     (defined in rule_handler_huma.go) — reused verbatim, NOT redefined here.
//   - Two cores, four lifecycle shells: terminate powers ConfirmHuma (StatusConfirmed
//     + service.Confirm) and ReleaseHuma (StatusReleased + service.Release);
//     terminateByTransaction powers ConfirmByTransactionHuma (StatusConfirmed +
//     service.ConfirmByTransaction) and ReleaseByTransactionHuma (StatusReleased +
//     service.ReleaseByTransaction). Each shell is explicit so the terminal status
//     and service method are pinned per route.

// ReserveInputHuma is the Huma request envelope for POST /v1/reservations. The body
// is taken raw (see file header) so the core's imperative json.Unmarshal +
// NormalizeAndReserveValidate remain the sole validators.
type ReserveInputHuma struct {
	RawBody []byte `contentType:"application/json"`
}

// ReserveOutputHuma is the Huma response envelope for POST /v1/reservations. Body is
// the ReserveResponse serialized verbatim; Status pins 201 to match pkgHTTP.Created.
type ReserveOutputHuma struct {
	Status int
	Body   *ReserveResponse
}

// ReservationIDInputHuma is the Huma request envelope for the by-id lifecycle ops
// (confirm/release). The path param carries NO format tag: uuid.Parse in the shared
// terminate core is the sole validator (canonical 400/0065, never a native 422).
type ReservationIDInputHuma struct {
	ID string `path:"id" doc:"Reservation ID (UUID)"`
}

// ReservationActionOutputHuma is the 200 response envelope for the by-id lifecycle
// ops. Body is the ReservationActionResponse serialized verbatim.
type ReservationActionOutputHuma struct {
	Status int
	Body   *ReservationActionResponse
}

// TransactionIDInputHuma is the Huma request envelope for the by-transaction
// lifecycle ops (confirm/release). The path param is transaction_id (NOT id) and
// carries NO format tag: uuid.Parse in the shared terminateByTransaction core is the
// sole validator (canonical 400/0065, never a native 422).
type TransactionIDInputHuma struct {
	TransactionID string `path:"transaction_id" doc:"Transaction ID (UUID)"`
}

// TransactionActionOutputHuma is the 200 response envelope for the by-transaction
// lifecycle ops. Body is the TransactionActionResponse serialized verbatim.
type TransactionActionOutputHuma struct {
	Status int
	Body   *TransactionActionResponse
}

// ReserveHuma is the Huma handler for POST /v1/reservations. It delegates to the
// shared core and, on success, returns 201 with the reservation handle.
func (h *ReservationHandler) ReserveHuma(ctx context.Context, in *ReserveInputHuma) (*ReserveOutputHuma, error) {
	result, err := h.reserve(ctx, in.RawBody)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &ReserveOutputHuma{Status: http.StatusCreated, Body: result}, nil
}

// ConfirmHuma is the Huma handler for POST /v1/reservations/{id}/confirm. It
// delegates to the shared terminate core with the CONFIRMED terminal status and
// service.Confirm.
func (h *ReservationHandler) ConfirmHuma(ctx context.Context, in *ReservationIDInputHuma) (*ReservationActionOutputHuma, error) {
	result, err := h.terminate(ctx, in.ID, "handler.reservations.confirm", string(model.StatusConfirmed), h.service.Confirm)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &ReservationActionOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// ReleaseHuma is the Huma handler for POST /v1/reservations/{id}/release. It
// delegates to the shared terminate core with the RELEASED terminal status and
// service.Release.
func (h *ReservationHandler) ReleaseHuma(ctx context.Context, in *ReservationIDInputHuma) (*ReservationActionOutputHuma, error) {
	result, err := h.terminate(ctx, in.ID, "handler.reservations.release", string(model.StatusReleased), h.service.Release)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &ReservationActionOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// ConfirmByTransactionHuma is the Huma handler for
// POST /v1/reservations/transaction/{transaction_id}/confirm. It delegates to the
// shared terminateByTransaction core with the CONFIRMED terminal status and
// service.ConfirmByTransaction.
func (h *ReservationHandler) ConfirmByTransactionHuma(ctx context.Context, in *TransactionIDInputHuma) (*TransactionActionOutputHuma, error) {
	result, err := h.terminateByTransaction(ctx, in.TransactionID, "handler.reservations.confirm_by_transaction", string(model.StatusConfirmed), h.service.ConfirmByTransaction)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &TransactionActionOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// ReleaseByTransactionHuma is the Huma handler for
// POST /v1/reservations/transaction/{transaction_id}/release. It delegates to the
// shared terminateByTransaction core with the RELEASED terminal status and
// service.ReleaseByTransaction.
func (h *ReservationHandler) ReleaseByTransactionHuma(ctx context.Context, in *TransactionIDInputHuma) (*TransactionActionOutputHuma, error) {
	result, err := h.terminateByTransaction(ctx, in.TransactionID, "handler.reservations.release_by_transaction", string(model.StatusReleased), h.service.ReleaseByTransaction)
	if err != nil {
		return nil, humaProblem(err)
	}

	return &TransactionActionOutputHuma{Status: http.StatusOK, Body: result}, nil
}

// RegisterReservationRoutes registers the migrated reservation operations on the
// shared Huma API. It is the per-file seam NewRoutes calls; the auth + reservation
// tenant middleware for these routes is attached in routes.go (Fiber-level), not
// here. Paths are GROUP-RELATIVE to the /v1 Fiber group.
func RegisterReservationRoutes(api huma.API, h *ReservationHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "createReservation",
		Method:      http.MethodPost,
		Path:        "/reservations",
		Summary:     "Reserve transaction capacity (phase one)",
		Tags:        []string{"Reservations"},
		// SkipValidateBody: the body is taken as RawBody and validated imperatively
		// by NormalizeAndReserveValidate inside the core, which produces the
		// canonical Midaz error codes. Without this, Huma validates the JSON body
		// against the RawBody schema and rejects it with a native 422 before the
		// handler runs.
		SkipValidateBody: true,
	}, h.ReserveHuma)

	huma.Register(api, huma.Operation{
		OperationID: "confirmReservation",
		Method:      http.MethodPost,
		Path:        "/reservations/{id}/confirm",
		Summary:     "Confirm a reservation (phase two — commit)",
		Tags:        []string{"Reservations"},
	}, h.ConfirmHuma)

	huma.Register(api, huma.Operation{
		OperationID: "releaseReservation",
		Method:      http.MethodPost,
		Path:        "/reservations/{id}/release",
		Summary:     "Release a reservation (phase two — abort)",
		Tags:        []string{"Reservations"},
	}, h.ReleaseHuma)

	huma.Register(api, huma.Operation{
		OperationID: "confirmReservationByTransaction",
		Method:      http.MethodPost,
		Path:        "/reservations/transaction/{transaction_id}/confirm",
		Summary:     "Confirm a transaction's reservations (phase two — commit by transaction)",
		Tags:        []string{"Reservations"},
	}, h.ConfirmByTransactionHuma)

	huma.Register(api, huma.Operation{
		OperationID: "releaseReservationByTransaction",
		Method:      http.MethodPost,
		Path:        "/reservations/transaction/{transaction_id}/release",
		Summary:     "Release a transaction's reservations (phase two — abort by transaction)",
		Tags:        []string{"Reservations"},
	}, h.ReleaseByTransactionHuma)
}
