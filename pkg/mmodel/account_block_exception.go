// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import "time"

// AccountBlockExceptionMaxBatchSize is the largest number of exceptions one
// request may create. The cap bounds the alias lookup (a single batched SELECT)
// and the Redis pipeline behind the route; a larger batch is rejected rather
// than truncated so the caller never believes it minted more grants than it did.
const AccountBlockExceptionMaxBatchSize = 100

// AccountBlockExceptionDefaultTTLSeconds is the lifetime applied to an exception
// whose request omits ttl. An exception is a single-use authorization to
// transact on a blocked account, so the default is deliberately short.
const AccountBlockExceptionDefaultTTLSeconds = 300

// AccountBlockExceptionMaxTTLSeconds is the largest ttl a request may ask for.
// An exception bypasses the account block, so an unbounded lifetime would let a
// caller mint a permanent bypass; the ceiling matches the 24h horizon of the
// balance cache entries the grant is consumed alongside.
const AccountBlockExceptionMaxTTLSeconds = 86400

// CreateAccountBlockExceptionsInput is the request body of the account
// block-exception create route: a batch of exceptions to mint in one call.
type CreateAccountBlockExceptionsInput struct {
	// The exceptions to create. At least one, at most
	// AccountBlockExceptionMaxBatchSize.
	Exceptions []CreateAccountBlockExceptionInput `json:"exceptions" validate:"required,min=1,dive" minItems:"1" maxItems:"100" doc:"Exceptions to create (1-100 per request)"`
}

// CreateAccountBlockExceptionInput is one exception in the create batch. It
// authorizes exactly one debit of Amount out of AccountAlias while that
// account is blocked, and dies on first use or when its TTL elapses.
type CreateAccountBlockExceptionInput struct {
	// Alias of the SOURCE account the exception authorizes. Must exist in the
	// organization and ledger named in the path.
	AccountAlias string `json:"accountAlias" validate:"required,max=100" example:"@fraud_account" doc:"Alias of the source account the exception authorizes"`
	// Exact amount of the authorized debit, as a positive decimal string.
	Amount string `json:"amount" validate:"required" example:"150.00" doc:"Exact amount of the authorized debit, as a positive decimal string"`
	// Lifetime in seconds. Optional; defaults to
	// AccountBlockExceptionDefaultTTLSeconds, capped at
	// AccountBlockExceptionMaxTTLSeconds.
	TTL *int `json:"ttl,omitempty" example:"300" doc:"Lifetime in seconds (1-86400, default 300)"`
}

// AccountBlockException is one minted exception as the create route returns it.
// The identifier is the authorization: it is presented in a transaction body and
// consumed atomically on use.
type AccountBlockException struct {
	// Single-use identifier presented in a transaction body.
	AccountBlockExceptionID string `json:"accountBlockExceptionId" example:"018f2c1e-6a3b-7c4d-8e5f-0a1b2c3d4e5f" doc:"Single-use exception identifier"`
	// Alias of the source account the exception authorizes, echoed from the request.
	AccountAlias string `json:"accountAlias" example:"@fraud_account" doc:"Alias of the source account the exception authorizes"`
	// Authorized amount, echoed verbatim from the request.
	Amount string `json:"amount" example:"150.00" doc:"Authorized amount, echoed from the request"`
	// Instant the exception expires, derived from the applied TTL.
	ExpiresAt time.Time `json:"expiresAt" format:"date-time" doc:"Instant the exception expires, derived from the applied TTL"`
}

// AccountBlockExceptions is the create route's response envelope: one entry per
// requested exception, in request order.
type AccountBlockExceptions struct {
	// The created exceptions, in request order.
	Exceptions []AccountBlockException `json:"exceptions" doc:"Created exceptions, in request order"`
}

// AccountBlockExceptionRedis is the cached representation of an exception — the
// ONLY place an exception is stored. Field names are CamelCase to match the
// casing contract of the balance blobs the transaction Lua script already reads
// and writes with cjson, so the same script can decode a grant without a second
// convention.
//
// Amount holds the CANONICAL decimal string (shopspring decimal.String()), which
// is the form the transaction script receives its operation amounts in, so the
// consumption check is a plain string comparison.
type AccountBlockExceptionRedis struct {
	// Alias of the source account the exception authorizes.
	Alias string `json:"Alias"`
	// Canonical decimal string of the authorized amount.
	Amount string `json:"Amount"`
}
