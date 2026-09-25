// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

// ResourceProfile describes deployment work ceilings, not asset precision.
// Both peers must select the same profile before activating the integration.
type ResourceProfile struct {
	Facts           Limits
	MaxBodyBytes    int
	MaxReservations int
}

// DefaultResourceProfile is an explicit bootstrap baseline, not a measured SLO
// or an implicit limit in decimal arithmetic. No amount is rounded to fit it.
func DefaultResourceProfile() ResourceProfile {
	return ResourceProfile{Facts: Limits{MaxAccounts: 128, MaxEntries: 512, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxBodyBytes: 1 << 20, MaxReservations: 1024}
}

func (p ResourceProfile) Validate() error {
	if err := p.Facts.Validate(); err != nil {
		return err
	}

	if p.MaxBodyBytes <= 0 || p.MaxReservations <= 0 {
		return invalid("resource profile")
	}

	return nil
}
