package in

import "encoding/json"

// LogsResponse is a struct to encapsulate audit logs response
//
// swagger:model LogsResponse
// @Description LogsResponse is the response with audit log values
type LogsResponse struct {
	TreeID int64  `json:"tree_id"`
	Leaves []Leaf `json:"leaves"`
} // @name LogsResponse

// Leaf encapsulates audit log values
//
// swagger:model Leaf
// @Description Leaf stores each audit log
type Leaf struct {
	LeafID string          `json:"leaf_id"`
	Body   json.RawMessage `json:"body"`
} // @Name Leaf

// HashValidationResponse encapsulates auditing validation results
//
// swagger:model HashValidationResponse
// @Description HashValidationResponse show if any of the logs has been tampered
type HashValidationResponse struct {
	AuditID        string `json:"auditId"`
	ExpectedHash   string `json:"expectedHash"`
	CalculatedHash string `json:"calculatedHash"`
	IsTampered    bool   `json:"isTampered"`
} // @Name HashValidationResponse
