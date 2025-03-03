package mockutil

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/jarcoal/httpmock"
)

// RoundTripFunc is a function type that implements the RoundTripper interface
type RoundTripFunc func(req *http.Request) *http.Response

// RoundTrip executes the mock round trip function
func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

// NewMockHTTPClient returns a new http.Client with Transport replaced with the mock round tripper
func NewMockHTTPClient(fn RoundTripFunc) *http.Client {
	return &http.Client{
		Transport: fn,
	}
}

func loadJSONResponse(filename string) ([]byte, error) {
	filename = filepath.Clean(filename)
	return os.ReadFile(filename)
}

func MockResponseFromFile(status int, path string) httpmock.Responder {
	data, err := loadJSONResponse(path)
	if err != nil {
		return httpmock.NewStringResponder(http.StatusInternalServerError,
			"Failed to load mock response")
	}

	return httpmock.NewBytesResponder(status, data)
}
