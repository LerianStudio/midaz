package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/pkg/mmodel"
)

type segment struct {
	Factory *factory.Factory
}

// func (r *segment) Create(organizationID, ledgerID string, inp mmodel.CreateSegmentInput) (*mmodel.Segment, error) { performs an operation
func (r *segment) Create(organizationID, ledgerID string, inp mmodel.CreateSegmentInput) (*mmodel.Segment, error) {
	jsonData, err := json.Marshal(inp)

	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/segments",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID)

	req, err := http.NewRequest(http.MethodPost, uri, bytes.NewBuffer(jsonData))

	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)

	if err != nil {
		return nil, errors.New("making POST request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusCreated); err != nil {
		return nil, err
	}

	var segmentResp mmodel.Segment

	if err := json.NewDecoder(resp.Body).Decode(&segmentResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &segmentResp, nil
}

// func (r *segment) Get(organizationID, ledgerID string, limit, page int, sortOrder, startDate, endDate string) (*mmodel.Segments, error) { performs an operation
func (r *segment) Get(organizationID, ledgerID string, limit, page int, sortOrder, startDate, endDate string) (*mmodel.Segments, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/segments",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID)

	reqURL, err := BuildPaginatedURL(baseURL, limit, page, sortOrder, startDate, endDate)

	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)

	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)

	if err != nil {
		return nil, errors.New("making GET request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var segmentsResp mmodel.Segments

	if err := json.NewDecoder(resp.Body).Decode(&segmentsResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &segmentsResp, nil
}

// func (r *segment) GetByID(organizationID, ledgerID, segmentID string) (*mmodel.Segment, error) { performs an operation
func (r *segment) GetByID(organizationID, ledgerID, segmentID string) (*mmodel.Segment, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/segments/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, segmentID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)

	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)

	if err != nil {
		return nil, errors.New("making GET request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var segmentResp mmodel.Segment

	if err := json.NewDecoder(resp.Body).Decode(&segmentResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &segmentResp, nil
}

// func (r *segment) Update( performs an operation
func (r *segment) Update(
	organizationID, ledgerID, segmentID string, inp mmodel.UpdateSegmentInput,
) (*mmodel.Segment, error) {
	jsonData, err := json.Marshal(inp)

	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/segments/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, segmentID)

	req, err := http.NewRequest(http.MethodPatch, uri, bytes.NewBuffer(jsonData))

	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)

	if err != nil {
		return nil, errors.New("making PATCH request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var segmentResp mmodel.Segment

	if err := json.NewDecoder(resp.Body).Decode(&segmentResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &segmentResp, nil
}

// func (r *segment) Delete(organizationID, ledgerID, segmentID string) error { performs an operation
func (r *segment) Delete(organizationID, ledgerID, segmentID string) error {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/segments/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, segmentID)

	req, err := http.NewRequest(http.MethodDelete, uri, nil)

	if err != nil {
		return errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)

	if err != nil {
		return errors.New("making Delete request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusNoContent); err != nil {
		return err
	}

	return nil
}

// \1 performs an operation
func NewSegment(f *factory.Factory) *segment {
	return &segment{f}
}
