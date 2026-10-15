// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"context"
	"strings"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/LerianStudio/lib-observability/v4/log"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel"

	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// The transaction create and revert pipelines call ValidateSendSourceAndDistribute
// twice over the same send: once on the raw input and once after the legs are
// normalized. These tests run both calls over one corpus and pin the relation between
// their outcomes: the two calls answer the same code for every send.
//
// Values are untouched by normalization — it only sets IsFrom on the sources, defaults
// the balance keys, and rewrites AccountAlias into the composite
// "index#alias#balanceKey" entry key — so the totals check answers identically on both
// sides of it. The ambiguity check compares Responses.Sources against
// Responses.Destinations, and CalculateTotal builds both lists in the
// "alias#balanceKey" form from each leg's bare alias and balance key, whichever alias
// form the leg carries. The check therefore sees the same account identities on the
// raw and on the normalized call, and answers ErrTransactionAmbiguous on both, before
// the remainder and totals rules run.
//
// The pipelines still validate twice for a reason this file does not own: the raw pass
// refuses malformed input — totals, remainders, ambiguity — before the idempotency
// claim, the ledger settings read and the fee engine, and the second pass validates
// the normalized, post-fee send. The equality pinned here is what makes the first pass
// answer the code the caller ends up with.

// differentialContext carries the logger and tracer ValidateSendSourceAndDistribute
// reads off the context.
func differentialContext() context.Context {
	ctx := libObservability.ContextWithLogger(context.Background(), &log.GoLogger{Level: log.LevelInfo})

	return libObservability.ContextWithTracer(ctx, otel.Tracer("test"))
}

// cloneSendLegs deep-copies the source and destination legs, including the Amount and
// Share pointers CalculateTotal rebinds, so the raw and the normalized run never
// observe each other's writes.
func cloneSendLegs(tx Transaction) Transaction {
	clone := tx
	clone.Send.Source.From = cloneLegs(tx.Send.Source.From)
	clone.Send.Distribute.To = cloneLegs(tx.Send.Distribute.To)

	return clone
}

func cloneLegs(entries []FromTo) []FromTo {
	if entries == nil {
		return nil
	}

	out := make([]FromTo, len(entries))

	for i, entry := range entries {
		out[i] = entry

		if entry.Amount != nil {
			amount := *entry.Amount
			out[i].Amount = &amount
		}

		if entry.Share != nil {
			share := *entry.Share
			out[i].Share = &share
		}
	}

	return out
}

// normalizeSendLegsLikeCreate applies, in the same order, the three mutations the
// create and revert pipelines run between their two validations: IsFrom on every
// source, the default balance key on both sides, and the composite alias on both
// sides.
func normalizeSendLegsLikeCreate(tx *Transaction) {
	for i := range tx.Send.Source.From {
		tx.Send.Source.From[i].IsFrom = true
	}

	ApplyDefaultBalanceKeys(tx.Send.Source.From)
	ApplyDefaultBalanceKeys(tx.Send.Distribute.To)

	MutateConcatAliases(tx.Send.Source.From)
	MutateConcatAliases(tx.Send.Distribute.To)
}

// leg builds a source or destination entry with an explicit amount.
func leg(alias, balanceKey, asset string, value int64) FromTo {
	return FromTo{
		AccountAlias: alias,
		BalanceKey:   balanceKey,
		Amount:       &Amount{Asset: asset, Value: decimal.NewFromInt(value)},
	}
}

// shareLeg builds an entry that resolves its value from a percentage of the send.
func shareLeg(alias string, percentage, percentageOfPercentage int64) FromTo {
	return FromTo{
		AccountAlias: alias,
		Share:        &Share{Percentage: percentage, PercentageOfPercentage: percentageOfPercentage},
	}
}

// remainingLeg builds an entry that absorbs whatever the other entries left.
func remainingLeg(alias string) FromTo {
	return FromTo{AccountAlias: alias, Remaining: "remaining"}
}

func sendOf(asset string, value int64, from, to []FromTo) Transaction {
	return Transaction{
		Send: Send{
			Asset:      asset,
			Value:      decimal.NewFromInt(value),
			Source:     Source{From: from},
			Distribute: Distribute{To: to},
		},
	}
}

