// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balancecache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Shape identifies the cache field set without changing or rewriting the value.
type Shape string

const (
	ShapeLegacy  Shape = "legacy"
	ShapeDual    Shape = "dual"
	ShapeNewOnly Shape = "new_only"
	ShapeMixed   Shape = "mixed"
	ShapeInvalid Shape = "invalid"
)

// InventoryEntry is one cache value supplied by a complete external key walk.
type InventoryEntry struct {
	Key   string
	Value []byte
}

// InventoryIssue identifies a value that a complete dual-writer rollout should
// not leave behind. It contains no balance data.
type InventoryIssue struct {
	Key   string
	Shape Shape
	Error string
}

// InventoryReport is a deterministic, read-only summary of supplied values.
// Callers remain responsible for proving that their key walk was complete.
type InventoryReport struct {
	Total   int
	Legacy  int
	Dual    int
	NewOnly int
	Mixed   int
	Invalid int
	Issues  []InventoryIssue
}

// ClassifyShape validates a value through the compatible reader and classifies
// its field set. Mixed schema-version-two values remain readable, but are not
// evidence that all dedicated writers emit a complete dual representation.
func ClassifyShape(raw []byte) (Shape, error) {
	fields, err := decodeObject(raw)
	if err != nil {
		return ShapeInvalid, err
	}

	if _, err := DecodeForRead(raw); err != nil {
		return ShapeInvalid, err
	}

	if _, versioned := fields["SchemaVersion"]; !versioned {
		return ShapeLegacy, nil
	}

	legacy, modern := 0, 0

	for _, name := range fieldNames {
		if _, present := fields[name]; present {
			legacy++
		}

		if _, present := fields[lowerName(name)]; present {
			modern++
		}
	}

	switch {
	case legacy == len(fieldNames) && modern == len(fieldNames):
		if err := validateDualEquivalence(raw, fields); err != nil {
			return ShapeInvalid, err
		}

		return ShapeDual, nil
	case legacy == 0 && modern == len(fieldNames):
		return ShapeNewOnly, nil
	default:
		return ShapeMixed, nil
	}
}

func validateDualEquivalence(raw []byte, fields map[string]json.RawMessage) error {
	authoritative, err := DecodeForRead(raw)
	if err != nil {
		return err
	}

	modernFields := make(map[string]json.RawMessage, len(fieldNames)+1)

	modernFields["SchemaVersion"] = fields["SchemaVersion"]

	for _, name := range fieldNames {
		modernFields[lowerName(name)] = fields[lowerName(name)]
	}

	modernRaw, err := json.Marshal(modernFields)
	if err != nil {
		return fmt.Errorf("encode lower-only balance cache view: %w", err)
	}

	modern, err := DecodeForRead(modernRaw)
	if err != nil {
		return fmt.Errorf("decode lower-only balance cache view: %w", err)
	}

	authoritativeCanonical, err := Encode(authoritative, FormatNewOnly)
	if err != nil {
		return fmt.Errorf("encode authoritative balance cache view: %w", err)
	}

	modernCanonical, err := Encode(modern, FormatNewOnly)
	if err != nil {
		return fmt.Errorf("encode lower-only balance cache view: %w", err)
	}

	if !bytes.Equal(authoritativeCanonical, modernCanonical) {
		return errors.New("dual balance cache representations disagree")
	}

	return nil
}

// BuildInventory classifies every supplied entry in key order. Duplicate keys
// are rejected because they make an operational count ambiguous.
func BuildInventory(entries []InventoryEntry) (InventoryReport, error) {
	ordered := append([]InventoryEntry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Key < ordered[j].Key })

	report := InventoryReport{Total: len(ordered)}
	for i, entry := range ordered {
		if entry.Key == "" {
			return InventoryReport{}, fmt.Errorf("balance cache inventory entry %d has an empty key", i)
		}

		if i > 0 && entry.Key == ordered[i-1].Key {
			return InventoryReport{}, fmt.Errorf("duplicate balance cache inventory key %q", entry.Key)
		}

		shape, classifyErr := ClassifyShape(entry.Value)
		switch shape {
		case ShapeLegacy:
			report.Legacy++
		case ShapeDual:
			report.Dual++
		case ShapeNewOnly:
			report.NewOnly++
		case ShapeMixed:
			report.Mixed++
		case ShapeInvalid:
			report.Invalid++
		}

		if shape == ShapeLegacy || shape == ShapeMixed || shape == ShapeInvalid {
			issue := InventoryIssue{Key: entry.Key, Shape: shape}
			if classifyErr != nil {
				issue.Error = classifyErr.Error()
			}

			report.Issues = append(report.Issues, issue)
		}
	}

	return report, nil
}

// FormatReadyForNewOnly reports only the non-empty report's format condition.
// It does not prove a complete key walk, deployment age, or consumer rollout.
func (r InventoryReport) FormatReadyForNewOnly() bool {
	return r.Total > 0 && r.Total == r.Dual+r.NewOnly && r.Legacy == 0 && r.Mixed == 0 && r.Invalid == 0
}
