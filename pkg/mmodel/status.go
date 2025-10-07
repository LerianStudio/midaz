// Package mmodel defines domain models for the Midaz platform.
// This file contains Status-related models.
package mmodel

// Status structure for marshaling/unmarshalling JSON.
//
// swagger:model Status
// @Description Entity status information with a standardized code and optional description. Common status codes include: ACTIVE, INACTIVE, PENDING, SUSPENDED, DELETED.
type Status struct {
	// Status code identifier, common values include: ACTIVE, INACTIVE, PENDING, SUSPENDED, DELETED
	Code string `json:"code" validate:"max=100" example:"ACTIVE" maxLength:"100" enum:"ACTIVE,INACTIVE,PENDING,SUSPENDED,DELETED"`

	// Optional human-readable description of the status
	Description *string `json:"description" validate:"omitempty,max=256" example:"Active status" maxLength:"256"`
} // @name Status

// IsEmpty determines if a Status has no data in any of its fields.
//
// This method checks whether the status is completely empty, which is useful for
// validation logic to determine if a status was provided in a request or should
// use default values.
//
// A status is considered empty if:
//   - Code is an empty string
//   - Description is nil
//
// Returns:
//   - true if both fields are empty/nil, false if any field has a value
//
// Example:
//
//	emptyStatus := Status{}
//	if emptyStatus.IsEmpty() {
//	    // Use default status (ACTIVE)
//	}
//
//	providedStatus := Status{Code: "INACTIVE"}
//	if !providedStatus.IsEmpty() {
//	    // Use provided status
//	}
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}
