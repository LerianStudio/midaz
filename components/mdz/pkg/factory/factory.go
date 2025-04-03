package factory

import (
	"net/http"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
)

// \1 represents an entity
type Factory struct {
	Token      string
	HTTPClient *http.Client
	IOStreams  *iostreams.IOStreams
	Env        *environment.Env
	Flags
}

// \1 represents an entity
type Flags struct {
	NoColor bool
}

// \1 performs an operation
func NewFactory(env *environment.Env) *Factory {
	return &Factory{
		HTTPClient: &http.Client{},
		IOStreams:  iostreams.System(),
		Env:        env,
	}
}
