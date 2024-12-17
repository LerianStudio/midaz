package factory

import (
	"net/http"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
)

type Factory struct {
	Token      string
	HTTPClient *http.Client
	IOStreams  *iostreams.IOStreams
	Env        *environment.Env
	Flags
}

type Flags struct {
	NoColor bool
}

func NewFactory(env *environment.Env) *Factory {
	return &Factory{
		HTTPClient: &http.Client{},
		IOStreams:  iostreams.System(),
		Env:        env,
	}
}
