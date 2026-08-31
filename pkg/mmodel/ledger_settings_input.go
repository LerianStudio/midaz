// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

// LedgerSettingsInput is the request-side projection of LedgerSettings for POST /ledgers.
// Every field is a pointer so an absent JSON key stays distinguishable from an explicitly
// sent zero value ("", 0, false) — the distinction ToSparseMap needs to emit only the keys
// the client actually sent. LedgerSettings itself must stay ==-comparable, so the request
// shape lives in its own type tree rather than on the domain/response type.
//
// The value space of each leaf is enforced by ValidateSettings, the single source of truth
// shared with PATCH /ledgers/{id}/settings; these types therefore carry no validate tags.
type LedgerSettingsInput struct {
	Accounting *AccountingValidationInput `json:"accounting,omitempty"`
	Tracer     *TracerSettingsInput       `json:"tracer,omitempty"`
	Overrides  *OverridePolicyInput       `json:"overrides,omitempty"`
}

// AccountingValidationInput is the request-side projection of AccountingValidation.
type AccountingValidationInput struct {
	ValidateAccountType *bool `json:"validateAccountType,omitempty" example:"false"`
	ValidateRoutes      *bool `json:"validateRoutes,omitempty" example:"false"`
	RequireHolder       *bool `json:"requireHolder,omitempty" example:"false"`
}

// TracerSettingsInput is the request-side projection of TracerSettings.
type TracerSettingsInput struct {
	Mode        *string `json:"mode,omitempty" example:"off"`
	FailPosture *string `json:"failPosture,omitempty" example:"open"`
	TimeoutMs   *int    `json:"timeoutMs,omitempty" example:"250"`
}

// OverridePolicyInput is the request-side projection of OverridePolicy.
type OverridePolicyInput struct {
	AllowFeeSkip    *bool `json:"allowFeeSkip,omitempty" example:"false"`
	AllowTracerSkip *bool `json:"allowTracerSkip,omitempty" example:"false"`
	AllowHolderSkip *bool `json:"allowHolderSkip,omitempty" example:"false"`
}

// ToSparseMap renders only the keys the client actually sent, in the nested shape
// ValidateSettings and ParseLedgerSettings consume. A nil receiver returns nil so the caller
// can distinguish "no settings key in the request" from "an empty settings object".
//
// Each call returns freshly allocated maps, so a caller may mutate the result without
// affecting the receiver or an earlier result.
func (in *LedgerSettingsInput) ToSparseMap() map[string]any {
	if in == nil {
		return nil
	}

	out := make(map[string]any, 3)

	if in.Accounting != nil {
		group := make(map[string]any, 3)
		putSettingsField(group, "validateAccountType", in.Accounting.ValidateAccountType)
		putSettingsField(group, "validateRoutes", in.Accounting.ValidateRoutes)
		putSettingsField(group, "requireHolder", in.Accounting.RequireHolder)
		out["accounting"] = group
	}

	if in.Tracer != nil {
		group := make(map[string]any, 3)
		putSettingsField(group, "mode", in.Tracer.Mode)
		putSettingsField(group, "failPosture", in.Tracer.FailPosture)
		putSettingsField(group, "timeoutMs", in.Tracer.TimeoutMs)
		out["tracer"] = group
	}

	if in.Overrides != nil {
		group := make(map[string]any, 3)
		putSettingsField(group, "allowFeeSkip", in.Overrides.AllowFeeSkip)
		putSettingsField(group, "allowTracerSkip", in.Overrides.AllowTracerSkip)
		putSettingsField(group, "allowHolderSkip", in.Overrides.AllowHolderSkip)
		out["overrides"] = group
	}

	return out
}

// putSettingsField writes key only when the client sent the field. A nil pointer means
// "absent", which must not reach ValidateSettings as a zero value.
func putSettingsField[T any](dst map[string]any, key string, value *T) {
	if value != nil {
		dst[key] = *value
	}
}
