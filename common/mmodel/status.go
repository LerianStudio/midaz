package mmodel

// StatusAllow structure for marshaling/unmarshalling JSON.
type StatusAllow struct {
	Code           string  `json:"code" validate:"max=100"`
	Description    *string `json:"description" validate:"omitempty,max=256"`
	AllowSending   *bool   `json:"allowSending"`
	AllowReceiving *bool   `json:"allowReceiving"`
}

// Status structure for marshaling/unmarshalling JSON.
type Status struct {
	Code        string  `json:"code" validate:"max=100"`
	Description *string `json:"description" validate:"omitempty,max=256"`
}
