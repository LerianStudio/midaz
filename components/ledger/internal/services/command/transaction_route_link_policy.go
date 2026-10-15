// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

// TransactionRouteLinkPolicy is what the operation-route lists of a transaction-route PATCH mean
// on the contract that received it. The transport shell picks it; the use case never infers it.
type TransactionRouteLinkPolicy int

const (
	// LinksFullSetV1 is the /v1 contract: operationRoutes is every link of the route. A link that
	// stays retains its optionality, a new one is required and an omitted one is removed. It never
	// changes the optionality of a link.
	LinksFullSetV1 TransactionRouteLinkPolicy = iota

	// LinksMergePatchV2 is the /v2 contract, a JSON merge patch per list: operationRoutes is the
	// required links and optionalOperationRoutes the optional ones. A list that is absent keeps
	// its current links; a list that is present replaces them, and an empty one removes them.
	LinksMergePatchV2
)
