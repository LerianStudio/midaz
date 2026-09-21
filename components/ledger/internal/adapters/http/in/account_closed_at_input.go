// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"

	"github.com/LerianStudio/midaz/v4/pkg"
)

// accountClosedAtInputKeys are the spellings the closing instant could arrive
// under: the wire name and the column name. Both are refused, because a caller
// that reaches for either is asking for the same forbidden write.
var accountClosedAtInputKeys = []string{"closedAt", "closed_at"}

// rejectAccountClosedAtInput refuses an account create or update body that names
// the closing instant at its root. The instant is written only by the close
// command, so the field is not an input on either contract.
//
// The rejection is by PRESENCE, whatever the value: null, false and 0 all count.
// Absence from the input struct is not enough to catch them — the shared unknown-
// field detection compares the body against the marshalled struct, and those three
// values are also what omitempty drops, so each is indistinguishable there from a
// field that was simply not sent.
//
// A body that is not a JSON object is left alone: malformed JSON keeps its own
// contract, and the shared decoder reports it.
func rejectAccountClosedAtInput(rawBody []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &root); err != nil {
		return nil //nolint:nilerr // a body this guard cannot read is not its to refuse; the shared decoder reports it
	}

	unexpected := make(pkg.UnknownFields)

	for _, key := range accountClosedAtInputKeys {
		raw, present := root[key]
		if !present {
			continue
		}

		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			value = nil
		}

		unexpected[key] = value
	}

	if len(unexpected) == 0 {
		return nil
	}

	return pkg.ValidateBadRequestFieldsError(pkg.FieldValidations{}, pkg.FieldValidations{}, "", unexpected)
}
