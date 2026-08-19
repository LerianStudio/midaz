// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/parser"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	// oldGlobalName is the CEL global variable being renamed.
	oldGlobalName = "currency"
	// newGlobalName is the replacement CEL global variable name.
	newGlobalName = "asset"
)

// rewriteEnv is a parse-only CEL environment used to obtain an AST for a stored
// rule. Macro-call tracking is REQUIRED: the unparser renders comprehension
// macros (exists/all/exists_one/map/filter) from the macro-call map, and without
// tracking it cannot serialize a macro expression at all. Standard macros are
// enabled by default. The environment is syntactic only (no variable declarations)
// because renaming operates on the parsed expression, never on type resolution;
// this keeps the rewriter usable for rules that reference the now-removed currency
// global, which would fail type checking.
var rewriteEnv = sync.OnceValues(func() (*cel.Env, error) {
	return cel.NewEnv(cel.EnableMacroCallTracking())
})

// RewriteCurrencyToAsset renames every reference that resolves to the GLOBAL
// currency variable to asset, leaving field selections (e.g. metadata.currency),
// string literals (e.g. "currency"), and comprehension-macro local bindings named
// currency untouched. Scope resolution is exact: a currency introduced as an
// iteration or accumulation variable shadows the global within the macro body, so
// those references are preserved.
//
// The second return value reports whether at least one global currency reference
// was renamed. It is false when the expression carries no global currency, even
// though the returned text is the canonical serialization and may therefore differ
// from a non-canonically stored input (e.g. "amount>0" serializes to "amount > 0"
// without any rename). Callers that persist only genuinely-renamed rules MUST
// branch on this flag, not on text inequality.
//
// The function is pure: it performs no I/O and mutates no shared state. It returns
// the canonical serialization of the rewritten expression. A parse failure is
// reported as constant.ErrExpressionSyntax.
func RewriteCurrencyToAsset(expression string) (string, bool, error) {
	if expression == "" {
		return "", false, fmt.Errorf("%w: expression cannot be empty", constant.ErrExpressionSyntax)
	}

	env, err := rewriteEnv()
	if err != nil {
		return "", false, fmt.Errorf("failed to create rewrite environment: %w", err)
	}

	parsed, issues := env.Parse(expression)
	if issues != nil && issues.Err() != nil {
		return "", false, fmt.Errorf("%w: %w", constant.ErrExpressionSyntax, issues.Err())
	}

	nativeAST := parsed.NativeRep()

	rw := &currencyRewriter{
		info: nativeAST.SourceInfo(),
		fac:  celast.NewExprFactory(),
	}
	rw.walk(nativeAST.Expr())

	out, err := parser.Unparse(nativeAST.Expr(), nativeAST.SourceInfo())
	if err != nil {
		return "", false, fmt.Errorf("%w: failed to serialize rewritten expression: %w", constant.ErrExpressionSyntax, err)
	}

	return out, rw.changed, nil
}

// currencyRewriter walks the surface expression tree the unparser renders (macro
// calls grafted at macro-expanded node IDs) while maintaining a scope stack of
// names bound by enclosing comprehension macros.
type currencyRewriter struct {
	info    *celast.SourceInfo
	fac     celast.ExprFactory
	scope   []map[string]struct{}
	changed bool // set once a global currency ident is renamed to asset
}

// shadowed reports whether name is currently bound by an enclosing comprehension.
func (r *currencyRewriter) shadowed(name string) bool {
	for _, frame := range r.scope {
		if _, ok := frame[name]; ok {
			return true
		}
	}

	return false
}

// comprehensionMacros are the standard CEL comprehension macros. Each binds an
// iteration variable (their first argument) that shadows the global inside the
// predicate/transform argument. `has` is deliberately absent: it binds nothing.
var comprehensionMacros = map[string]struct{}{
	"exists":     {},
	"all":        {},
	"exists_one": {},
	"map":        {},
	"filter":     {},
}

// frameFromComprehension collects the names a materialized comprehension binds:
// its iteration variable(s) and accumulator.
func frameFromComprehension(comp celast.ComprehensionExpr) map[string]struct{} {
	frame := map[string]struct{}{}

	if v := comp.IterVar(); v != "" {
		frame[v] = struct{}{}
	}

	if comp.HasIterVar2() {
		frame[comp.IterVar2()] = struct{}{}
	}

	if v := comp.AccuVar(); v != "" {
		frame[v] = struct{}{}
	}

	return frame
}

// push binds a comprehension's iteration and accumulation variables for the
// duration of its body traversal.
func (r *currencyRewriter) push(comp celast.ComprehensionExpr) {
	r.scope = append(r.scope, frameFromComprehension(comp))
}

