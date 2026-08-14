// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import "github.com/google/uuid"

// AccountStatus holds the code and description of an account's status as returned by Midaz.
type AccountStatus struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// Account represents a Midaz account returned by the onboarding API.
// Fields SegmentID and PortfolioID are optional (nullable) in Midaz responses.
type Account struct {
	ID          string         `json:"id"`
	Alias       string         `json:"alias"`
	SegmentID   *uuid.UUID     `json:"segmentId,omitempty"`
	PortfolioID *uuid.UUID     `json:"portfolioId,omitempty"`
	Status      *AccountStatus `json:"status,omitempty"`
	Type        string         `json:"type"`
}
