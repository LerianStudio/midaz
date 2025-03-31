package assetrate

import (
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdAssetRate(t *testing.T) {
	tests := []struct {
		name           string
		expectedUse    string
		expectedShort  string
		subcommandUses []string
	}{
		{
			name:          "creates asset rate command with correct attributes",
			expectedUse:   "asset-rate",
			expectedShort: "Manages asset rates within a ledger.",
			subcommandUses: []string{
				"create",
				"update",
				"list",
				"describe",
				"list-by-asset",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ios := iostreams.System()
			f := &factory.Factory{
				IOStreams: ios,
			}

			// Execute
			cmd := NewCmdAssetRate(f)

			// Verify
			assert.Equal(t, tt.expectedUse, cmd.Use)
			assert.Equal(t, tt.expectedShort, cmd.Short)
			assert.True(t, cmd.HasSubCommands())

			// Verify subcommands
			subcommandUses := make([]string, 0, len(cmd.Commands()))
			for _, subCmd := range cmd.Commands() {
				subcommandUses = append(subcommandUses, subCmd.Use)
			}

			assert.ElementsMatch(t, tt.subcommandUses, subcommandUses)

			// Verify help flag
			helpFlag := cmd.Flag("help")
			assert.NotNil(t, helpFlag)
			assert.Equal(t, "h", helpFlag.Shorthand)
		})
	}
}

func TestFactoryAssetRateSetCmds(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}
	fAssetRate := &factoryAssetRate{
		factory: f,
	}

	cmd := &cobra.Command{
		Use: "test-cmd",
	}

	// Execute
	fAssetRate.setCmds(cmd)

	// Verify
	assert.Equal(t, 5, len(cmd.Commands()))

	// Verify each subcommand exists
	subcommandUses := []string{"create", "update", "list", "describe", "list-by-asset"}
	for _, use := range subcommandUses {
		found := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Use == use {
				found = true
				break
			}
		}
		assert.True(t, found, "Subcommand %s not found", use)
	}
}
