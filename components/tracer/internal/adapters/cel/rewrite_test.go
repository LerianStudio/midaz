// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"testing"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// currencyRefs counts, with correct scope resolution, how many references in an
// expression resolve to the GLOBAL currency variable versus how many
// comprehension-macro bindings are named currency. It is an INDEPENDENT verifier
// (it counts, it never rewrites) used to assert rewriter output structurally,
// since a consistent alpha-rename of a shadowed local is invisible to evaluation.
type currencyRefs struct {
	globals       int // free references resolving to the global currency
	localBindings int // comprehension iter/accu vars named currency
}

// inspectCurrency parses expr with macro-call tracking and walks the same surface
// tree the unparser renders, tracking a scope stack of comprehension-bound names.
func inspectCurrency(t *testing.T, expr string) currencyRefs {
	t.Helper()

	env, err := cel.NewEnv(cel.EnableMacroCallTracking())
	require.NoError(t, err)

	a, iss := env.Parse(expr)
	require.False(t, iss != nil && iss.Err() != nil, "parse %q: %v", expr, iss.Err())

	w := &refWalker{info: a.NativeRep().SourceInfo()}
	w.walk(a.NativeRep().Expr())

	return w.refs
}

type refWalker struct {
	info  *celast.SourceInfo
	refs  currencyRefs
	scope []map[string]struct{}
}

func (w *refWalker) shadowed(name string) bool {
	for _, f := range w.scope {
		if _, ok := f[name]; ok {
			return true
		}
	}

	return false
}

func (w *refWalker) push(comp celast.ComprehensionExpr) {
	w.pushNames(comp.IterVar(), comp.IterVar2(), comp.AccuVar())
}

func (w *refWalker) pushNames(names ...string) {
	f := map[string]struct{}{}

	for _, v := range names {
		if v != "" {
			f[v] = struct{}{}
		}

		if v == "currency" {
			w.refs.localBindings++
		}
	}

	w.scope = append(w.scope, f)
}

// macroBindings mirrors the rewriter's macroFrame: the bound iteration variable
// comes from the materialized comprehension, or from a nested comprehension
// macro's first argument when the expanded node is only a placeholder.
func (w *refWalker) macroBindings(expanded celast.Expr, call celast.CallExpr) ([]string, bool) {
	if expanded.Kind() == celast.ComprehensionKind {
		comp := expanded.AsComprehension()
		return []string{comp.IterVar(), comp.IterVar2(), comp.AccuVar()}, true
	}

	if _, ok := comprehensionMacros[call.FunctionName()]; ok {
		if args := call.Args(); len(args) > 0 && args[0].Kind() == celast.IdentKind {
			return []string{args[0].AsIdent()}, true
		}
	}

	return nil, false
}

func (w *refWalker) pop() { w.scope = w.scope[:len(w.scope)-1] }

func (w *refWalker) walk(e celast.Expr) {
	if e == nil {
		return
	}

	if mc, ok := w.info.GetMacroCall(e.ID()); ok {
		if mc.Kind() == celast.CallKind {
			call := mc.AsCall()

			if names, isComp := w.macroBindings(e, call); isComp {
				if call.IsMemberFunction() {
					w.walk(call.Target())
				}

				w.pushNames(names...)

				for _, a := range call.Args() {
					w.walk(a)
				}

				w.pop()

				return
			}
		}

		w.walk(mc)

		return
	}

	switch e.Kind() {
	case celast.IdentKind:
		if e.AsIdent() == "currency" && !w.shadowed("currency") {
			w.refs.globals++
		}
	case celast.SelectKind:
		w.walk(e.AsSelect().Operand())
	case celast.CallKind:
		c := e.AsCall()
		if c.IsMemberFunction() {
			w.walk(c.Target())
		}

		for _, a := range c.Args() {
			w.walk(a)
		}
	case celast.ListKind:
		for _, el := range e.AsList().Elements() {
			w.walk(el)
		}
	case celast.MapKind:
		for _, entry := range e.AsMap().Entries() {
			me := entry.AsMapEntry()
			w.walk(me.Key())
			w.walk(me.Value())
		}
	case celast.ComprehensionKind:
		comp := e.AsComprehension()
		w.walk(comp.IterRange())
		w.walk(comp.AccuInit())
		w.push(comp)
		w.walk(comp.LoopCondition())
		w.walk(comp.LoopStep())
		w.walk(comp.Result())
		w.pop()
	}
}

