package version

import (
	"bytes"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestFactoryVersionRunE(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "Standard version output",
			version:  "1.0.0",
			expected: color.New(color.Bold).Sprint("1.0.0") + "\n",
		},
		{
			name:     "Empty version",
			version:  "",
			expected: color.New(color.Bold).Sprint("") + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &factory.Factory{
				CLIVersion: tt.version,
				IOStreams: &iostreams.IOStreams{
					Out: new(bytes.Buffer),
				},
			}

			fVersion := &factoryVersion{
				factory: f,
			}
			cmd := &cobra.Command{
				RunE: fVersion.runE,
			}

			err := cmd.Execute()
			assert.NoError(t, err)

			output := f.IOStreams.Out.(*bytes.Buffer).String()
			assert.Equal(t, tt.expected, output)
		})
	}
}
