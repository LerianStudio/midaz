package models

import (
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// Status represents the status of an entity in the Midaz system.
// It contains a status code and an optional description providing additional context.
// Status is used across various models to indicate the current state of resources.
type Status struct {
	// Code is the status code identifier (e.g., "active", "pending", "closed")
	Code string `json:"code"`

	// Description provides optional additional context about the status
	Description *string `json:"description,omitempty"`
}

// NewStatus creates a new Status with the given code.
// This is a convenience constructor for creating Status objects.
//
// Parameters:
//   - code: The status code to set (e.g., "active", "pending", "closed")
//
// Returns:
//   - A new Status instance with the specified code
func NewStatus(code string) Status {
	return Status{
		Code: code,
	}
}

// WithDescription adds a description to the status.
// This is a fluent-style method that returns the modified Status.
//
// Parameters:
//   - description: The description text to add to the status
//
// Returns:
//   - The modified Status instance with the added description
func (s Status) WithDescription(description string) Status {
	s.Description = &description
	return s
}

// IsEmpty returns true if the status is empty.
// A status is considered empty if it has no code and no description.
//
// Returns:
//   - true if the status is empty, false otherwise
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// ToMmodelStatus converts an SDK Status to an mmodel Status (internal use only).
// This method is used for internal SDK operations when interfacing with the backend.
//
// Returns:
//   - An mmodel.Status instance with the same values as this Status
func (s Status) ToMmodelStatus() mmodel.Status {
	return mmodel.Status{
		Code:        s.Code,
		Description: s.Description,
	}
}

// FromMmodelStatus converts an mmodel Status to an SDK Status (internal use only).
// This function is used for internal SDK operations when processing responses from the backend.
//
// Parameters:
//   - modelStatus: The mmodel.Status to convert
//
// Returns:
//   - A models.Status instance with the same values as the input mmodel.Status
func FromMmodelStatus(modelStatus mmodel.Status) Status {
	return Status{
		Code:        modelStatus.Code,
		Description: modelStatus.Description,
	}
}

// Address represents a physical address.
// This structure is used across various models where address information is required,
// such as for organizations or account holders.
type Address struct {
	// Line1 is the primary address line (e.g., street number and name)
	Line1 string `json:"line1"`

	// Line2 is an optional secondary address line (e.g., apartment or suite number)
	Line2 *string `json:"line2,omitempty"`

	// ZipCode is the postal or ZIP code
	ZipCode string `json:"zipCode"`

	// City is the city or locality name
	City string `json:"city"`

	// State is the state, province, or region
	State string `json:"state"`

	// Country is the country, typically using ISO country codes
	Country string `json:"country"`
}

// NewAddress creates a new Address with the given parameters.
// This is a convenience constructor for creating Address objects with required fields.
//
// Parameters:
//   - line1: The primary address line
//   - zipCode: The postal or ZIP code
//   - city: The city or locality name
//   - state: The state, province, or region
//   - country: The country code
//
// Returns:
//   - A new Address instance with the specified fields
func NewAddress(line1, zipCode, city, state, country string) Address {
	return Address{
		Line1:   line1,
		ZipCode: zipCode,
		City:    city,
		State:   state,
		Country: country,
	}
}

// WithLine2 adds the optional Line2 field to the address.
// This is a fluent-style method that returns the modified Address.
//
// Parameters:
//   - line2: The secondary address line to add
//
// Returns:
//   - The modified Address instance with the added Line2
func (a Address) WithLine2(line2 string) Address {
	a.Line2 = &line2
	return a
}

// ToMmodelAddress converts an SDK Address to an mmodel Address (internal use only).
// This method is used for internal SDK operations when interfacing with the backend.
//
// Returns:
//   - An mmodel.Address instance with the same values as this Address
func (a Address) ToMmodelAddress() mmodel.Address {
	return mmodel.Address{
		Line1:   a.Line1,
		Line2:   a.Line2,
		ZipCode: a.ZipCode,
		City:    a.City,
		State:   a.State,
		Country: a.Country,
	}
}

// FromMmodelAddress converts an mmodel Address to an SDK Address (internal use only).
// This function is used for internal SDK operations when processing responses from the backend.
//
// Parameters:
//   - modelAddress: The mmodel.Address to convert
//
// Returns:
//   - A models.Address instance with the same values as the input mmodel.Address
func FromMmodelAddress(modelAddress mmodel.Address) Address {
	return Address{
		Line1:   modelAddress.Line1,
		Line2:   modelAddress.Line2,
		ZipCode: modelAddress.ZipCode,
		City:    modelAddress.City,
		State:   modelAddress.State,
		Country: modelAddress.Country,
	}
}

// Pagination represents pagination information for list operations.
// This structure is used in list responses to provide context about the pagination state
// and to help with navigating through paginated results.
type Pagination struct {
	// Limit is the number of items per page
	Limit int `json:"limit"`

	// Offset is the starting position for the current page
	Offset int `json:"offset"`

	// Total is the total number of items available across all pages
	Total int `json:"total"`

	// PrevCursor is the cursor for the previous page (for cursor-based pagination)
	PrevCursor string `json:"prevCursor,omitempty"`

	// NextCursor is the cursor for the next page (for cursor-based pagination)
	NextCursor string `json:"nextCursor,omitempty"`
}

// ListOptions represents the common options for list operations.
// This structure is used to specify filtering, pagination, and sorting parameters
// when retrieving lists of resources from the Midaz API.
type ListOptions struct {
	// Limit is the maximum number of items to return per page
	Limit int `json:"limit,omitempty"`

	// Offset is the starting position for pagination
	Offset int `json:"offset,omitempty"`

	// Filters are additional filters to apply to the query
	// The map keys are filter names and values are the filter criteria
	Filters map[string]string `json:"filters,omitempty"`

	// OrderBy specifies the field to order results by
	OrderBy string `json:"orderBy,omitempty"`

	// OrderDirection is the order direction ("asc" for ascending or "desc" for descending)
	OrderDirection string `json:"orderDirection,omitempty"`

	// Page is the page number to return (when using page-based pagination)
	// This is kept for backward compatibility
	Page int `json:"page,omitempty"`

	// Cursor is the cursor for pagination (when using cursor-based pagination)
	// This is kept for backward compatibility
	Cursor string `json:"cursor,omitempty"`

	// StartDate and EndDate for filtering by date range
	// These should be in ISO 8601 format (YYYY-MM-DD)
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`

	// AdditionalParams contains additional parameters that are specific to certain endpoints
	// These parameters are not serialized to JSON but are used when making API requests
	AdditionalParams map[string]string `json:"-"`
}

// ToQueryParams converts ListOptions to a map of query parameters.
// This method transforms the ListOptions structure into a format suitable
// for use as URL query parameters in API requests.
//
// Returns:
//   - A map of string key-value pairs representing the query parameters
func (o *ListOptions) ToQueryParams() map[string]string {
	params := make(map[string]string)

	// Add pagination parameters
	o.addPaginationParams(params)

	// Add filtering parameters
	o.addFilteringParams(params)

	// Add sorting parameters
	o.addSortingParams(params)

	// Add date range parameters
	o.addDateRangeParams(params)

	// Add additional parameters
	o.addAdditionalParams(params)

	return params
}

// addPaginationParams adds pagination-related parameters to the query parameters map.
// This is an internal helper method used by ToQueryParams.
//
// Parameters:
//   - params: The map to add the pagination parameters to
func (o *ListOptions) addPaginationParams(params map[string]string) {
	if o.Limit > 0 {
		params["limit"] = fmt.Sprintf("%d", o.Limit)
	}

	if o.Offset > 0 {
		params["offset"] = fmt.Sprintf("%d", o.Offset)
	}

	// These are kept for backward compatibility
	if o.Page > 0 {
		params["page"] = fmt.Sprintf("%d", o.Page)
	}

	if o.Cursor != "" {
		params["cursor"] = o.Cursor
	}
}

// addFilteringParams adds filter-related parameters to the query parameters map.
// This is an internal helper method used by ToQueryParams.
//
// Parameters:
//   - params: The map to add the filter parameters to
func (o *ListOptions) addFilteringParams(params map[string]string) {
	if o.Filters != nil {
		for k, v := range o.Filters {
			// If the filter value is empty, skip it
			if v == "" {
				continue
			}
			params[k] = v
		}
	}
}

// addSortingParams adds sorting-related parameters to the query parameters map.
// This is an internal helper method used by ToQueryParams.
//
// Parameters:
//   - params: The map to add the sorting parameters to
func (o *ListOptions) addSortingParams(params map[string]string) {
	if o.OrderBy != "" {
		params["orderBy"] = o.OrderBy
	}

	if o.OrderDirection != "" {
		params["orderDirection"] = o.OrderDirection
	}
}

// addDateRangeParams adds date range parameters to the query parameters map.
// This is an internal helper method used by ToQueryParams.
//
// Parameters:
//   - params: The map to add the date range parameters to
func (o *ListOptions) addDateRangeParams(params map[string]string) {
	if o.StartDate != "" {
		params["startDate"] = o.StartDate
	}

	if o.EndDate != "" {
		params["endDate"] = o.EndDate
	}
}

// addAdditionalParams adds additional parameters to the query parameters map.
// This is an internal helper method used by ToQueryParams.
//
// Parameters:
//   - params: The map to add the additional parameters to
func (o *ListOptions) addAdditionalParams(params map[string]string) {
	if o.AdditionalParams != nil {
		for k, v := range o.AdditionalParams {
			params[k] = v
		}
	}
}

// Metadata is a map of key-value pairs that can be attached to resources.
// It allows for storing arbitrary data with resources in a flexible way.
type Metadata map[string]any

// Timestamps represents common timestamp fields for resources.
// This structure is embedded in many models to provide standard
// creation, update, and deletion timestamps.
type Timestamps struct {
	// CreatedAt is the timestamp when the resource was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the resource was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// DeletedAt is the timestamp when the resource was deleted (if applicable)
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

// BaseResponse represents the common fields in all API responses.
// This structure is embedded in response models to provide standard
// fields that are present in all API responses.
type BaseResponse struct {
	// RequestID is a unique identifier for the API request
	// This can be used for troubleshooting and support
	RequestID string `json:"requestId,omitempty"`
}

// ListResponse is a generic response for list operations.
// It contains a collection of items along with pagination information.
type ListResponse[T any] struct {
	// Embedding BaseResponse to include common response fields
	BaseResponse

	// Items is the collection of resources returned by the list operation
	Items []T `json:"items"`

	// Pagination contains information about the pagination state
	Pagination Pagination `json:"pagination,omitempty"`
}

// ErrorResponse represents an error response from the API.
// This structure is used to parse and represent error responses
// returned by the Midaz API.
type ErrorResponse struct {
	// Error is the error message
	Error string `json:"error"`

	// Code is the error code for programmatic handling
	Code string `json:"code,omitempty"`

	// Details contains additional information about the error
	Details map[string]any `json:"details,omitempty"`
}

// ObjectWithMetadata is an object that has metadata.
// This interface is implemented by resources that support
// attaching arbitrary metadata.
type ObjectWithMetadata struct {
	// Metadata is a map of key-value pairs associated with the object
	Metadata map[string]any `json:"metadata,omitempty"`
}

// HasMetadata returns true if the object has metadata.
// This method checks if the Metadata field is non-nil and non-empty.
//
// Returns:
//   - true if the object has metadata, false otherwise
func (o *ObjectWithMetadata) HasMetadata() bool {
	return o.Metadata != nil && len(o.Metadata) > 0
}
