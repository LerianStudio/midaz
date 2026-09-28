// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingPackageModel_EventFilterStatusIsUpperCased(t *testing.T) {
	t.Parallel()

	var written BillingPackageMongoDBModel
	written.FromEntity(&model.BillingPackage{
		EventFilter: &model.EventFilter{TransactionRoute: "route", Status: "approved"},
	})
	assert.Equal(t, constant.APPROVED, written.EventFilter.Status, "a write stores the upper-case status")

	legacy := BillingPackageMongoDBModel{
		EventFilter: &EventFilterModel{TransactionRoute: "route", Status: "approved"},
	}

	bp, err := legacy.ToEntity()
	require.NoError(t, err)
	assert.Equal(t, constant.APPROVED, bp.EventFilter.Status, "a stored lower-case row reads upper-case")
}
