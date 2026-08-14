// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bsondecimal

import (
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Decimal is a wrapper for decimal.Decimal
type Decimal struct {
	decimal.Decimal
}

// MarshalBSONValue serializes the decimal to BSON as its canonical string form,
// preserving exact precision (no float conversion). Implements the mongo-driver
// v2 bson.ValueMarshaler interface: the type tag is returned as a raw byte.
func (d *Decimal) MarshalBSONValue() (byte, []byte, error) {
	t, data, err := bson.MarshalValue(d.String())

	return byte(t), data, err
}

// UnmarshalBSONValue deserializes the decimal from its BSON string form.
// Implements the mongo-driver v2 bson.ValueUnmarshaler interface: the type tag
// arrives as a raw byte and is converted back to bson.Type for UnmarshalValue.
func (d *Decimal) UnmarshalBSONValue(t byte, data []byte) error {
	var s string
	if err := bson.UnmarshalValue(bson.Type(t), data, &s); err != nil {
		return err
	}

	dec, err := decimal.NewFromString(s)
	if err != nil {
		return err
	}

	d.Decimal = dec

	return nil
}