// differentialCase is one input run through both validations.
type differentialCase struct {
	name   string
	status string
	build  func() Transaction
}

func differentialCorpus() []differentialCase {
	return []differentialCase{
		{
			name:   "balanced transfer",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 100)}, []FromTo{leg("@destination", "", "USD", 100)})
			},
		},
		{
			name:   "balanced transfer pending",
			status: pkgConstant.PENDING,
			build: func() Transaction {
				tx := sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 100)}, []FromTo{leg("@destination", "", "USD", 100)})
				tx.Pending = true

				return tx
			},
		},
		{
			name:   "balanced transfer noted",
			status: pkgConstant.NOTED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 100)}, []FromTo{leg("@destination", "", "USD", 100)})
			},
		},
		{
			name:   "balanced transfer approved",
			status: pkgConstant.APPROVED,
			build: func() Transaction {
				tx := sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 100)}, []FromTo{leg("@destination", "", "USD", 100)})
				tx.Pending = true

				return tx
			},
		},
		{
			name:   "sources below the send value",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 60)}, []FromTo{leg("@destination", "", "USD", 100)})
			},
		},
		{
			name:   "destinations above the send value",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 100)}, []FromTo{leg("@destination", "", "USD", 140)})
			},
		},
		{
			name:   "both sides agree but neither matches the send value",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 70)}, []FromTo{leg("@destination", "", "USD", 70)})
			},
		},
		{
			name:   "explicit balance keys on both sides",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@source", "savings", "USD", 100)}, []FromTo{leg("@destination", "wallet", "USD", 100)})
			},
		},
		{
			name:   "alias carrying the separator with an explicit balance key",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@source#savings", "savings", "USD", 100)}, []FromTo{leg("@destination", "", "USD", 100)})
			},
		},
		{
			name:   "share split across two destinations",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{leg("@source", "", "USD", 100)},
					[]FromTo{shareLeg("@destination1", 40, 0), shareLeg("@destination2", 60, 0)})
			},
		},
		{
			name:   "share of a share",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{leg("@source", "", "USD", 100)},
					[]FromTo{shareLeg("@destination1", 50, 50), shareLeg("@destination2", 75, 0)})
			},
		},
		{
			name:   "amount plus remaining on the destinations",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{leg("@source", "", "USD", 100)},
					[]FromTo{leg("@destination1", "", "USD", 30), remainingLeg("@destination2")})
			},
		},
		{
			name:   "remaining on the sources",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{leg("@source1", "", "USD", 25), remainingLeg("@source2")},
					[]FromTo{leg("@destination", "", "USD", 100)})
			},
		},
		{
			name:   "share plus remaining with a mismatching total",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{leg("@source", "", "USD", 90)},
					[]FromTo{shareLeg("@destination1", 40, 0), remainingLeg("@destination2")})
			},
		},
		{
			name:   "repeated alias on the same side",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{leg("@source", "", "USD", 40), leg("@source", "", "USD", 60)},
					[]FromTo{leg("@destination", "", "USD", 100)})
			},
		},
		{
			name:   "same alias on both sides at the same index",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@account", "", "USD", 100)}, []FromTo{leg("@account", "", "USD", 100)})
			},
		},
		{
			name:   "same alias on both sides at the same index with a mismatching total",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@account", "", "USD", 100)}, []FromTo{leg("@account", "", "USD", 64)})
			},
		},
		{
			name:   "same alias on both sides pending",
			status: pkgConstant.PENDING,
			build: func() Transaction {
				tx := sendOf("USD", 100, []FromTo{leg("@account", "", "USD", 100)}, []FromTo{leg("@account", "", "USD", 100)})
				tx.Pending = true

				return tx
			},
		},
		{
			name:   "same alias on both sides with different balance keys",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@account", "savings", "USD", 100)}, []FromTo{leg("@account", "wallet", "USD", 100)})
			},
		},
		{
			name:   "same alias on both sides at different indexes",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{leg("@other", "", "USD", 40), leg("@account", "", "USD", 60)},
					[]FromTo{leg("@account", "", "USD", 100)})
			},
		},
		{
			name:   "empty legs on both sides",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 0, []FromTo{}, []FromTo{})
			},
		},
		{
			name:   "v1 json canonical shape",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				tx := sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 100)}, []FromTo{leg("@destination", "", "USD", 100)})
				tx.Send.Source.From[0].IsFrom = true

				return tx
			},
		},
		{
			name:   "v1 json canonical shape with a mismatching total",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				tx := sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 55)}, []FromTo{leg("@destination", "", "USD", 55)})
				tx.Send.Source.From[0].IsFrom = true

				return tx
			},
		},
		{
			name:   "v1 json canonical shape with one alias on both sides",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				tx := sendOf("USD", 100, []FromTo{leg("@account", "", "USD", 100)}, []FromTo{leg("@account", "", "USD", 100)})
				tx.Send.Source.From[0].IsFrom = true

				return tx
			},
		},
		{
			name:   "v1 inflow canonical shape",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{{IsFrom: true, AccountAlias: pkgConstant.DefaultExternalAccountAliasPrefix + "USD", Amount: &Amount{Asset: "USD", Value: decimal.NewFromInt(100)}}},
					[]FromTo{leg("@destination", "", "USD", 100)})
			},
		},
		{
			name:   "v1 outflow canonical shape",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				tx := sendOf("USD", 100,
					[]FromTo{leg("@source", "", "USD", 100)},
					[]FromTo{{AccountAlias: pkgConstant.DefaultExternalAccountAliasPrefix + "USD", Amount: &Amount{Asset: "USD", Value: decimal.NewFromInt(100)}}})
				tx.Send.Source.From[0].IsFrom = true

				return tx
			},
		},
		{
			name:   "v1 legs without IsFrom",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				// The shape a v1 body decodes into before BuildTransaction stamps IsFrom:
				// the flag is not on the wire, so the raw call sees it unset on every
				// source.
				return sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 100)}, []FromTo{leg("@destination", "", "USD", 100)})
			},
		},
		{
			name:   "v1 legs without IsFrom and a mismatching total",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@source", "", "USD", 100)}, []FromTo{leg("@destination", "", "USD", 80)})
			},
		},
	}
}

