// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg"
)

// numberChars are the characters a decimal literal may contain.
const numberChars = "0123456789.+-eE"

// maxBodyDepth bounds request nesting; the deepest Midaz request schema has seven levels.
const maxBodyDepth = 64

type scanFrame struct {
	key      string
	index    int
	object   bool
	awaitKey bool
	node     *decimalNode
}

// RefuseOutOfBoundTokens refuses a body nested deeper than maxBodyDepth, an object key or JSON number
// out of bounds, or a decimal field of target whose text safe.ParseDecimal refuses as out of bounds.
// It reads every raw token, a repeated key included, before any decode.
func RefuseOutOfBoundTokens(body []byte, target any) ([]pkg.FieldError, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	root := decimalNodeOf(reflect.TypeOf(target))

	var stack []scanFrame

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, nil //nolint:nilerr // end of body; a malformed body is refused by the decode that follows
		}

		node := root

		if n := len(stack); n > 0 {
			top := &stack[n-1]
			if key, ok := tok.(string); ok && top.awaitKey {
				if len(key) > safe.MaxDecimalTextLength {
					return refuseToken(stack[:n-1], "has a key longer than the supported length")
				}

				top.key, top.awaitKey = key, false

				continue
			}

			node = top.node.child(*top)
		}

		if d, ok := tok.(json.Delim); ok && (d == '{' || d == '[') {
			if len(stack) == maxBodyDepth {
				return refuseToken(stack, "exceeds the supported nesting depth")
			}

			stack = append(stack, scanFrame{object: d == '{', awaitKey: d == '{', node: node})

			continue
		} else if ok {
			stack = stack[:len(stack)-1]
		} else if reason := scalarRefusal(tok, node); reason != "" {
			return refuseToken(stack, reason)
		}

		if n := len(stack); n > 0 {
			stack[n-1].awaitKey = stack[n-1].object
			stack[n-1].index++
		}
	}
}

// scalarRefusal bounds every JSON number, and a string only where the target holds a decimal.
func scalarRefusal(tok json.Token, node *decimalNode) string {
	switch t := tok.(type) {
	case json.Number:
		return numericTextRefusal(string(t))
	case string:
		if node != nil && node.decimal {
			return numericTextRefusal(t)
		}
	}

	return ""
}

func numericTextRefusal(text string) string {
	run := len(text) - len(strings.TrimLeft(text, numberChars))
	if run > safe.MaxDecimalTextLength {
		return "exceeds the supported numeric length"
	}

	if text != "" && run == len(text) {
		if _, err := safe.ParseDecimal(text); errors.Is(err, safe.ErrDecimalOutOfBounds) {
			return "is outside the supported decimal range"
		}
	}

	return ""
}

func refuseToken(frames []scanFrame, reason string) ([]pkg.FieldError, error) {
	field := renderPath(frames)

	subject := field
	if subject == "" {
		subject = "request body"
	}

	invalid := pkg.FieldValidations{field: subject + " " + reason}

	return fieldValidationDetails(invalid), pkg.ValidateBadRequestFieldsError(pkg.FieldValidations{}, invalid, "", nil)
}

// renderPath writes each object frame's key and each array frame's index, as validator namespaces do.
func renderPath(frames []scanFrame) string {
	var path strings.Builder

	for _, f := range frames {
		if !f.object {
			fmt.Fprintf(&path, "[%d]", f.index)

			continue
		}

		if path.Len() > 0 {
			path.WriteByte('.')
		}

		path.WriteString(f.key)
	}

	return path.String()
}

// decimalNode is one JSON value of a decode target: a decimal, a struct's fields, or a container's element.
type decimalNode struct {
	decimal bool
	fields  map[string]*decimalNode // struct fields by folded JSON name
	elem    *decimalNode            // slice or array element, map value
}

var (
	decimalType  = reflect.TypeFor[decimal.Decimal]()
	decimalNodes sync.Map // reflect.Type -> *decimalNode
)

func decimalNodeOf(t reflect.Type) *decimalNode {
	if t == nil {
		return nil
	}

	if n, ok := decimalNodes.Load(t); ok {
		return n.(*decimalNode)
	}

	n := buildDecimalNode(t, map[reflect.Type]*decimalNode{})
	decimalNodes.Store(t, n)

	return n
}

func buildDecimalNode(t reflect.Type, seen map[reflect.Type]*decimalNode) *decimalNode {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if n, ok := seen[t]; ok {
		return n
	}

	n := &decimalNode{decimal: t == decimalType}
	seen[t] = n

	switch {
	case n.decimal:
	case t.Kind() == reflect.Slice, t.Kind() == reflect.Array, t.Kind() == reflect.Map:
		n.elem = buildDecimalNode(t.Elem(), seen)
	case t.Kind() == reflect.Struct:
		n.fields = map[string]*decimalNode{}
		addDecimalFields(n.fields, t, seen, false)
	}

	return n
}

// addDecimalFields names fields as encoding/json does; a promoted field never replaces a direct one.
func addDecimalFields(fields map[string]*decimalNode, t reflect.Type, seen map[reflect.Type]*decimalNode, promoted bool) {
	for i := range t.NumField() {
		f := t.Field(i)

		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}

		name, _, _ := strings.Cut(tag, ",")

		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		if f.Anonymous && name == "" && ft.Kind() == reflect.Struct {
			addDecimalFields(fields, ft, seen, true)

			continue
		}

		if !f.IsExported() {
			continue
		}

		if name == "" {
			name = f.Name
		}

		key := foldName(name)
		if _, ok := fields[key]; ok && promoted {
			continue
		}

		fields[key] = buildDecimalNode(f.Type, seen)
	}
}

func (n *decimalNode) child(f scanFrame) *decimalNode {
	switch {
	case n == nil:
		return nil
	case f.object && n.fields != nil:
		return n.fields[foldName(f.key)]
	default:
		return n.elem
	}
}

// foldName folds a key as encoding/json does when it matches a key to a field without regard to case.
func foldName(s string) string {
	return strings.Map(func(r rune) rune { return unicode.ToUpper(unicode.ToLower(r)) }, s)
}