func (r *currencyRewriter) pop() {
	r.scope = r.scope[:len(r.scope)-1]
}

// walk visits e, renaming the global currency ident and recursing into children.
// A macro-expanded node is handled through its tracked macro call, mirroring the
// unparser so the serialized output reflects the rename.
func (r *currencyRewriter) walk(e celast.Expr) {
	if e == nil {
		return
	}

	if mc, ok := r.info.GetMacroCall(e.ID()); ok {
		r.walkMacroCall(e, mc)
		return
	}

	switch e.Kind() {
	case celast.IdentKind:
		if e.AsIdent() == oldGlobalName && !r.shadowed(oldGlobalName) {
			e.SetKindCase(r.fac.NewIdent(e.ID(), newGlobalName))
			r.changed = true
		}
	case celast.SelectKind:
		// Recurse only into the operand; the field name is not an identifier and
		// must never be renamed (metadata.currency stays metadata.currency).
		r.walk(e.AsSelect().Operand())
	case celast.CallKind:
		c := e.AsCall()
		if c.IsMemberFunction() {
			r.walk(c.Target())
		}

		for _, a := range c.Args() {
			r.walk(a)
		}
	case celast.ListKind:
		for _, el := range e.AsList().Elements() {
			r.walk(el)
		}
	case celast.MapKind:
		for _, entry := range e.AsMap().Entries() {
			me := entry.AsMapEntry()
			r.walk(me.Key())
			r.walk(me.Value())
		}
	case celast.StructKind:
		for _, f := range e.AsStruct().Fields() {
			r.walk(f.AsStructField().Value())
		}
	case celast.ComprehensionKind:
		r.walkComprehension(e.AsComprehension())
	case celast.LiteralKind, celast.UnspecifiedExprKind:
		// Literals carry no identifiers to rename.
	}
}

// walkComprehension applies comprehension scope rules: the iteration range and the
// accumulator initializer evaluate in the enclosing scope (bound names not yet in
// effect); the loop condition, loop step, and result evaluate with the bound names
// pushed.
func (r *currencyRewriter) walkComprehension(comp celast.ComprehensionExpr) {
	r.walk(comp.IterRange())
	r.walk(comp.AccuInit())
	r.push(comp)
	r.walk(comp.LoopCondition())
	r.walk(comp.LoopStep())
	r.walk(comp.Result())
	r.pop()
}

// walkMacroCall rewrites the surface (pre-expansion) form the unparser serializes.
// For a comprehension macro the receiver is the iteration range and is walked in
// the enclosing scope; the remaining call arguments (the iteration-variable
// binding occurrences and the macro body) are walked with the comprehension's
// bound names pushed, so a binding named currency shadows the global. Non-macro
// forms (e.g. a has() presence test) introduce no bindings and are walked as-is.
//
// The bound names come from the expanded comprehension when it is materialized
// here (the top-level macro), or from the macro call's binding argument when the
// expanded node is only a placeholder — which is how a comprehension macro
// NESTED as another macro's argument is represented (kind Unspecified, but still
// macro-tracked). Deriving the frame from the call in that case keeps a nested
// binding named currency shadowing the global; otherwise the nested binding
// would be walked in the enclosing scope only and wrongly renamed.
func (r *currencyRewriter) walkMacroCall(expanded, macroCall celast.Expr) {
	if macroCall.Kind() != celast.CallKind {
		r.walk(macroCall)
		return
	}

	call := macroCall.AsCall()

	frame, ok := r.macroFrame(expanded, call)
	if !ok {
		r.walk(macroCall)
		return
	}

	if call.IsMemberFunction() {
		r.walk(call.Target())
	}

	r.scope = append(r.scope, frame)

	for _, a := range call.Args() {
		r.walk(a)
	}

	r.pop()
}

// macroFrame returns the names an enclosing comprehension macro binds, and
// whether macroCall is a comprehension macro at all. A materialized expanded
// comprehension carries the authoritative iter/accumulator vars; a placeholder
// expanded node (a nested macro argument) falls back to the standard macro's
// first argument, which is its iteration variable.
func (r *currencyRewriter) macroFrame(expanded celast.Expr, call celast.CallExpr) (map[string]struct{}, bool) {
	if expanded.Kind() == celast.ComprehensionKind {
		return frameFromComprehension(expanded.AsComprehension()), true
	}

	if _, ok := comprehensionMacros[call.FunctionName()]; ok {
		if args := call.Args(); len(args) > 0 && args[0].Kind() == celast.IdentKind {
			return map[string]struct{}{args[0].AsIdent(): {}}, true
		}
	}

	return nil, false
}
