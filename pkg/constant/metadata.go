// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// MetadataKeyFeeLeg is the operation metadata key the ledger reserves for its own fee mark.
// The fee engine writes it on every movement it mints, and the transport layer refuses a caller
// that supplies it, so a client can name a fee movement from the mark alone.
//
// It lives here rather than beside the fee engine because the producer (the fee package) and the
// refusal (the shared body validator) sit in different layers, and one spelling shared by both is
// what stops a rename from leaving the published contract pointing at a key nothing writes.
const MetadataKeyFeeLeg = "feeLeg"

// MetadataValueFeeLeg is the value written under MetadataKeyFeeLeg. It is the string true rather
// than a boolean, matching the transaction-level feeApplied marker, so a client parses one value
// type for both.
const MetadataValueFeeLeg = "true"

// IsReservedMetadataKey reports whether the ledger reserves a metadata key for its own writes.
// A request body naming one is rejected with ErrReservedMetadataKey rather than stripped, because
// a stripped key is indistinguishable on the wire from a key that was stored.
//
// It is a function rather than an exported map on purpose. A package-level map in a published
// module is writable from every package that imports it, inside midaz and downstream, so one
// stray delete would switch the refusal off on every route at once with nothing failing at build
// time, and the published contract would silently stop being true.
//
// The set is deliberately narrow. A key earns a place here only once the ledger publishes it as
// its own word about a record, which is what makes a caller-written copy a lie rather than a
// collision; the transaction-level fee markers are not here, because they predate this rule and
// removing them from an existing caller's body would break a shipped contract.
func IsReservedMetadataKey(key string) bool {
	return key == MetadataKeyFeeLeg
}
