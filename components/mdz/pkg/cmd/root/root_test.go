package root

import (
	"bytes"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

func TestNewCmdRoot(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		expectedOut  string
		expectedErr  string
		expectedHelp string
	}{
		{
			name:        "Standard output",
			version:     "1.0.0",
			expectedOut: "1.0.0",
		},
		{
			name:        "Version empty",
			version:     "",
			expectedOut: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outBuf := new(bytes.Buffer)
			errBuf := new(bytes.Buffer)
			f := &factory.Factory{
				CLIVersion: tt.version,
				IOStreams: &iostreams.IOStreams{
					Out: outBuf,
					Err: errBuf,
				},
			}

			cmd := NewCmdRoot(f)
			cmd.SetArgs([]string{"--help"})
			err := cmd.Execute()
			assert.NoError(t, err)
		})
	}
}
