package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// SegmentsService defines the interface for segment-related operations.
// It provides methods to create, read, update, and delete segments
// within a portfolio, ledger, and organization.
type SegmentsService interface {
	// ListSegments retrieves a paginated list of segments for a portfolio with optional filters.
	// The organizationID, ledgerID, and portfolioID parameters specify which organization, ledger, and portfolio to query.
	// The opts parameter can be used to specify pagination, sorting, and filtering options.
	// Returns a ListResponse containing the segments and pagination information, or an error if the operation fails.
	ListSegments(ctx context.Context, organizationID, ledgerID, portfolioID string, opts *models.ListOptions) (*models.ListResponse[models.Segment], error)

	// GetSegment retrieves a specific segment by its ID.
	// The organizationID, ledgerID, and portfolioID parameters specify which organization, ledger, and portfolio the segment belongs to.
	// The id parameter is the unique identifier of the segment to retrieve.
	// Returns the segment if found, or an error if the operation fails or the segment doesn't exist.
	GetSegment(ctx context.Context, organizationID, ledgerID, portfolioID, id string) (*models.Segment, error)

	// CreateSegment creates a new segment in the specified portfolio.
	//
	// Segments allow for further categorization and grouping of accounts or other entities
	// within a portfolio, enabling more detailed reporting and management.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//   - organizationID: The ID of the organization where the segment will be created.
	//     Must be a valid organization ID.
	//   - ledgerID: The ID of the ledger where the segment will be created.
	//     Must be a valid ledger ID within the specified organization.
	//   - portfolioID: The ID of the portfolio where the segment will be created.
	//     Must be a valid portfolio ID within the specified ledger.
	//   - input: The segment details, including required fields:
	//     - Name: The human-readable name of the segment
	//     Optional fields include:
	//     - Status: The initial status (defaults to ACTIVE if not specified)
	//     - Metadata: Additional custom information about the segment
	//
	// Returns:
	//   - *models.Segment: The created segment if successful, containing the segment ID,
	//     name, and other properties.
	//   - error: An error if the operation fails. Possible errors include:
	//     - Invalid input (missing required fields or invalid values)
	//     - Authentication failure (invalid auth token)
	//     - Authorization failure (insufficient permissions)
	//     - Resource not found (invalid organization, ledger, or portfolio ID)
	//     - Network or server errors
	//
	// Example - Creating a basic segment:
	//
	//	// Create a simple segment with just required fields
	//	segment, err := segmentsService.CreateSegment(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    "portfolio-789",
	//	    models.NewCreateSegmentInput("North America Region"),
	//	)
	//
	//	if err != nil {
	//	    log.Fatalf("Failed to create segment: %v", err)
	//	}
	//
	//	// Use the segment
	//	fmt.Printf("Segment created: %s (status: %s)\n",
	//	    segment.ID, segment.Status.Code)
	//
	// Example - Creating a segment with metadata:
	//
	//	// Create a segment with custom metadata
	//	input := models.NewCreateSegmentInput("EMEA Region").
	//	    WithStatus(models.StatusActive).
	//	    WithMetadata(map[string]any{
	//	        "regionCode": "EMEA",
	//	        "countries": []string{"UK", "France", "Germany", "Italy"},
	//	        "headquarters": "London",
	//	        "manager": "John Smith",
	//	    })
	//
	//	segment, err := segmentsService.CreateSegment(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    "portfolio-789",
	//	    input,
	//	)
	//
	//	if err != nil {
	//	    log.Fatalf("Failed to create segment: %v", err)
	//	}
	//
	//	// Use the segment
	//	fmt.Printf("Segment created: %s\n", segment.ID)
	//	fmt.Printf("Name: %s\n", segment.Name)
	CreateSegment(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error)

	// UpdateSegment updates an existing segment.
	// The organizationID, ledgerID, and portfolioID parameters specify which organization, ledger, and portfolio the segment belongs to.
	// The id parameter is the unique identifier of the segment to update.
	// The input parameter contains the segment details to update, such as name, description, or status.
	// Returns the updated segment, or an error if the operation fails.
	UpdateSegment(ctx context.Context, organizationID, ledgerID, portfolioID, id string, input *models.UpdateSegmentInput) (*models.Segment, error)

	// DeleteSegment deletes a segment.
	// The organizationID, ledgerID, and portfolioID parameters specify which organization, ledger, and portfolio the segment belongs to.
	// The id parameter is the unique identifier of the segment to delete.
	// Returns an error if the operation fails.
	DeleteSegment(ctx context.Context, organizationID, ledgerID, portfolioID, id string) error
}

