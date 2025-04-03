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

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}