// TestValidateSendSourceAndDistribute_RawAndNormalizedAgree runs every corpus entry
// through both validations and requires the same code from each.
func TestValidateSendSourceAndDistribute_RawAndNormalizedAgree(t *testing.T) {
	t.Parallel()

	ctx := differentialContext()

	for _, tc := range differentialCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rawCode, normalizedCode := runBothValidations(ctx, tc)

			if rawCode != normalizedCode {
				t.Fatalf("raw and normalized returned different codes: raw=%q normalized=%q", rawCode, normalizedCode)
			}
		})
	}
}

// TestValidateSendSourceAndDistribute_NormalizedNeverAcceptsWhatRawRejects asserts the
// direction that governs whether the raw call can be dropped: every rejection survives
// normalization and keeps its code.
func TestValidateSendSourceAndDistribute_NormalizedNeverAcceptsWhatRawRejects(t *testing.T) {
	t.Parallel()

	ctx := differentialContext()

	for _, tc := range differentialCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rawCode, normalizedCode := runBothValidations(ctx, tc)
			if rawCode == "" {
				return
			}

			if normalizedCode == "" {
				t.Fatalf("raw validation rejected with %q and the normalized validation accepted the same input", rawCode)
			}

			if normalizedCode != rawCode {
				t.Fatalf("normalized validation answered %q for an input the raw validation rejected with %q", normalizedCode, rawCode)
			}
		})
	}
}

// runBothValidations validates the case's input raw and normalized, over independent
// copies, and returns the two business error codes ("" for an accepted input).
func runBothValidations(ctx context.Context, tc differentialCase) (rawCode, normalizedCode string) {
	base := tc.build()

	rawInput := cloneSendLegs(base)
	_, rawErr := ValidateSendSourceAndDistribute(ctx, rawInput, tc.status)

	normalizedInput := cloneSendLegs(base)
	normalizeSendLegsLikeCreate(&normalizedInput)
	_, normalizedErr := ValidateSendSourceAndDistribute(ctx, normalizedInput, tc.status)

	return codeFromError(rawErr), codeFromError(normalizedErr)
}

