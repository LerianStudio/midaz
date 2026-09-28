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

// MetadataKeyFeeDeferPair marks both legs of one deferrable fee with the same
// "<feeIndex>:<payerKey>" token, so translation can pair the payer debit with its fee credit.
const MetadataKeyFeeDeferPair = "feeDeferPair"

// MetadataKeyFeeDebtDebtor names, on a fee-account settlement row, the debtor balance
// (alias#key) whose debt that row settled.
const MetadataKeyFeeDebtDebtor = "feeDebtDebtor"

// MetadataKeyFeeDebtID and MetadataKeyFeeDebtSeq name, on the same row, the settled
// debt's id and its seq as a decimal integer string.
const (
	MetadataKeyFeeDebtID  = "feeDebtId"
	MetadataKeyFeeDebtSeq = "feeDebtSeq"
)

// MetadataKeyFeeDebtCollection marks, with the string true, a transaction created by a
// standalone fee-debt collection.
const MetadataKeyFeeDebtCollection = "feeDebtCollection"

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
// collision.
//
// Nine keys qualify. MetadataKeyFeeLeg is the operation-level fee mark. Three are the
// transaction-level statements the fee engine writes about a charge: whether a fee was actually
// charged, which fee package the engine applied, and which exemption it recorded. They are spelled
// literally here because the wire spelling is the contract a client reads, and this refusal has to
// match what the client can send rather than what the engine happens to name its writes. The last
// five are the fee-debt marks: the deferrable pair, the collection marker, and the debtor, id and
// seq of the debt a settlement row settled.
func IsReservedMetadataKey(key string) bool {
	switch key {
	case MetadataKeyFeeLeg, "feeApplied", "packageAppliedID", "feeExemption",
		MetadataKeyFeeDeferPair, MetadataKeyFeeDebtDebtor, MetadataKeyFeeDebtID, MetadataKeyFeeDebtSeq,
		MetadataKeyFeeDebtCollection:
		return true
	default:
		return false
	}
}
