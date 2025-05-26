package fixtures

import (
	"encoding/json"
	"time"
)

// TestData provides common test data for various scenarios
type TestData struct {
	ValidTransactionJSON   string
	InvalidTransactionJSON string
	ValidAccountJSON       string
	LargeDatasetSize       int
}

// GetTestData returns pre-configured test data
func GetTestData() *TestData {
	return &TestData{
		ValidTransactionJSON: `{
			"id": "123e4567-e89b-12d3-a456-426614174000",
			"description": "Payment for services",
			"amount": 10000,
			"currency": "USD",
			"status": "pending",
			"metadata": {
				"invoice_id": "INV-2024-001",
				"customer_id": "CUST-123"
			}
		}`,
		InvalidTransactionJSON: `{
			"amount": "not-a-number",
			"currency": 123
		}`,
		ValidAccountJSON: `{
			"id": "acc_123",
			"name": "Operating Account",
			"type": "asset",
			"balance": 50000,
			"currency": "USD"
		}`,
		LargeDatasetSize: 1000,
	}
}

// GenerateTimeRange creates a time range for testing
func GenerateTimeRange(days int) (start, end time.Time) {
	end = time.Now()
	start = end.AddDate(0, 0, -days)
	return
}

// GenerateJSONArray creates a JSON array of test objects
func GenerateJSONArray(objectJSON string, count int) string {
	objects := make([]json.RawMessage, count)
	for i := 0; i < count; i++ {
		objects[i] = json.RawMessage(objectJSON)
	}
	
	result, _ := json.Marshal(objects)
	return string(result)
}