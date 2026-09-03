// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"context"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/LerianStudio/lib-observability/v4/log"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel"

	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// The transaction create and revert pipelines call ValidateSendSourceAndDistribute
// twice over the same send: once on the raw input and once after the legs are
// normalized. These tests run both calls over one corpus and pin the exact relation
// between their outcomes.
//
// Values are untouched by normalization — it only sets IsFrom on the sources, defaults
// the balance keys, and rewrites AccountAlias into the composite
// "index#alias#balanceKey" entry key — so the totals check answers identically on both
// sides of it. The ambiguity check does not: it reads Responses.To keyed by the entry
// key ConcatAlias builds, while Responses.To on a raw input is keyed by the bare alias
// the caller sent. The two key spaces only meet once MutateConcatAliases has run.
//
// That makes the raw call blind to ambiguity, and the two calls diverge in two ways
// whenever one alias appears on both sides of a send at the same index:
//
//   - the raw call accepts and the normalized call answers ErrTransactionAmbiguous;
//   - the raw call answers ErrTransactionValueMismatch and the normalized call answers
//     ErrTransactionAmbiguous instead, because the ambiguity check runs before the
//     totals check.
//
// Cases in either class carry normalizedAnswersAmbiguous. Outside that class the two
// calls agree on the code, and no rejection the raw call makes is ever lost.

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

// ambiguousCode is the code the normalized call answers for a send carrying one alias
// on both sides at the same index.
func ambiguousCode() string {
	return pkgConstant.ErrTransactionAmbiguous.Error()
}

// differentialCase is one input run through both validations.
type differentialCase struct {
	name   string
	status string
	build  func() Transaction

	// normalizedAnswersAmbiguous marks the class the raw call cannot see: the
	// normalized call answers ErrTransactionAmbiguous where the raw call answered
	// something else — nil, or the totals mismatch the ambiguity check preempts.
	normalizedAnswersAmbiguous bool
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
			normalizedAnswersAmbiguous: true,
		},
		{
			name:   "same alias on both sides at the same index with a mismatching total",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				return sendOf("USD", 100, []FromTo{leg("@account", "", "USD", 100)}, []FromTo{leg("@account", "", "USD", 64)})
			},
			normalizedAnswersAmbiguous: true,
		},
		{
			name:   "same alias on both sides pending",
			status: pkgConstant.PENDING,
			build: func() Transaction {
				tx := sendOf("USD", 100, []FromTo{leg("@account", "", "USD", 100)}, []FromTo{leg("@account", "", "USD", 100)})
				tx.Pending = true

				return tx
			},
			normalizedAnswersAmbiguous: true,
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
			name:   "v1 json dto",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				input := &CreateTransactionInput{
					Send: Send{
						Asset:      "USD",
						Value:      decimal.NewFromInt(100),
						Source:     Source{From: []FromTo{leg("@source", "", "USD", 100)}},
						Distribute: Distribute{To: []FromTo{leg("@destination", "", "USD", 100)}},
					},
				}

				return *input.BuildTransaction()
			},
		},
		{
			name:   "v1 json dto with a mismatching total",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				input := &CreateTransactionInput{
					Send: Send{
						Asset:      "USD",
						Value:      decimal.NewFromInt(100),
						Source:     Source{From: []FromTo{leg("@source", "", "USD", 55)}},
						Distribute: Distribute{To: []FromTo{leg("@destination", "", "USD", 55)}},
					},
				}

				return *input.BuildTransaction()
			},
		},
		{
			name:   "v1 json dto with one alias on both sides",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				input := &CreateTransactionInput{
					Send: Send{
						Asset:      "USD",
						Value:      decimal.NewFromInt(100),
						Source:     Source{From: []FromTo{leg("@account", "", "USD", 100)}},
						Distribute: Distribute{To: []FromTo{leg("@account", "", "USD", 100)}},
					},
				}

				return *input.BuildTransaction()
			},
			normalizedAnswersAmbiguous: true,
		},
		{
			name:   "v1 inflow dto",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				input := &CreateTransactionInflowInput{
					Send: SendInflow{
						Asset:      "USD",
						Value:      decimal.NewFromInt(100),
						Distribute: Distribute{To: []FromTo{leg("@destination", "", "USD", 100)}},
					},
				}

				return *input.BuildInflowEntry()
			},
		},
		{
			name:   "v1 outflow dto",
			status: pkgConstant.CREATED,
			build: func() Transaction {
				input := &CreateTransactionOutflowInput{
					Send: SendOutflow{
						Asset:  "USD",
						Value:  decimal.NewFromInt(100),
						Source: Source{From: []FromTo{leg("@source", "", "USD", 100)}},
					},
				}

				return *input.BuildOutflowEntry()
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
// through both validations and requires the same code, except for the ambiguity class
// the raw call cannot see, whose divergence is pinned exactly.
func TestValidateSendSourceAndDistribute_RawAndNormalizedAgree(t *testing.T) {
	t.Parallel()

	ctx := differentialContext()

	for _, tc := range differentialCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rawCode, normalizedCode := runBothValidations(ctx, tc)

			if tc.normalizedAnswersAmbiguous {
				if normalizedCode != ambiguousCode() {
					t.Fatalf("normalized validation answered %q, want %q", normalizedCode, ambiguousCode())
				}

				if rawCode == ambiguousCode() {
					t.Fatal("raw validation answered the ambiguity code; the raw call has no entry keys to see it with")
				}

				return
			}

			if rawCode != normalizedCode {
				t.Fatalf("raw and normalized returned different codes: raw=%q normalized=%q", rawCode, normalizedCode)
			}
		})
	}
}

