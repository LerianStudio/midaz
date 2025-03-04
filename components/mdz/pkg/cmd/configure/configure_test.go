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

func TestNewCmdConfigure(t *testing.T) {
	vlClientSecret := "secret123"
	vlClientID := "1234"
	vlURLAPIAuth := "http://localhost:8080"
	vlURLAPIOnboarding := "http://localhost:3000"
	vlURLAPITransaction := "http://localhost:3001"

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
					ClientID:          "",
					ClientSecret:      "",
					URLAPIAuth:        "",
					URLAPIOnboarding:  "",
					URLAPITransaction: "",
				},
			}, nil
		},
		save: func(sett setting.Setting) error {
			return nil
		},
		flagsConfigure: flagsConfigure{
			ClientID:          vlClientID,
			ClientSecret:      vlClientSecret,
			URLAPIAuth:        vlURLAPIAuth,
			URLAPIOnboarding:  vlURLAPIOnboarding,
			URLAPITransaction: vlURLAPITransaction,
		},
	}

	cmd := NewCmdConfigure(&confFactory)
	cmd.SetArgs([]string{
		"--client-id", vlClientID,
		"--url-api-auth", vlURLAPIAuth,
		"--url-api-onboarding", vlURLAPIOnboarding,
		"--url-api-transaction", vlURLAPITransaction,
	})

	err := cmd.Execute()
	assert.NoError(t, err)

	output := confFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
	golden.AssertBytes(t, output, "output_configure.golden")

}
