// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// LegacyTransactionIdempotencyHash reproduces the released payload-scoped
// transaction identity. Bridge HTTP admission and phase-zero backup adoption
// share it so the durable claim records the exact legacy fence key.
// The caller's slices are copied before defaulting so the input is not mutated.
func LegacyTransactionIdempotencyHash(input mtransaction.Transaction) (string, error) {
	input.Send.Source.From = append([]mtransaction.FromTo(nil), input.Send.Source.From...)
	input.Send.Distribute.To = append([]mtransaction.FromTo(nil), input.Send.Distribute.To...)

	mtransaction.ApplyDefaultBalanceKeys(input.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(input.Send.Distribute.To)

	serialized, err := libCommons.StructToJSONString(input)
	if err != nil {
		return "", fmt.Errorf("serialize transaction for legacy idempotency hash: %w", err)
	}

	return libCommons.HashSHA256(serialized), nil
}
