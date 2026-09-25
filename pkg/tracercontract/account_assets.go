// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"context"

	"github.com/google/uuid"
)

// AccountAsset is a producer's official account-to-asset fact. It contains no
// limit definition, policy, organization or Ledger-specific primitive. Only a
// verified producer may attest these facts; syntax alone proves no ownership.
type AccountAsset struct {
	AccountID uuid.UUID `json:"accountId"`
	Asset     AssetRef  `json:"asset"`
}

// ValidateAccountAssets checks a complete bounded fact set. Matching those facts
// to a limit and requiring one economic asset are Tracer's responsibilities.
func ValidateAccountAssets(ctx context.Context, facts []AccountAsset, namespace string, bounds Limits) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := bounds.Validate(); err != nil {
		return err
	}

	if !validText(namespace, bounds.MaxTextBytes) || len(facts) == 0 || len(facts) > bounds.MaxAccounts {
		return invalid("account asset facts")
	}

	seen := make(map[uuid.UUID]struct{}, len(facts))

	assets := make(map[AssetIdentity]string, len(facts))
	for _, fact := range facts {
		if err := ctx.Err(); err != nil {
			return err
		}

		if _, duplicate := seen[fact.AccountID]; duplicate || fact.AccountID == uuid.Nil {
			return invalid("account asset identity")
		}

		if err := validateAsset(fact.Asset, namespace, bounds, assets); err != nil {
			return err
		}

		seen[fact.AccountID] = struct{}{}
	}

	return nil
}
