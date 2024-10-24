package factory

import (
	"net/http"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
)

const CLIVersion = "Mdz CLI 1.0.0"

type Factory struct {
	CLIVersion string
	Token      string
	HTTPClient *http.Client
	IOStreams  *iostreams.IOStreams
	Env        *environment.Env
}

func NewFactory(env *environment.Env) *Factory {
	return &Factory{
		CLIVersion: CLIVersion,
		HTTPClient: &http.Client{},
		IOStreams:  iostreams.System(),
		Env:        env,
	}
}