// mustCompileAsset asserts the expression compiles against the asset environment
// (the env after Task 2.2.1, which declares asset and NOT currency). A surviving
// GLOBAL currency reference fails here with "undeclared reference to 'currency'".
func mustCompileAsset(t *testing.T, expr string) {
	t.Helper()

	env, err := NewEnvironment()
	require.NoError(t, err)

	_, err = env.Compile(expr)
	require.NoError(t, err, "rewritten expression must compile against asset env: %q", expr)
}

func TestRewriteCurrencyToAsset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		wantGlobals    int  // expected GLOBAL currency refs in the OUTPUT (always 0)
		wantLocalKept  bool // a comprehension binding named currency must survive
		wantContains   string
		wantNotContain string
	}{
		// ---- MUST rewrite: references that resolve to the global currency ----
		{
			name:           "root global equality",
			input:          `currency == "BRL"`,
			wantContains:   "asset",
			wantNotContain: "currency",
		},
		{
			name:           "global in boolean composition",
			input:          `amount > 0 && currency == "USD"`,
			wantContains:   "asset",
			wantNotContain: "currency",
		},
		{
			name:         "global alongside unrelated comprehension iterVar",
			input:        `["USD"].exists(x, x == currency)`,
			wantContains: "asset",
		},
		// ---- MUST NOT change: field selections and string literals ----
		{
			name:         "field selection metadata.currency",
			input:        `metadata.currency == "BRL"`,
			wantContains: "metadata.currency",
		},
		{
			name:         "string literal in map index",
			input:        `metadata["currency"] == "BRL"`,
			wantContains: `"currency"`,
		},
		{
			name:         "map literal: global value renamed, literal key preserved",
			input:        `{"a": currency, "b": "currency"}.size() >= 0`,
			wantContains: `"currency"`, // the literal value must survive
		},
		// ---- MUST NOT change: comprehension-local bindings named currency ----
		{
			name:          "exists shadowing iterVar",
			input:         `["BRL"].exists(currency, currency == "BRL")`,
			wantLocalKept: true,
		},
		{
			name:          "map shadowing iterVar",
			input:         `["USD"].map(currency, currency).size() > 0`,
			wantLocalKept: true,
		},
		{
			name:          "filter shadowing iterVar",
			input:         `["BRL"].filter(currency, currency == "BRL").size() > 0`,
			wantLocalKept: true,
		},
		{
			name:          "all shadowing iterVar",
			input:         `["BRL"].all(currency, currency == "BRL")`,
			wantLocalKept: true,
		},
		// ---- Mixed: global rewrites, shadowed local preserved ----
		{
			name:          "mixed global and shadowing local",
			input:         `currency == "USD" && ["BRL"].exists(currency, currency == "BRL")`,
			wantLocalKept: true,
			wantContains:  "asset",
		},
		{
			name:          "global in range, shadowing local in body",
			input:         `[currency].exists(currency, currency == "X")`,
			wantLocalKept: true,
			wantContains:  "[asset]",
		},
		// ---- Nested comprehensions: the scope stack must resolve depth > 1 ----
		{
			// Inner `currency` is a LOCAL binding of the nested exists; `x` is the
			// outer iterVar; no global is present, so nothing is renamed.
			name:           "nested exists, inner currency is a local binding",
			input:          `["A"].exists(x, ["B"].exists(currency, currency == x))`,
			wantLocalKept:  true,
			wantContains:   "currency",
			wantNotContain: "asset",
		},
		{
			// The FIRST currency is the GLOBAL. The currency inside the nested all
			// body has no local currency binding in scope (iterVars are x and y),
			// so it ALSO resolves to the GLOBAL. Both are renamed to asset.
			name:           "nested all, deep global is renamed",
			input:          `currency == "Z" && ["A"].exists(x, ["B"].all(y, currency == y))`,
			wantContains:   "asset",
			wantNotContain: "currency",
		},
		{
			// Shadow inside shadow: both bindings are named currency, so the
			// innermost body reference is a local and stays currency; the ranges
			// carry no global.
			name:           "shadow inside shadow, both bindings named currency",
			input:          `["A"].exists(currency, ["B"].exists(currency, currency == "X"))`,
			wantLocalKept:  true,
			wantContains:   "currency",
			wantNotContain: "asset",
		},
		{
			// exists_one binds currency locally, so the body reference is preserved.
			name:           "exists_one shadowing iterVar",
			input:          `["BRL"].exists_one(currency, currency == "BRL")`,
			wantLocalKept:  true,
			wantNotContain: "asset",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := RewriteCurrencyToAsset(tt.input)
			require.NoError(t, err)

			// The output must always compile against the asset env: this proves no
			// GLOBAL currency reference survived (it would be undeclared).
			mustCompileAsset(t, out)

			refs := inspectCurrency(t, out)
			assert.Equal(t, tt.wantGlobals, refs.globals,
				"output %q must have no global currency references", out)

			if tt.wantLocalKept {
				assert.Positive(t, refs.localBindings,
					"shadowed local binding named currency must be preserved in %q", out)
			}

			if tt.wantContains != "" {
				assert.Contains(t, out, tt.wantContains)
			}

			if tt.wantNotContain != "" {
				assert.NotContains(t, out, tt.wantNotContain)
			}
		})
	}
}

