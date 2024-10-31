package mockutil

import (
	"net/http"
	"os"

	"github.com/jarcoal/httpmock"
)

func loadJSONResponse(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func MockResponseFromFile(status int, filepath string) httpmock.Responder {
	data, err := loadJSONResponse(filepath)
	if err != nil {
		return httpmock.NewStringResponder(http.StatusInternalServerError,
			"Failed to load mock response")
	}

	return httpmock.NewBytesResponder(status, data)
}
