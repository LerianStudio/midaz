package factory

import (
	"net/http"

	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
)

const CLIVersion = "Mdz CLI 1.0.0"

type Factory struct {
	CLIVersion string
	HTTPClient *http.Client
	IOStreams  *iostreams.IOStreams
}

func NewFactory() *Factory {
	return &Factory{
		CLIVersion: CLIVersion,
		HTTPClient: &http.Client{},
		IOStreams:  iostreams.System(),
	}
}
