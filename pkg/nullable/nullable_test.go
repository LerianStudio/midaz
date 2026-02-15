package nullable

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNullable_UnmarshalJSON_NotProvided(t *testing.T) {
	type TestStruct struct {
		Name      string           `json:"name"`
		SegmentID Nullable[string] `json:"segmentId"`
	}

	// JSON without the segmentId field
	jsonData := `{"name": "Test Account"}`

	var result TestStruct

	err := json.Unmarshal([]byte(jsonData), &result)

	require.NoError(t, err)
	assert.Equal(t, "Test Account", result.Name)
	assert.False(t, result.SegmentID.IsSet, "SegmentID should not be set when absent from JSON")
	assert.False(t, result.SegmentID.IsNull, "SegmentID should not be null when absent from JSON")
	assert.False(t, result.SegmentID.ShouldUpdate(), "ShouldUpdate should be false when field is absent")
}

func TestNullable_UnmarshalJSON_ExplicitNull(t *testing.T) {
	type TestStruct struct {
		Name      string           `json:"name"`
		SegmentID Nullable[string] `json:"segmentId"`
	}

	// JSON with explicit null for segmentId
	jsonData := `{"name": "Test Account", "segmentId": null}`

	var result TestStruct

	err := json.Unmarshal([]byte(jsonData), &result)

	require.NoError(t, err)
	assert.Equal(t, "Test Account", result.Name)
	assert.True(t, result.SegmentID.IsSet, "SegmentID should be set when explicitly null")
	assert.True(t, result.SegmentID.IsNull, "SegmentID should be null when explicitly null")
	assert.True(t, result.SegmentID.ShouldUpdate(), "ShouldUpdate should be true when field is explicitly null")
	assert.True(t, result.SegmentID.ShouldSetNull(), "ShouldSetNull should be true when field is explicitly null")
}

func TestNullable_UnmarshalJSON_WithValue(t *testing.T) {
	type TestStruct struct {
		Name      string           `json:"name"`
		SegmentID Nullable[string] `json:"segmentId"`
	}

	// JSON with a value for segmentId
	jsonData := `{"name": "Test Account", "segmentId": "550e8400-e29b-41d4-a716-446655440000"}`

	var result TestStruct

	err := json.Unmarshal([]byte(jsonData), &result)

	require.NoError(t, err)
	assert.Equal(t, "Test Account", result.Name)
	assert.True(t, result.SegmentID.IsSet, "SegmentID should be set when has value")
	assert.False(t, result.SegmentID.IsNull, "SegmentID should not be null when has value")
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", result.SegmentID.Value)
	assert.True(t, result.SegmentID.ShouldUpdate(), "ShouldUpdate should be true when field has value")
	assert.False(t, result.SegmentID.ShouldSetNull(), "ShouldSetNull should be false when field has value")
}

func TestNullable_Get(t *testing.T) {
	// Test with value
	withValue := Set("test-value")

	val, ok := withValue.Get()
	assert.True(t, ok)
	assert.Equal(t, "test-value", val)

	// Test with null
	withNull := Null[string]()

	val, ok = withNull.Get()
	assert.False(t, ok)
	assert.Equal(t, "", val)

	// Test unset
	unset := Unset[string]()

	val, ok = unset.Get()
	assert.False(t, ok)
	assert.Equal(t, "", val)
}

func TestNullable_ToPointer(t *testing.T) {
	// Test with value
	withValue := Set("test-value")

	ptr := withValue.ToPointer()
	require.NotNil(t, ptr)
	assert.Equal(t, "test-value", *ptr)

	// Test with null
	withNull := Null[string]()

	ptr = withNull.ToPointer()
	assert.Nil(t, ptr)

	// Test unset
	unset := Unset[string]()

	ptr = unset.ToPointer()
	assert.Nil(t, ptr)
}

func TestNullable_MarshalJSON(t *testing.T) {
	type TestStruct struct {
		Name      string           `json:"name"`
		SegmentID Nullable[string] `json:"segmentId"`
	}

	// Test with value
	withValue := TestStruct{Name: "Test", SegmentID: Set("test-id")}

	data, err := json.Marshal(withValue)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"Test","segmentId":"test-id"}`, string(data))

	// Test with null
	withNull := TestStruct{Name: "Test", SegmentID: Null[string]()}

	data, err = json.Marshal(withNull)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"Test","segmentId":null}`, string(data))
}

func TestNullable_Bool(t *testing.T) {
	type TestStruct struct {
		Blocked Nullable[bool] `json:"blocked"`
	}

	// Test explicit false (important: false should be different from null/absent)
	jsonData := `{"blocked": false}`

	var result TestStruct

	err := json.Unmarshal([]byte(jsonData), &result)

	require.NoError(t, err)
	assert.True(t, result.Blocked.IsSet)
	assert.False(t, result.Blocked.IsNull)
	assert.False(t, result.Blocked.Value)
	assert.True(t, result.Blocked.ShouldUpdate())
}

func TestNullable_GetOrDefault(t *testing.T) {
	// Test with value
	withValue := Set("actual-value")
	assert.Equal(t, "actual-value", withValue.GetOrDefault("default"))

	// Test with null
	withNull := Null[string]()
	assert.Equal(t, "default", withNull.GetOrDefault("default"))

	// Test unset
	unset := Unset[string]()
	assert.Equal(t, "default", unset.GetOrDefault("default"))
}

// TestPatchScenario simulates the real-world PATCH scenario from issue #1778
func TestPatchScenario_UnlinkAccountFromSegment(t *testing.T) {
	type UpdateAccountInput struct {
		Name      string           `json:"name"`
		SegmentID Nullable[string] `json:"segmentId"`
	}

	// Scenario: User wants to unlink account from segment by sending null
	jsonData := `{"segmentId": null}`

	var input UpdateAccountInput

	err := json.Unmarshal([]byte(jsonData), &input)

	require.NoError(t, err)

	// The key assertion: we should know to SET segment_id to NULL
	assert.True(t, input.SegmentID.ShouldUpdate(), "Should update segment_id in DB")
	assert.True(t, input.SegmentID.ShouldSetNull(), "Should set segment_id to NULL")
}

// TestPatchScenario_PartialUpdate simulates updating only specific fields
func TestPatchScenario_PartialUpdate(t *testing.T) {
	type UpdateAccountInput struct {
		Name        string           `json:"name"`
		SegmentID   Nullable[string] `json:"segmentId"`
		PortfolioID Nullable[string] `json:"portfolioId"`
		EntityID    Nullable[string] `json:"entityId"`
	}

	// Scenario: User wants to update only the name, keep everything else
	jsonData := `{"name": "New Account Name"}`

	var input UpdateAccountInput

	err := json.Unmarshal([]byte(jsonData), &input)

	require.NoError(t, err)

	assert.Equal(t, "New Account Name", input.Name)
	assert.False(t, input.SegmentID.ShouldUpdate(), "Should NOT update segment_id")
	assert.False(t, input.PortfolioID.ShouldUpdate(), "Should NOT update portfolio_id")
	assert.False(t, input.EntityID.ShouldUpdate(), "Should NOT update entity_id")
}
