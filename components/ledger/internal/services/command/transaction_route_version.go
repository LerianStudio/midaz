// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

// RouteVersionPolicy states which route version received the request. The use cases are
// transport-agnostic and cannot read the request path, so the version travels from the
// transport shell to every seam that contracts on it as an explicit argument.
//
// It is a named type rather than a bare bool because the seams it gates already take an
// adjacent boolean (honoredTracerSkip) — one more of those would be transposable without
// a compile error.
//
// Create and revert encode the version in the use-case name instead
// (CreateTransactionV1/V2, RevertTransactionV1/V2), so the only readers left are the two
// by-transaction tracer seams commit and cancel drive: ConfirmReservationsByTransaction
// and ReleaseReservationsByTransaction. The policy disappears once commit/cancel are
// split by version too.
type RouteVersionPolicy bool

const (
	// RouteV1 is the /v1 transaction contract, frozen at what it shipped with: no fee
	// engine and no tracer. A client integrated against it must not acquire fee legs,
	// a tenant fee-DB resolution failure, or a reservation rejection from a version
	// upgrade it never asked for.
	RouteV1 RouteVersionPolicy = false

	// RouteV2 is the /v2 transaction contract, which includes both fees and the tracer
	// reservation lifecycle.
	RouteV2 RouteVersionPolicy = true
)