// segmentsEntity implements the SegmentsService interface.
// It handles the communication with the Midaz API for segment-related operations.
type segmentsEntity struct {
	httpClient *http.Client
	authToken  string
	baseURLs   map[string]string
}

// NewSegmentsEntity creates a new segments entity.
//
// Parameters:
//   - httpClient: The HTTP client used for API requests. Can be configured with custom timeouts
//     and transport options. If nil, a default client will be used.
//   - authToken: The authentication token for API authorization. Must be a valid JWT token
//     issued by the Midaz authentication service.
//   - baseURLs: Map of service names to base URLs. Must include an "onboarding" key with
//     the URL of the onboarding service (e.g., "https://api.midaz.io/v1").
//
// Returns:
//   - SegmentsService: An implementation of the SegmentsService interface that provides
//     methods for creating, retrieving, updating, and managing segments.
//
// Example:
//
//	// Create a segments entity with default HTTP client
//	segmentsEntity := entities.NewSegmentsEntity(
//	    &http.Client{Timeout: 30 * time.Second},
//	    "your-auth-token",
//	    map[string]string{"onboarding": "https://api.midaz.io/v1"},
//	)
//
//	// Use the entity to retrieve segments
//	segments, err := segmentsEntity.ListSegments(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    "portfolio-789",
//	    nil, // No pagination options
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to retrieve segments: %v", err)
//	}
//
//	fmt.Printf("Retrieved %d segments\n", len(segments.Items))
func NewSegmentsEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) SegmentsService {
	return &segmentsEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// ListSegments lists all segments in a portfolio with optional filters.
// The organizationID, ledgerID, and portfolioID parameters specify which organization, ledger, and portfolio to query.
// The opts parameter can be used to specify pagination, sorting, and filtering options.
// Returns a ListResponse containing the segments and pagination information, or an error if the operation fails.
func (e *segmentsEntity) ListSegments(
	ctx context.Context,
	organizationID, ledgerID, portfolioID string,
	opts *models.ListOptions,
) (*models.ListResponse[models.Segment], error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if portfolioID == "" {
		return nil, fmt.Errorf("portfolio ID is required")
	}

	url := e.buildURL(organizationID, ledgerID, portfolioID, "")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	// Add query parameters if provided
	if opts != nil {
		q := req.URL.Query()

		for key, value := range opts.ToQueryParams() {
			q.Add(key, value)
		}

		req.URL.RawQuery = q.Encode()
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check for error status codes
	if resp.StatusCode >= 400 {
		var errorResponse map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("server error: %v", errorResponse)
	}

	var response models.ListResponse[models.Segment]

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetSegment gets a segment by ID.
// The organizationID, ledgerID, and portfolioID parameters specify which organization, ledger, and portfolio the segment belongs to.
// The id parameter is the unique identifier of the segment to retrieve.
// Returns the segment if found, or an error if the operation fails or the segment doesn't exist.
func (e *segmentsEntity) GetSegment(
	ctx context.Context,
	organizationID, ledgerID, portfolioID, id string,
) (*models.Segment, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if portfolioID == "" {
		return nil, fmt.Errorf("portfolio ID is required")
	}

	if id == "" {
		return nil, fmt.Errorf("segment ID is required")
	}

	url := e.buildURL(organizationID, ledgerID, portfolioID, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check for error status codes
	if resp.StatusCode >= 400 {
		var errorResponse map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("server error: %v", errorResponse)
	}

	var segment models.Segment

	if err := json.NewDecoder(resp.Body).Decode(&segment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &segment, nil
}

// CreateSegment creates a new segment in the specified portfolio.
//
// Segments allow for further categorization and grouping of accounts or other entities
// within a portfolio, enabling more detailed reporting and management.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - organizationID: The ID of the organization where the segment will be created.
//     Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the segment will be created.
//     Must be a valid ledger ID within the specified organization.
//   - portfolioID: The ID of the portfolio where the segment will be created.
//     Must be a valid portfolio ID within the specified ledger.
//   - input: The segment details, including required fields:
//   - Name: The human-readable name of the segment
//     Optional fields include:
//   - Status: The initial status (defaults to ACTIVE if not specified)
//   - Metadata: Additional custom information about the segment
//
// Returns:
//   - *models.Segment: The created segment if successful, containing the segment ID,
//     name, and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Invalid input (missing required fields or invalid values)
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization, ledger, or portfolio ID)
//   - Network or server errors
//
// Example - Creating a basic segment:
//
//	// Create a simple segment with just required fields
//	segment, err := segmentsService.CreateSegment(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    "portfolio-789",
//	    models.NewCreateSegmentInput("North America Region"),
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create segment: %v", err)
//	}
//
//	// Use the segment
//	fmt.Printf("Segment created: %s (status: %s)\n",
//	    segment.ID, segment.Status.Code)
//
// Example - Creating a segment with metadata:
//
//	// Create a segment with custom metadata
//	input := models.NewCreateSegmentInput("EMEA Region").
//	    WithStatus(models.StatusActive).
//	    WithMetadata(map[string]any{
//	        "regionCode": "EMEA",
//	        "countries": []string{"UK", "France", "Germany", "Italy"},
//	        "headquarters": "London",
//	        "manager": "John Smith",
//	    })
//
//	segment, err := segmentsService.CreateSegment(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    "portfolio-789",
//	    input,
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create segment: %v", err)
//	}
//
//	// Use the segment
//	fmt.Printf("Segment created: %s\n", segment.ID)
//	fmt.Printf("Name: %s\n", segment.Name)
func (e *segmentsEntity) CreateSegment(
	ctx context.Context,
	organizationID, ledgerID, portfolioID string,
	input *models.CreateSegmentInput,
) (*models.Segment, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if portfolioID == "" {
		return nil, fmt.Errorf("portfolio ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("segment input cannot be nil")
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid segment input: %v", err)
	}

	url := e.buildURL(organizationID, ledgerID, portfolioID, "")

	body, err := json.Marshal(input)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check for error status codes
	if resp.StatusCode >= 400 {
		var errorResponse map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("server error: %v", errorResponse)
	}

	var segment models.Segment

	if err := json.NewDecoder(resp.Body).Decode(&segment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &segment, nil
}

// UpdateSegment updates an existing segment.
// The organizationID, ledgerID, and portfolioID parameters specify which organization, ledger, and portfolio the segment belongs to.
// The id parameter is the unique identifier of the segment to update.
// The input parameter contains the segment details to update, such as name, description, or status.
// Returns the updated segment, or an error if the operation fails.
func (e *segmentsEntity) UpdateSegment(
	ctx context.Context,
	organizationID, ledgerID, portfolioID, id string,
	input *models.UpdateSegmentInput,
) (*models.Segment, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if portfolioID == "" {
		return nil, fmt.Errorf("portfolio ID is required")
	}

	if id == "" {
		return nil, fmt.Errorf("segment ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("segment input cannot be nil")
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid segment update input: %v", err)
	}

	url := e.buildURL(organizationID, ledgerID, portfolioID, id)

	body, err := json.Marshal(input)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(body))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check for error status codes
	if resp.StatusCode >= 400 {
		var errorResponse map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("server error: %v", errorResponse)
	}

	var segment models.Segment

	if err := json.NewDecoder(resp.Body).Decode(&segment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &segment, nil
}

// DeleteSegment deletes a segment.
// The organizationID, ledgerID, and portfolioID parameters specify which organization, ledger, and portfolio the segment belongs to.
// The id parameter is the unique identifier of the segment to delete.
// Returns an error if the operation fails.
func (e *segmentsEntity) DeleteSegment(
	ctx context.Context,
	organizationID, ledgerID, portfolioID, id string,
) error {
	if organizationID == "" {
		return fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return fmt.Errorf("ledger ID is required")
	}

	if portfolioID == "" {
		return fmt.Errorf("portfolio ID is required")
	}

	if id == "" {
		return fmt.Errorf("segment ID is required")
	}

	url := e.buildURL(organizationID, ledgerID, portfolioID, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)

	if err != nil {
		return fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// Check for error status codes
	if resp.StatusCode >= 400 {
		var errorResponse map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return fmt.Errorf("server returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("server error: %v", errorResponse)
	}

	return nil
}

// buildURL builds the URL for segments API calls.
// The organizationID, ledgerID, and portfolioID parameters specify which organization, ledger, and portfolio to query.
// The id parameter is the unique identifier of the segment to retrieve, or an empty string for a list of segments.
// Returns the built URL.
func (e *segmentsEntity) buildURL(organizationID, ledgerID, portfolioID, segmentID string) string {
	base := e.baseURLs["onboarding"]
	if segmentID == "" {
		return fmt.Sprintf("%s/organizations/%s/ledgers/%s/portfolios/%s/segments", base, organizationID, ledgerID, portfolioID)
	}
	return fmt.Sprintf("%s/organizations/%s/ledgers/%s/portfolios/%s/segments/%s", base, organizationID, ledgerID, portfolioID, segmentID)
}

// setCommonHeaders sets common headers for API requests.
// It sets the Content-Type header to application/json, the Authorization header with the client's auth token,
// and the User-Agent header with the client's user agent.
func (e *segmentsEntity) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.authToken))
}
