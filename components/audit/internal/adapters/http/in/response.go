package in

import "encoding/json"

// LogsResponse is a struct to encapsulate audit logs response
type LogsResponse struct {
	TreeID int64  `json:"tree_id"`
	Leaves []Leaf `json:"leaves"`
}

// Leaf encapsulates audit log values
type Leaf struct {
	LeafID string          `json:"leaf_id"`
	Body   json.RawMessage `json:"body"`
}

// HashValidationResponse encapsulates auditing validation results
type HashValidationResponse struct {
	AuditID        string `json:"auditId"`
	ExpectedHash   string `json:"expectedHash"`
	CalculatedHash string `json:"calculatedHash"`
	WasTempered    bool   `json:"wasTempered"`
}
