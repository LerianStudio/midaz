// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"fmt"
	"reflect"

	celgo "github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type contextAsset struct {
	Namespace string `cel:"namespace"`
	ID        string `cel:"id"`
	Code      string `cel:"code"`
}

type contextAccount struct {
	ID      string       `cel:"id"`
	Type    string       `cel:"type"`
	Status  string       `cel:"status"`
	Blocked *bool        `cel:"blocked"`
	Asset   contextAsset `cel:"asset"`
}

type contextEntry struct {
	AccountID *string      `cel:"accountId"`
	External  bool         `cel:"external"`
	Direction string       `cel:"direction"`
	Amount    decimalValue `cel:"amount"`
	Asset     contextAsset `cel:"asset"`
}

type contextDebit struct {
	AccountID string       `cel:"accountId"`
	Amount    decimalValue `cel:"amount"`
	Asset     contextAsset `cel:"asset"`
}

// contextNativeType overrides reflected Decimal fields with the opaque CEL
// type. All other fields retain their real types and native presence checks.
type contextNativeType struct{ *types.NativeType }

func (t contextNativeType) FindFieldType(name string) (*types.FieldType, bool) {
	field, ok := t.NativeType.FindFieldType(name)
	if ok && name == "amount" {
		field.Type = decimalCELType
	}

	return field, ok
}

func (contextNativeType) NewValue(_ types.Adapter, _ map[string]ref.Val) ref.Val {
	return types.NewErr("evaluation context objects are read-only inputs")
}

func contextEnvironment(config ContextAdapterConfig) (*celgo.Env, error) {
	registry, err := types.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("create context type registry: %w", err)
	}

	for _, native := range []reflect.Type{
		reflect.TypeFor[contextAsset](), reflect.TypeFor[contextAccount](),
		reflect.TypeFor[contextEntry](), reflect.TypeFor[contextDebit](),
	} {
		typeInfo, err := types.NewNativeType(native, types.ParseStructTags(true))
		if err != nil {
			return nil, fmt.Errorf("describe context type: %w", err)
		}

		if err := registry.RegisterType(contextNativeType{NativeType: typeInfo}); err != nil {
			return nil, fmt.Errorf("register context type: %w", err)
		}
	}

	return celgo.NewEnv(
		celgo.CustomTypeProvider(registry), celgo.CustomTypeAdapter(registry),
		celgo.Lib(decimalLibrary{limits: config.Limits}),
		celgo.Variable("accounts", celgo.ListType(celgo.ObjectType("cel.contextAccount"))),
		celgo.Variable("entries", celgo.ListType(celgo.ObjectType("cel.contextEntry"))),
		celgo.Variable("debits", celgo.ListType(celgo.ObjectType("cel.contextDebit"))),
	)
}

// ContextActivation is a validated, detached snapshot reusable across rules of
// one evaluation. Its fields are private so callers cannot inject float values,
// bypass size limits or supply fabricated precomputed consumption.
type ContextActivation struct {
	owner  *ContextAdapter
	values map[string]any
}

// Prepare validates producer facts, computes consumption within the Tracer
// domain and creates typed CEL values. It never imports a Ledger model or makes
// an external lookup. Namespace comes from authenticated integration configuration.
func (a *ContextAdapter) Prepare(ctx context.Context, facts tracercontract.Context, namespace string) (*ContextActivation, error) {
	debits, err := model.AccountDebits(ctx, facts, namespace, a.config.Limits)
	if err != nil {
		return nil, err
	}

	accounts := make([]contextAccount, 0, len(facts.Accounts))
	for _, account := range facts.Accounts {
		blocked := *account.Blocked // validated by AccountDebits
		accounts = append(accounts, contextAccount{
			ID: account.ID.String(), Type: account.Type, Status: account.Status, Blocked: &blocked,
			Asset: assetActivation(account.Asset),
		})
	}

	entries := make([]contextEntry, 0, len(facts.Entries))
	for _, entry := range facts.Entries {
		amount, err := entry.Amount.Decimal(ctx, a.config.Limits)
		if err != nil {
			return nil, err
		}

		view := contextEntry{
			External: entry.External, Direction: string(entry.Direction),
			Amount: decimalValue{amount: amount, size: uint64(len(entry.Amount))}, Asset: assetActivation(entry.Asset),
		}
		if !entry.External {
			id := entry.AccountID.String()
			view.AccountID = &id
		}

		entries = append(entries, view)
	}

	consumption := make([]contextDebit, 0, len(debits))
	for _, debit := range debits {
		raw, err := tracercontract.AmountFromDecimal(ctx, debit.Amount, a.config.Limits)
		if err != nil {
			return nil, err
		}

		consumption = append(consumption, contextDebit{
			AccountID: debit.AccountID.String(), Asset: assetActivation(debit.Asset),
			Amount: decimalValue{amount: debit.Amount, size: uint64(len(raw))},
		})
	}

	return &ContextActivation{owner: a, values: map[string]any{"accounts": accounts, "entries": entries, "debits": consumption}}, nil
}

func assetActivation(asset tracercontract.AssetRef) contextAsset {
	return contextAsset{Namespace: asset.Namespace, ID: asset.ID, Code: asset.Code}
}