// TestRewriteCurrencyToAsset_SemanticEquivalence asserts meaning is preserved by
// evaluating the ORIGINAL against a currency env and the REWRITTEN against the
// asset env across activations. A naive rewrite that mangled metadata.currency,
// a string literal, or the global reference would diverge here.
func TestRewriteCurrencyToAsset_SemanticEquivalence(t *testing.T) {
	t.Parallel()

	oldEnv, err := cel.NewEnv(
		cel.CrossTypeNumericComparisons(true),
		cel.Variable("amount", cel.DynType),
		cel.Variable("currency", cel.StringType),
		cel.Variable("metadata", cel.MapType(cel.StringType, cel.DynType)),
	)
	require.NoError(t, err)

	newEnv, err := cel.NewEnv(
		cel.CrossTypeNumericComparisons(true),
		cel.Variable("amount", cel.DynType),
		cel.Variable("asset", cel.StringType),
		cel.Variable("metadata", cel.MapType(cel.StringType, cel.DynType)),
	)
	require.NoError(t, err)

	evalBool := func(t *testing.T, env *cel.Env, expr string, act map[string]any) bool {
		t.Helper()

		ast, iss := env.Compile(expr)
		require.False(t, iss != nil && iss.Err() != nil, "compile %q: %v", expr, iss.Err())

		prg, perr := env.Program(ast)
		require.NoError(t, perr)

		out, _, eerr := prg.Eval(act)
		require.NoError(t, eerr)

		return out.Value().(bool)
	}

	cases := []string{
		`currency == "BRL"`,
		`amount > 0 && currency == "USD"`,
		`metadata.currency == "BRL"`,
		`metadata["currency"] == "BRL"`,
		`currency == "USD" && ["BRL"].exists(currency, currency == "BRL")`,
		// Nested comprehension: the top global and the global deep inside the all
		// body must both be renamed, or the rewritten expr would not compile
		// against the asset env below.
		`currency == "USD" && ["BRL"].all(y, currency == y)`,
	}

	// Activations vary the currency/asset value and the metadata payload so that a
	// mishandled literal or field select would surface as a divergent result.
	values := []string{"BRL", "USD", "EUR"}
	metas := []map[string]any{
		{"currency": "BRL"},
		{"currency": "USD"},
	}

	for _, expr := range cases {
		expr := expr
		t.Run(expr, func(t *testing.T) {
			t.Parallel()

			out, rerr := RewriteCurrencyToAsset(expr)
			require.NoError(t, rerr)

			for _, v := range values {
				for _, m := range metas {
					oldAct := map[string]any{"amount": 100.0, "currency": v, "metadata": m}
					newAct := map[string]any{"amount": 100.0, "asset": v, "metadata": m}

					got := evalBool(t, oldEnv, expr, oldAct)
					want := evalBool(t, newEnv, out, newAct)

					assert.Equal(t, got, want,
						"semantics diverged for %q (v=%s meta=%v): original=%v rewritten=%v (%q)",
						expr, v, m, got, want, out)
				}
			}
		})
	}
}

