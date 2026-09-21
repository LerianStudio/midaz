// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertPreservesMappingOrderAndScalarTypes(t *testing.T) {
	t.Parallel()

	input := bytes.NewBufferString(`openapi: 3.1.0
info:
  title: Midaz
enabled: true
count: 2
tags:
  - one
  - two
`)
	var output bytes.Buffer

	require.NoError(t, convert(input, &output))
	require.Equal(t, `{
  "openapi": "3.1.0",
  "info": {
    "title": "Midaz"
  },
  "enabled": true,
  "count": 2,
  "tags": [
    "one",
    "two"
  ]
}
`, output.String())
}

func TestConvertRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := convert(bytes.NewBufferString("first: true\n---\nsecond: true\n"), &output)

	require.ErrorContains(t, err, "expected exactly one YAML document")
}

func TestConvertRejectsDuplicateMappingKeys(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := convert(bytes.NewBufferString("paths:\n  /v1/assets: first\n  /v1/assets: second\n"), &output)

	require.ErrorContains(t, err, `duplicate mapping key "/v1/assets"`)
}
