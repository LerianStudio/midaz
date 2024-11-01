package mockutil

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/jarcoal/httpmock"
)

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