// collectIdents gathers every identifier name in the expanded AST (no scope
// tracking); used to assert renames on the non-macro comprehension path.
func collectIdents(e celast.Expr, out *[]string) {
	if e == nil {
		return
	}

	switch e.Kind() {
	case celast.IdentKind:
		*out = append(*out, e.AsIdent())
	case celast.SelectKind:
		collectIdents(e.AsSelect().Operand(), out)
	case celast.CallKind:
		c := e.AsCall()
		if c.IsMemberFunction() {
			collectIdents(c.Target(), out)
		}

		for _, a := range c.Args() {
			collectIdents(a, out)
		}
	case celast.ListKind:
		for _, el := range e.AsList().Elements() {
			collectIdents(el, out)
		}
	case celast.ComprehensionKind:
		comp := e.AsComprehension()
		collectIdents(comp.IterRange(), out)
		collectIdents(comp.AccuInit(), out)
		collectIdents(comp.LoopCondition(), out)
		collectIdents(comp.LoopStep(), out)
		collectIdents(comp.Result(), out)
	}
}

// TestRewriteWalker_NonMacroComprehension exercises the defensive walkComprehension
// path directly: with macro-call tracking OFF, a comprehension node carries no
// macro-call entry, so walk() falls through to the ComprehensionKind case rather
// than walkMacroCall. Scope resolution must still hold on the expanded tree.
func TestRewriteWalker_NonMacroComprehension(t *testing.T) {
	t.Parallel()

	env, err := cel.NewEnv() // no EnableMacroCallTracking: no macro-call entries
	require.NoError(t, err)

	tests := []struct {
		name        string
		expression  string
		wantIdent   string
		wantMsg     string
		absentIdent string
		absentMsg   string
	}{
		{
			name:        "global reference in body is renamed",
			expression:  `["USD"].exists(x, x == currency)`,
			wantIdent:   "asset",
			wantMsg:     "global currency must be renamed to asset",
			absentIdent: "currency",
			absentMsg:   "no global currency should remain",
		},
		{
			name:        "shadowed local binding is preserved",
			expression:  `["BRL"].exists(currency, currency == "BRL")`,
			wantIdent:   "currency",
			wantMsg:     "shadowed local binding must be preserved",
			absentIdent: "asset",
			absentMsg:   "shadowed local must not be renamed to asset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a, iss := env.Parse(tt.expression)
			require.False(t, iss != nil && iss.Err() != nil)

			nat := a.NativeRep()
			rw := &currencyRewriter{info: nat.SourceInfo(), fac: celast.NewExprFactory()}
			rw.walk(nat.Expr())

			var idents []string
			collectIdents(nat.Expr(), &idents)

			assert.Contains(t, idents, tt.wantIdent, tt.wantMsg)
			assert.NotContains(t, idents, tt.absentIdent, tt.absentMsg)
		})
	}
}

func TestRewriteCurrencyToAsset_Errors(t *testing.T) {
	t.Parallel()

	t.Run("empty expression", func(t *testing.T) {
		t.Parallel()

		_, err := RewriteCurrencyToAsset("")
		require.Error(t, err)
	})

	t.Run("invalid syntax", func(t *testing.T) {
		t.Parallel()

		_, err := RewriteCurrencyToAsset(`currency == ==`)
		require.Error(t, err)
	})

	t.Run("no currency reference is a no-op", func(t *testing.T) {
		t.Parallel()

		out, err := RewriteCurrencyToAsset(`amount > 0`)
		require.NoError(t, err)
		assert.NotContains(t, out, "currency")
		assert.NotContains(t, out, "asset")

		refs := inspectCurrency(t, out)
		assert.Equal(t, 0, refs.globals)
	})
}
