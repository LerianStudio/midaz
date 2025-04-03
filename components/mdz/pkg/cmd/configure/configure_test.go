package configure

import (
	"bytes"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/setting"
	"github.com/stretchr/testify/assert"
	"gotest.tools/golden"
)

// \1 performs an operation
func TestNewCmdConfigure(t *testing.T) {
	vlClientSecret := "secret123"
	vlClientID := "1234"
	vlURLAPIAuth := "http://localhost:8080"
	vlURLAPILedger := "http://localhost:3000"

	confFactory := factoryConfigure{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		tuiInput: func(message string) (string, error) {
			return vlClientSecret, nil
		},
		read: func() (*setting.Setting, error) {
			return &setting.Setting{
				Env: environment.Env{
					ClientID:     "",
					ClientSecret: "",
					URLAPIAuth:   "",
					URLAPILedger: "",
				},
			}, nil
		},
		save: func(sett setting.Setting) error {
			return nil
		},
		flagsConfigure: flagsConfigure{
			ClientID:     vlClientID,
			ClientSecret: vlClientSecret,
			URLAPIAuth:   vlURLAPIAuth,
			URLAPILedger: vlURLAPILedger,
		},
	}

	cmd := NewCmdConfigure(&confFactory)
	cmd.SetArgs([]string{
		"--client-id", vlClientID,
		"--url-api-auth", vlURLAPIAuth,
		"--url-api-ledger", vlURLAPILedger,
	})

	err := cmd.Execute()
	assert.NoError(t, err)

	output := confFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
	golden.AssertBytes(t, output, "output_configure.golden")

}
