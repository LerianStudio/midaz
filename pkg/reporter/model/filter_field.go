// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"regexp"
)

// filterFieldPattern matches a legitimate filter field reference: an identifier
// root, optionally followed by dotted JSONB/document path segments. It bounds the
// whole field to the SQL/Mongo identifier charset so a value like
// "metadata.x) OR (1=1) --" can never reach a query builder sink (a squirrel map
// key emitted unquoted, or a BSON map key written verbatim) and inject.
// Legitimate dotted paths such as "metadata.foo" remain valid.
var filterFieldPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)*$`)

// ValidateFieldName rejects a filter field whose full string is not a safe dotted
// identifier. Schema gates check only the ROOT column against the discovered
// schema, but the FULL field string is used verbatim as a query-builder map key —
// so a dotted path carrying SQL/operator escapes would pass the root check yet
// inject at the sink. This charset whitelist is the single source of truth shared
// by every connector gate that turns a caller-supplied field into a builder key.
func ValidateFieldName(field string) error {
	if !filterFieldPattern.MatchString(field) {
		return fmt.Errorf("invalid filter field name %q", field)
	}

	return nil
}