// FuzzValidateSendRawVsNormalized generates sends over arbitrary aliases, balance keys,
// values and value expressions and holds both validations to the relation the corpus
// pins: the raw and the normalized call answer the same code, accepted or rejected.
func FuzzValidateSendRawVsNormalized(f *testing.F) {
	f.Add(int64(100), "@source", "@destination", "", int64(100), int64(100), uint8(0), uint8(0))
	f.Add(int64(100), "@source", "@destination", "savings", int64(60), int64(100), uint8(0), uint8(1))
	f.Add(int64(100), "@account", "@account", "", int64(100), int64(100), uint8(0), uint8(0))
	f.Add(int64(100), "@a", "@a", "", int64(100), int64(64), uint8(0), uint8(0))
	f.Add(int64(100), "@source", "@destination", "", int64(0), int64(100), uint8(1), uint8(2))
	f.Add(int64(100), "@source", "@destination", "", int64(0), int64(100), uint8(2), uint8(3))
	f.Add(int64(0), "", "", "", int64(0), int64(0), uint8(0), uint8(0))
	f.Add(int64(100), "@source#savings", "@destination", "wallet", int64(100), int64(100), uint8(0), uint8(0))

	statuses := []string{pkgConstant.CREATED, pkgConstant.PENDING, pkgConstant.NOTED, pkgConstant.APPROVED, pkgConstant.CANCELED}

	// maxStringLen bounds fuzzer-generated strings to prevent resource exhaustion.
	const maxStringLen = 48

	ctx := differentialContext()

	f.Fuzz(func(t *testing.T, sendValue int64, sourceAlias, destinationAlias, balanceKey string,
		sourceValue, destinationValue int64, expression, statusSelector uint8,
	) {
		if len(sourceAlias) > maxStringLen {
			sourceAlias = sourceAlias[:maxStringLen]
		}

		if len(destinationAlias) > maxStringLen {
			destinationAlias = destinationAlias[:maxStringLen]
		}

		// A client alias never carries the separator (the accepted alias characters
		// exclude it), and a bare alias that does is read by the raw call as if it were
		// an entry key, which is a property of the alias form rather than of the
		// relation under test. Keep the generated aliases inside the client domain.
		sourceAlias = strings.ReplaceAll(sourceAlias, AliasSeparatorString, "")
		destinationAlias = strings.ReplaceAll(destinationAlias, AliasSeparatorString, "")

		if len(balanceKey) > maxStringLen {
			balanceKey = balanceKey[:maxStringLen]
		}

		status := statuses[int(statusSelector)%len(statuses)]

		source := leg(sourceAlias, balanceKey, "USD", sourceValue)

		switch expression % 3 {
		case 1:
			source = shareLeg(sourceAlias, sourceValue, 0)
			source.BalanceKey = balanceKey
		case 2:
			source = remainingLeg(sourceAlias)
			source.BalanceKey = balanceKey
		}

		base := sendOf("USD", sendValue, []FromTo{source}, []FromTo{leg(destinationAlias, "", "USD", destinationValue)})
		base.Pending = status == pkgConstant.PENDING || status == pkgConstant.APPROVED

		rawInput := cloneSendLegs(base)
		_, rawErr := ValidateSendSourceAndDistribute(ctx, rawInput, status)

		normalizedInput := cloneSendLegs(base)
		normalizeSendLegsLikeCreate(&normalizedInput)
		_, normalizedErr := ValidateSendSourceAndDistribute(ctx, normalizedInput, status)

		rawCode := codeFromError(rawErr)
		normalizedCode := codeFromError(normalizedErr)

		if normalizedCode != rawCode {
			t.Fatalf("raw answered %q and normalized answered %q: sendValue=%d source=%q destination=%q balanceKey=%q expression=%d status=%s",
				rawCode, normalizedCode, sendValue, sourceAlias, destinationAlias, balanceKey, expression%3, status)
		}
	})
}

