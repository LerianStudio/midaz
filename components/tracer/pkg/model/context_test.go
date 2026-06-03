// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"tracer/internal/testutil"

	"github.com/stretchr/testify/assert"
)

func TestAccountContext_ToMap_NilReceiver(t *testing.T) {
	var acc *AccountContext
	result := acc.ToMap()
	assert.Nil(t, result, "ToMap on nil AccountContext should return nil")
}

func TestAccountContext_ToMap_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(1)
	acc := &AccountContext{
		ID:     id,
		Type:   "checking",
		Status: "active",
	}
	result := acc.ToMap()
	assert.NotNil(t, result)
	assert.Equal(t, id.String(), result["accountId"])
	assert.Equal(t, "checking", result["type"])
	assert.Equal(t, "active", result["status"])
}

func TestMerchantContext_ToMap_NilReceiver(t *testing.T) {
	var m *MerchantContext
	result := m.ToMap()
	assert.Nil(t, result, "ToMap on nil MerchantContext should return nil")
}

func TestMerchantContext_ToMap_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(2)
	m := &MerchantContext{
		ID:       id,
		Name:     "Test Merchant",
		Category: "5411",
		Country:  "US",
	}
	result := m.ToMap()
	assert.NotNil(t, result)
	assert.Equal(t, id.String(), result["merchantId"])
	assert.Equal(t, "Test Merchant", result["name"])
}

func TestSegmentContext_ToMap_NilReceiver(t *testing.T) {
	var s *SegmentContext
	result := s.ToMap()
	assert.Nil(t, result, "ToMap on nil SegmentContext should return nil")
}

func TestSegmentContext_ToMap_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(3)
	s := &SegmentContext{
		ID:   id,
		Name: "Premium",
	}
	result := s.ToMap()
	assert.NotNil(t, result)
	assert.Equal(t, id.String(), result["segmentId"])
	assert.Equal(t, "Premium", result["name"])
}

func TestPortfolioContext_ToMap_NilReceiver(t *testing.T) {
	var p *PortfolioContext
	result := p.ToMap()
	assert.Nil(t, result, "ToMap on nil PortfolioContext should return nil")
}

func TestPortfolioContext_ToMap_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(4)
	p := &PortfolioContext{
		ID:   id,
		Name: "Growth",
	}
	result := p.ToMap()
	assert.NotNil(t, result)
	assert.Equal(t, id.String(), result["portfolioId"])
	assert.Equal(t, "Growth", result["name"])
}
