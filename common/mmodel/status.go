package mmodel

// Status structure for marshaling/unmarshalling JSON.
//
// swagger:model Status
// @Description Status is the struct designed to store the status data of an entity.
type Status struct {
	Code        string  `json:"code" validate:"max=100" example:"ACTIVE"`
	Description *string `json:"description" validate:"omitempty,max=256" example:"Active status"`
} // @name Status

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}