// TestValidateSendSourceAndDistribute_RemainingResolvesSameOnBothPasses runs the raw and
// the normalized validation over ONE send, without the clone the corpus tests use,
// because that is how the pipelines call it: the raw call writes each resolved
// remainder into its leg's Amount, and the normalized call must still resolve the
// leg to the remainder instead of counting that written Amount a second time.
func TestValidateSendSourceAndDistribute_RemainingResolvesSameOnBothPasses(t *testing.T) {
	t.Parallel()

	ctx := differentialContext()

	tests := []struct {
		name          string
		send          func() Transaction
		fromSide      bool
		rawKey        string
		normalizedKey string
		want          int64
	}{
		{
			name: "source remainder after an explicit amount",
			send: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{leg("@srcA", "", "USD", 60), remainingLeg("@srcB")},
					[]FromTo{leg("@dst", "", "USD", 100)})
			},
			fromSide:      true,
			rawKey:        "@srcB",
			normalizedKey: "1#@srcB#default",
			want:          40,
		},
		{
			name: "destination remainder after an explicit amount",
			send: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{leg("@src", "", "USD", 100)},
					[]FromTo{leg("@dstA", "", "USD", 70), remainingLeg("@dstB")})
			},
			rawKey:        "@dstB",
			normalizedKey: "1#@dstB#default",
			want:          30,
		},
		{
			name: "destination remainder after a share",
			send: func() Transaction {
				return sendOf("USD", 100,
					[]FromTo{leg("@src", "", "USD", 100)},
					[]FromTo{shareLeg("@dstA", 40, 0), remainingLeg("@dstB")})
			},
			rawKey:        "@dstB",
			normalizedKey: "1#@dstB#default",
			want:          60,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			send := tc.send()
			want := decimal.NewFromInt(tc.want)

			resolved := func(r *Responses) map[string]Amount {
				if tc.fromSide {
					return r.From
				}

				return r.To
			}

			raw, err := ValidateSendSourceAndDistribute(ctx, send, pkgConstant.CREATED)
			if err != nil {
				t.Fatalf("raw validation: unexpected error %v", err)
			}

			if got := resolved(raw)[tc.rawKey].Value; !got.Equal(want) {
				t.Fatalf("raw validation resolved %s to %s, want %s", tc.rawKey, got, want)
			}

			normalizeSendLegsLikeCreate(&send)

			normalized, err := ValidateSendSourceAndDistribute(ctx, send, pkgConstant.CREATED)
			if err != nil {
				t.Fatalf("normalized validation: unexpected error %v", err)
			}

			if got := resolved(normalized)[tc.normalizedKey].Value; !got.Equal(want) {
				t.Fatalf("normalized validation resolved %s to %s, want %s", tc.normalizedKey, got, want)
			}

			legs := send.Send.Distribute.To
			if tc.fromSide {
				legs = send.Send.Source.From
			}

			remaining := legs[len(legs)-1]
			if remaining.Remaining == "" {
				t.Fatal("the remaining expression must survive validation on the leg")
			}

			if remaining.Amount == nil || !remaining.Amount.Value.Equal(want) {
				t.Fatalf("the leg must carry the resolved remainder %s as its Amount, got %+v", want, remaining.Amount)
			}
		})
	}
}

// TestValidateSendSourceAndDistribute_RemainingIgnoresWrittenAmount validates, in a single
// call, a send whose remaining leg already carries an Amount, which is the shape a
// persisted transaction body has when a pending commit validates it again. The leg must
// resolve to the remainder whatever that written Amount is: the remainder itself, a zero
// left by a pass that drained it, or any other value, which is the only case that tells
// "the remainder is recomputed" apart from "the written Amount is used".
func TestValidateSendSourceAndDistribute_RemainingIgnoresWrittenAmount(t *testing.T) {
	t.Parallel()

	ctx := differentialContext()

	for _, tc := range []struct {
		name    string
		written int64
	}{
		{"written amount equals the remainder", 40},
		{"written amount drained to zero", 0},
		{"written amount differs from the remainder", 25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			remaining := remainingLeg("@srcB")
			remaining.Amount = &Amount{Asset: "USD", Value: decimal.NewFromInt(tc.written)}

			send := sendOf("USD", 100,
				[]FromTo{leg("@srcA", "", "USD", 60), remaining},
				[]FromTo{leg("@dst", "", "USD", 100)})
			normalizeSendLegsLikeCreate(&send)

			validate, err := ValidateSendSourceAndDistribute(ctx, send, pkgConstant.APPROVED)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			if got := validate.From["1#@srcB#default"].Value; !got.Equal(decimal.NewFromInt(40)) {
				t.Fatalf("remaining leg resolved to %s, want 40", got)
			}
		})
	}
}
