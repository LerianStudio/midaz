// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"github.com/LerianStudio/lib-streaming/v3/billing"
)

// ActiveAccountBillable pairs an active-account billing payload with the
// internal account ID it was derived from. The account ID is the ce-subject at
// the emit site, kept separate from the payload's SubscriptionId (the billing
// customer: the tenant when multi-tenant is enabled, otherwise the organization).
//
// The active-account event uses the raw billing.BillablePayload wire type
// (Confluent-Protobuf), so it deliberately does not follow the
// Definition/Payload/ToEmitRequest convention of the other events in this
// package.
type ActiveAccountBillable struct {
	// AccountID is the internal account the payload was derived from. It is the
	// event's ce-subject, distinct from Payload.SubscriptionId.
	AccountID string

	// Payload is the Confluent-Protobuf billing payload for one active account.
	Payload billing.BillablePayload
}