// TestValidateSendSourceAndDistribute_NormalizedNeverAcceptsWhatRawRejects asserts the
// direction that governs whether the raw call can be dropped: every rejection survives
// normalization, and it keeps its code unless the ambiguity check preempts it.
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

			if normalizedCode != rawCode && normalizedCode != ambiguousCode() {
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
// pins: no rejection is lost across normalization, and the only code the normalized
// call may answer in place of the raw one is the ambiguity the entry keys make visible.
func FuzzValidateSendRawVsNormalized(f *testing.F) {
	f.Add(int64(100), "@source", "@destination", "", int64(100), int64(100), uint8(0), uint8(0))
	f.Add(int64(100), "@source", "@destination", "savings", int64(60), int64(100), uint8(0), uint8(1))
	f.Add(int64(100), "@account", "@account", "", int64(100), int64(100), uint8(0), uint8(0))
	f.Add(int64(100), "@source", "@destination", "", int64(0), int64(100), uint8(1), uint8(2))
	f.Add(int64(100), "@source", "@destination", "", int64(0), int64(100), uint8(2), uint8(3))
	f.Add(int64(0), "", "", "", int64(0), int64(0), uint8(0), uint8(0))
	f.Add(int64(100), "@source#savings", "@destination", "wallet", int64(100), int64(100), uint8(0), uint8(0))

	statuses := []string{pkgConstant.CREATED, pkgConstant.PENDING, pkgConstant.NOTED, pkgConstant.APPROVED}

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

		if rawCode != "" && normalizedCode == "" {
			t.Fatalf("raw rejected with %q and normalized accepted: sendValue=%d source=%q destination=%q balanceKey=%q expression=%d status=%s",
				rawCode, sendValue, sourceAlias, destinationAlias, balanceKey, expression%3, status)
		}

		if normalizedCode != rawCode && normalizedCode != ambiguousCode() {
			t.Fatalf("raw answered %q and normalized answered %q, which is outside the ambiguity class: sendValue=%d source=%q destination=%q balanceKey=%q expression=%d status=%s",
				rawCode, normalizedCode, sendValue, sourceAlias, destinationAlias, balanceKey, expression%3, status)
		}
	})
}
