// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const (
	CrossLedgerGroupIntentFormatVersion = 1
	CrossLedgerGroupRoleOrigin          = "origin"
	CrossLedgerGroupRoleDestination     = "destination"
)

// CrossLedgerGroupIntent freezes all normalized parts needed to complete a
// cross-ledger hold after the original request has returned.
type CrossLedgerGroupIntent struct {
	FormatVersion int                          `json:"formatVersion"`
	Asset         string                       `json:"asset"`
	Parts         []CrossLedgerGroupIntentPart `json:"parts"`
}

// CrossLedgerGroupIntentPart is one ordered, single-ledger transaction part.
type CrossLedgerGroupIntentPart struct {
	OrganizationID uuid.UUID                `json:"organizationId"`
	LedgerID       uuid.UUID                `json:"ledgerId"`
	Role           string                   `json:"role"`
	Order          int                      `json:"order"`
	Transaction    mtransaction.Transaction `json:"transaction"`
}

func buildCrossLedgerGroupIntent(
	asset string,
	parts []decomposedCrossLedgerPart,
) (CrossLedgerGroupIntent, error) {
	intent := CrossLedgerGroupIntent{
		FormatVersion: CrossLedgerGroupIntentFormatVersion,
		Asset:         asset,
		Parts:         make([]CrossLedgerGroupIntentPart, len(parts)),
	}

	for index := range parts {
		role, err := crossLedgerGroupPartRole(parts[index].transaction, asset)
		if err != nil {
			return CrossLedgerGroupIntent{}, fmt.Errorf("classify cross-ledger group part %d: %w", index, err)
		}

		intent.Parts[index] = CrossLedgerGroupIntentPart{
			OrganizationID: parts[index].ledgerRef.organizationID,
			LedgerID:       parts[index].ledgerRef.ledgerID,
			Role:           role,
			Order:          index + 1,
			Transaction:    parts[index].transaction,
		}
	}

	if err := validateCrossLedgerGroupIntent(intent); err != nil {
		return CrossLedgerGroupIntent{}, err
	}

	return intent, nil
}

func crossLedgerGroupPartRole(transaction mtransaction.Transaction, asset string) (string, error) {
	bridgeAlias := "@external/" + asset
	origin := crossLedgerLegsContainAlias(transaction.Send.Distribute.To, bridgeAlias)
	destination := crossLedgerLegsContainAlias(transaction.Send.Source.From, bridgeAlias)

	switch {
	case origin && !destination:
		return CrossLedgerGroupRoleOrigin, nil
	case destination && !origin:
		return CrossLedgerGroupRoleDestination, nil
	case !origin && !destination && len(transaction.Send.Source.From) > 0:
		// A net-zero participant has no bridge. It still owns source funds and
		// therefore belongs to the hold/transition side of the lifecycle.
		return CrossLedgerGroupRoleOrigin, nil
	default:
		return "", errors.New("part does not have one unambiguous external bridge")
	}
}

func crossLedgerLegsContainAlias(legs []mtransaction.FromTo, alias string) bool {
	for index := range legs {
		if legs[index].AccountAlias == alias {
			return true
		}
	}

	return false
}

func encodeCrossLedgerGroupIntent(intent CrossLedgerGroupIntent) ([]byte, error) {
	if err := validateCrossLedgerGroupIntent(intent); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(intent)
	if err != nil {
		return nil, fmt.Errorf("encode cross-ledger group intent: %w", err)
	}

	return raw, nil
}

func decodeCrossLedgerGroupIntent(raw []byte) (*CrossLedgerGroupIntent, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()

	var intent CrossLedgerGroupIntent
	if err := decoder.Decode(&intent); err != nil {
		return nil, fmt.Errorf("decode cross-ledger group intent: %w", err)
	}
	if decoder.More() {
		return nil, errors.New("decode cross-ledger group intent: trailing JSON value")
	}
	if err := validateCrossLedgerGroupIntent(intent); err != nil {
		return nil, err
	}

	return &intent, nil
}

func validateCrossLedgerGroupIntent(intent CrossLedgerGroupIntent) error {
	if intent.FormatVersion != CrossLedgerGroupIntentFormatVersion {
		return fmt.Errorf("unsupported cross-ledger group intent version %d", intent.FormatVersion)
	}
	if intent.Asset == "" || len(intent.Parts) < 2 {
		return errors.New("cross-ledger group intent is incomplete")
	}

	origins := 0
	destinations := 0
	for index := range intent.Parts {
		part := intent.Parts[index]
		if part.OrganizationID == uuid.Nil || part.LedgerID == uuid.Nil || part.Order != index+1 || part.Transaction.IsEmpty() {
			return fmt.Errorf("cross-ledger group intent part %d is invalid", index)
		}
		if part.Transaction.Send.Asset != intent.Asset {
			return fmt.Errorf("cross-ledger group intent part %d has a different asset", index)
		}

		switch part.Role {
		case CrossLedgerGroupRoleOrigin:
			origins++
		case CrossLedgerGroupRoleDestination:
			destinations++
		default:
			return fmt.Errorf("cross-ledger group intent part %d has invalid role %q", index, part.Role)
		}
	}

	if origins == 0 || destinations == 0 {
		return errors.New("cross-ledger group intent requires origin and destination parts")
	}

	return nil
}
